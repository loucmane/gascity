package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	convoycore "github.com/gastownhall/gascity/internal/convoy"
	"github.com/gastownhall/gascity/internal/formula"
	"github.com/gastownhall/gascity/internal/graphv2"
	"github.com/gastownhall/gascity/internal/molecule"
)

// ErrDrainItemRetryRefused classifies a retry request that could not prove a
// safe, single failed drain item to replace. Callers surface it as operator
// attention; the operation never mutates on this error.
var ErrDrainItemRetryRefused = errors.New("failed drain item retry refused")

// DrainItemReplacementResult identifies the append-forward replacement. A
// repeated request after a successful swap returns the same NewRootID with
// AlreadyReplaced set and creates nothing.
type DrainItemReplacementResult struct {
	OldRootID       string
	NewRootID       string
	AlreadyReplaced bool
}

// RetryFailedDrainItem atomically retires one terminally failed drain-item
// workflow and replaces it from the exact formula and frozen runtime variables
// recorded on that workflow root. The old graph remains closed failure
// evidence; the persisted drain manifest switches to the replacement in the
// same transaction so the controller never observes a half-applied retry.
func RetryFailedDrainItem(ctx context.Context, store beads.Store, controlID, memberID string, opts ProcessOptions) (result DrainItemReplacementResult, err error) {
	controlID = strings.TrimSpace(controlID)
	memberID = strings.TrimSpace(memberID)
	defer func() {
		if err != nil {
			opts.tracef("drain-item-retry attention control=%s member=%s err=%v", controlID, memberID, err)
		}
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	if store == nil || controlID == "" || memberID == "" {
		return result, retryDrainRefusal("store, control id, and member id are required")
	}
	if !beads.StoreSupportsAtomicTx(store) {
		return result, retryDrainRefusal("store %T does not provide atomic transactions", store)
	}

	control, err := store.Get(controlID)
	if err != nil {
		return result, retryDrainRefusal("loading drain control %s: %v", controlID, err)
	}
	if control.Status == "closed" || strings.TrimSpace(control.Metadata[beadmeta.KindMetadataKey]) != beadmeta.KindDrain {
		return result, retryDrainRefusal("%s is not an open drain control", controlID)
	}
	rawManifest := strings.TrimSpace(control.Metadata[drainManifestMetadataKey])
	manifest, err := parseDrainManifest(rawManifest)
	if err != nil {
		return result, retryDrainRefusal("%s has no valid persisted drain manifest: %v", controlID, err)
	}
	rowIndex := -1
	for i := range manifest.Rows {
		if strings.TrimSpace(manifest.Rows[i].MemberID) == memberID {
			if rowIndex >= 0 {
				return result, retryDrainRefusal("%s manifest has ambiguous rows for member %s", controlID, memberID)
			}
			rowIndex = i
		}
	}
	if rowIndex < 0 {
		return result, retryDrainRefusal("%s manifest has no row for member %s", controlID, memberID)
	}
	row := manifest.Rows[rowIndex]
	if strings.TrimSpace(row.ItemRootID) == "" || strings.TrimSpace(row.UnitConvoyID) == "" || strings.TrimSpace(row.ItemRootKey) == "" {
		return result, retryDrainRefusal("%s member %s row is not fully materialized", controlID, memberID)
	}
	oldRoot, err := store.Get(row.ItemRootID)
	if err != nil {
		return result, retryDrainRefusal("loading drain item root %s: %v", row.ItemRootID, err)
	}
	if replaced := strings.TrimSpace(oldRoot.Metadata[beadmeta.DrainReplacesMetadataKey]); replaced != "" {
		return DrainItemReplacementResult{OldRootID: replaced, NewRootID: oldRoot.ID, AlreadyReplaced: true}, nil
	}
	if strings.TrimSpace(oldRoot.Metadata[beadmeta.OutcomeMetadataKey]) == beadmeta.OutcomePass {
		return result, retryDrainRefusal("drain item root %s already succeeded", oldRoot.ID)
	}

	oldWorkflow, err := listByWorkflowRoot(store, oldRoot.ID)
	if err != nil {
		return result, retryDrainRefusal("listing workflow %s: %v", oldRoot.ID, err)
	}
	if !drainWorkflowTerminallyFailed(oldRoot, oldWorkflow) {
		return result, retryDrainRefusal("drain item root %s is not terminally failed", oldRoot.ID)
	}

	unit, err := store.Get(row.UnitConvoyID)
	if err != nil {
		return result, retryDrainRefusal("loading drain unit %s: %v", row.UnitConvoyID, err)
	}
	memberStore, err := drainMemberOwningStore(store, memberID, opts)
	if err != nil {
		return result, retryDrainRefusal("resolving member %s store: %v", memberID, err)
	}
	member, err := memberStore.Get(memberID)
	if err != nil {
		return result, retryDrainRefusal("loading drain member %s: %v", memberID, err)
	}

	formulaSource := filepath.Clean(strings.TrimSpace(oldRoot.Metadata[beadmeta.FormulaSourceMetadataKey]))
	if formulaSource == "." || !filepath.IsAbs(formulaSource) {
		return result, retryDrainRefusal("drain item root %s has no absolute gc.formula_source", oldRoot.ID)
	}
	formulaName, ok := formula.TrimTOMLFilename(filepath.Base(formulaSource))
	if !ok || strings.TrimSpace(formulaName) == "" {
		return result, retryDrainRefusal("drain item root %s has invalid gc.formula_source %q", oldRoot.ID, formulaSource)
	}
	frozenVars, err := graphv2.ParseRuntimeVarsMetadata(oldRoot.Metadata[graphv2.RuntimeVarsMetadataKey])
	if err != nil {
		return result, retryDrainRefusal("parsing frozen runtime vars on %s: %v", oldRoot.ID, err)
	}
	// gc.runtime_vars.v1 intentionally omits reserved invocation variables.
	// Reconstruct only those values from the persisted drain identity, exactly
	// as initial drain-item materialization does; every non-reserved value still
	// comes exclusively from the frozen root metadata.
	runtimeVars := make(map[string]string, len(frozenVars)+2)
	for key, value := range frozenVars {
		runtimeVars[key] = value
	}
	runtimeVars[graphv2.ConvoyIDVar] = unit.ID
	if !convoycore.IsUnresolvedTrackedItem(member) && member.ID != "" {
		runtimeVars[graphv2.LegacyIssueVar] = member.ID
	}
	recipe, err := formula.CompileWithoutRuntimeVarValidation(ctx, formulaName, []string{filepath.Dir(formulaSource)}, runtimeVars)
	if err != nil {
		return result, retryDrainRefusal("compiling pinned formula %q: %v", formulaSource, err)
	}
	if filepath.Clean(recipe.FormulaSource) != formulaSource {
		return result, retryDrainRefusal("compiled formula source %q does not match pinned %q", recipe.FormulaSource, formulaSource)
	}
	if !isGraphV2WorkflowRecipe(recipe) {
		return result, retryDrainRefusal("pinned formula %q is not a graph.v2 workflow", formulaSource)
	}
	if err := molecule.ValidateRecipeRuntimeVars(recipe, molecule.Options{Vars: runtimeVars}); err != nil {
		return result, retryDrainRefusal("validating frozen runtime vars for %q: %v", formulaSource, err)
	}
	stampDrainItemRecipe(recipe, control, unit, member, len(manifest.Rows), &row, manifest.Formula, runtimeVars)
	if opts.PrepareRecipe != nil {
		if err := opts.PrepareRecipe(recipe, control); err != nil {
			return result, retryDrainRefusal("preparing pinned formula %q: %v", formulaSource, err)
		}
	}
	blockerIDs, err := drainProjectedBlockerIDs(store, memberID, manifest, opts)
	if err != nil {
		return result, retryDrainRefusal("projecting blockers for member %s: %v", memberID, err)
	}
	dependents, err := store.DepList(oldRoot.ID, "up")
	if err != nil {
		return result, retryDrainRefusal("listing dependents of old root %s: %v", oldRoot.ID, err)
	}

	guardControl := cloneRetryGuardBead(control)
	guardUnit := cloneRetryGuardBead(unit)
	guardWorkflow := make(map[string]beads.Bead, len(oldWorkflow))
	for _, bead := range oldWorkflow {
		guardWorkflow[bead.ID] = cloneRetryGuardBead(bead)
	}

	err = store.Tx("gc: replace failed drain item "+oldRoot.ID, func(rawTx beads.Tx) error {
		tx, ok := rawTx.(beads.GraphTx)
		if !ok {
			return retryDrainRefusal("store %T transaction does not support graph reads and dependency writes", store)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		currentControl, err := tx.Get(control.ID)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(cloneRetryGuardBead(currentControl), guardControl) || strings.TrimSpace(currentControl.Metadata[drainManifestMetadataKey]) != rawManifest {
			return retryDrainRefusal("drain control %s changed concurrently", control.ID)
		}
		currentUnit, err := tx.Get(unit.ID)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(cloneRetryGuardBead(currentUnit), guardUnit) {
			return retryDrainRefusal("drain unit %s changed concurrently", unit.ID)
		}
		for id, expected := range guardWorkflow {
			current, err := tx.Get(id)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(cloneRetryGuardBead(current), expected) {
				return retryDrainRefusal("old workflow bead %s changed concurrently", id)
			}
		}

		created, err := molecule.InstantiateTx(tx, recipe, molecule.Options{
			Vars:             runtimeVars,
			ExternalDeps:     drainWorkflowExternalDeps(recipe, blockerIDs),
			PriorityOverride: member.Priority,
		})
		if err != nil {
			return fmt.Errorf("instantiating replacement from %q: %w", formulaSource, err)
		}
		result = DrainItemReplacementResult{OldRootID: oldRoot.ID, NewRootID: created.RootID}
		if err := tx.SetMetadataBatch(created.RootID, map[string]string{beadmeta.DrainReplacesMetadataKey: oldRoot.ID}); err != nil {
			return fmt.Errorf("linking replacement root %s: %w", created.RootID, err)
		}

		for _, dep := range dependents {
			if !beads.IsReadyBlockingDependencyType(dep.Type) || dep.IssueID == "" || dep.IssueID == created.RootID {
				continue
			}
			if err := tx.DepAdd(dep.IssueID, created.RootID, dep.Type); err != nil {
				return fmt.Errorf("rewiring dependent %s to replacement %s: %w", dep.IssueID, created.RootID, err)
			}
		}

		for _, old := range oldWorkflow {
			metadata := map[string]string{
				beadmeta.DrainReplacedByMetadataKey: created.RootID,
				beadmeta.FailureReasonMetadataKey:   drainReplacementFailureReason(old),
			}
			if old.ID == oldRoot.ID {
				metadata[beadmeta.OutcomeMetadataKey] = beadmeta.OutcomeFail
				metadata[beadmeta.MoleculeFailedMetadataKey] = "true"
			}
			if err := tx.SetMetadataBatch(old.ID, metadata); err != nil {
				return fmt.Errorf("recording replacement lineage on %s: %w", old.ID, err)
			}
			if strings.TrimSpace(old.Assignee) != "" {
				empty := ""
				if err := tx.Update(old.ID, beads.UpdateOpts{Assignee: &empty}); err != nil {
					return fmt.Errorf("clearing stale owner on %s: %w", old.ID, err)
				}
			}
			if old.Status != "closed" {
				if err := tx.Close(old.ID); err != nil {
					return fmt.Errorf("closing old workflow bead %s: %w", old.ID, err)
				}
			}
		}

		manifest.Rows[rowIndex].ItemRootID = created.RootID
		manifest.Rows[rowIndex].Status = "wired"
		manifest.Rows[rowIndex].OutcomeBead = ""
		manifest.Rows[rowIndex].OutcomeKind = ""
		manifest.Rows[rowIndex].Failure = ""
		encoded, err := json.Marshal(manifest)
		if err != nil {
			return err
		}
		if err := tx.SetMetadataBatch(control.ID, map[string]string{drainManifestMetadataKey: string(encoded)}); err != nil {
			return fmt.Errorf("persisting replacement manifest: %w", err)
		}
		return nil
	})
	if err != nil {
		return DrainItemReplacementResult{}, err
	}
	return result, nil
}

func retryDrainRefusal(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrDrainItemRetryRefused, fmt.Sprintf(format, args...))
}

func drainWorkflowTerminallyFailed(root beads.Bead, workflow []beads.Bead) bool {
	if root.Status == "closed" && (root.Metadata[beadmeta.OutcomeMetadataKey] == beadmeta.OutcomeFail || root.Metadata[beadmeta.FailureReasonMetadataKey] != "") {
		return true
	}
	for _, bead := range workflow {
		if bead.Status != "closed" {
			continue
		}
		if bead.Metadata[beadmeta.OutcomeMetadataKey] == beadmeta.OutcomeFail || bead.Metadata[beadmeta.FailureReasonMetadataKey] != "" {
			return true
		}
	}
	return false
}

func drainReplacementFailureReason(bead beads.Bead) string {
	if reason := strings.TrimSpace(bead.Metadata[beadmeta.FailureReasonMetadataKey]); reason != "" {
		return reason
	}
	return "replaced_failed_drain_item"
}

func cloneRetryGuardBead(bead beads.Bead) beads.Bead {
	// Revision and ClaimFence are backend-local concurrency tokens. The native
	// transaction itself provides serialization; compare the durable shape that
	// defines the retry instead of rejecting a cache refresh of those tokens.
	bead.Revision = 0
	bead.ClaimFence = 0
	return bead
}

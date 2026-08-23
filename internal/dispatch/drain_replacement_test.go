package dispatch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/formula"
)

// atomicDrainTestStore advertises the production all-or-nothing transaction
// requirement while retaining MemStore's deterministic IDs for the behavioral
// fixture. Failure-boundary tests below use a staged transaction store so this
// success fixture cannot accidentally stand in for rollback evidence.
type atomicDrainTestStore struct{ *beads.MemStore }

func (*atomicDrainTestStore) AtomicTx() bool { return true }

func TestRetryFailedDrainItemReplacesTerminallyBlockedWorkflow(t *testing.T) {
	dir := t.TempDir()
	formulaPath := writeRetryableDrainItemFormula(t, dir)
	mem, drain := seedDrainWorkflow(t)
	store := &atomicDrainTestStore{MemStore: mem}

	if _, err := ProcessControl(store, drain, ProcessOptions{FormulaSearchPaths: []string{dir}}); err != nil {
		t.Fatalf("ProcessControl(drain expand): %v", err)
	}
	drain = mustGetBead(t, store, drain.ID)
	manifestBefore := mustDrainManifest(t, drain)
	rowBefore := manifestBefore.Rows[0]
	oldRoot := mustGetBead(t, store, rowBefore.ItemRootID)
	if got := oldRoot.Metadata[beadmeta.FormulaSourceMetadataKey]; got != formulaPath {
		t.Fatalf("old root formula source = %q, want pinned %q", got, formulaPath)
	}
	member := mustGetBead(t, store, rowBefore.MemberID)
	if err := store.SetMetadataBatch(member.ID, map[string]string{
		"gc.base_commit":   "a83fa64caa0778f3b0212a0fc7728d1b2a43432e",
		"gc.worktree_path": filepath.Join(t.TempDir(), "policy-worktrees", member.ID),
	}); err != nil {
		t.Fatalf("stamp corrected source inputs: %v", err)
	}

	prepare := mustFindDrainItemStepByRef(t, store, oldRoot.ID, "prepare")
	if err := updateMetadataAndClose(store, prepare.ID, map[string]string{
		beadmeta.OutcomeMetadataKey:       beadmeta.OutcomeFail,
		beadmeta.FailureClassMetadataKey:  beadmeta.FailureClassHard,
		beadmeta.FailureReasonMetadataKey: "validator_missing",
	}); err != nil {
		t.Fatalf("close prepare step as failure evidence: %v", err)
	}

	prepareCalls := 0
	result, err := RetryFailedDrainItem(context.Background(), store, drain.ID, member.ID, ProcessOptions{
		FormulaSearchPaths: []string{dir},
		PrepareRecipe: func(recipe *formula.Recipe, _ beads.Bead) error {
			prepareCalls++
			if recipe.FormulaSource != formulaPath {
				return errors.New("replacement did not use the pinned formula source")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("RetryFailedDrainItem: %v", err)
	}
	if result.OldRootID != oldRoot.ID || result.NewRootID == "" || result.NewRootID == oldRoot.ID {
		t.Fatalf("result = %+v, want old %s and one distinct replacement", result, oldRoot.ID)
	}
	if prepareCalls != 1 {
		t.Fatalf("PrepareRecipe calls = %d, want exactly one pinned preparation", prepareCalls)
	}

	drainAfter := mustGetBead(t, store, drain.ID)
	manifestAfter := mustDrainManifest(t, drainAfter)
	rowAfter := manifestAfter.Rows[0]
	if rowAfter.ItemRootID != result.NewRootID || rowAfter.Status != "wired" {
		t.Fatalf("replacement row = %+v, want new root %s wired", rowAfter, result.NewRootID)
	}
	if rowAfter.UnitConvoyID != rowBefore.UnitConvoyID || rowAfter.ItemRootKey != rowBefore.ItemRootKey || rowAfter.MemberID != rowBefore.MemberID {
		t.Fatalf("replacement row lost frozen identity: before=%+v after=%+v", rowBefore, rowAfter)
	}

	oldWorkflow := mustListDrainWorkflow(t, store, oldRoot.ID)
	for _, bead := range oldWorkflow {
		if bead.Status != "closed" {
			t.Errorf("old workflow bead %s status = %q, want closed", bead.ID, bead.Status)
		}
		if bead.Metadata[beadmeta.FailureReasonMetadataKey] == "" {
			t.Errorf("old workflow bead %s missing failure reason", bead.ID)
		}
		if got := bead.Metadata[beadmeta.DrainReplacedByMetadataKey]; got != result.NewRootID {
			t.Errorf("old workflow bead %s replaced_by = %q, want %q", bead.ID, got, result.NewRootID)
		}
	}

	replacement := mustGetBead(t, store, result.NewRootID)
	if got := replacement.Metadata[beadmeta.DrainReplacesMetadataKey]; got != oldRoot.ID {
		t.Fatalf("replacement gc.drain_replaces = %q, want %q", got, oldRoot.ID)
	}
	if replacement.Metadata[beadmeta.FormulaSourceMetadataKey] != formulaPath {
		t.Fatalf("replacement formula source = %q, want %q", replacement.Metadata[beadmeta.FormulaSourceMetadataKey], formulaPath)
	}
	if replacement.Metadata[beadmeta.RuntimeVarsMetadataKey] != oldRoot.Metadata[beadmeta.RuntimeVarsMetadataKey] {
		t.Fatalf("replacement runtime vars = %q, want frozen %q", replacement.Metadata[beadmeta.RuntimeVarsMetadataKey], oldRoot.Metadata[beadmeta.RuntimeVarsMetadataKey])
	}
	if replacement.Metadata[beadmeta.DrainControlIDMetadataKey] != drain.ID ||
		replacement.Metadata[beadmeta.DrainMemberIDMetadataKey] != member.ID ||
		replacement.Metadata[beadmeta.InputConvoyIDMetadataKey] != rowBefore.UnitConvoyID {
		t.Fatalf("replacement metadata lost drain identity: %#v", replacement.Metadata)
	}
	assertHasBlockingDep(t, store, drain.ID, replacement.ID)

	again, err := RetryFailedDrainItem(context.Background(), store, drain.ID, member.ID, ProcessOptions{
		FormulaSearchPaths: []string{dir},
	})
	if err != nil {
		t.Fatalf("idempotent RetryFailedDrainItem: %v", err)
	}
	if !again.AlreadyReplaced || again.NewRootID != replacement.ID {
		t.Fatalf("idempotent result = %+v, want existing replacement %s", again, replacement.ID)
	}
	roots, err := store.ListByMetadata(map[string]string{beadmeta.ItemRootKeyMetadataKey: rowBefore.ItemRootKey}, 0, beads.IncludeClosed)
	if err != nil {
		t.Fatalf("list roots by stable key: %v", err)
	}
	if len(roots) != 2 {
		t.Fatalf("roots for stable key = %d, want old evidence + one replacement", len(roots))
	}

	if err := updateMetadataAndClose(store, replacement.ID, map[string]string{beadmeta.OutcomeMetadataKey: beadmeta.OutcomePass}); err != nil {
		t.Fatalf("close replacement pass: %v", err)
	}
	_, err = ProcessControl(store, drainAfter, ProcessOptions{FormulaSearchPaths: []string{dir}})
	if err != nil && !errors.Is(err, ErrControlPending) {
		t.Fatalf("controller continuation after replacement: %v", err)
	}
}

func writeRetryableDrainItemFormula(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "drain-item.formula.toml")
	content := `formula = "drain-item"
version = 1
contract = "graph.v2"
type = "workflow"

[[steps]]
id = "prepare"
title = "Prepare {{convoy_id}}"

[[steps]]
id = "work"
title = "Work {{convoy_id}}"

[[deps]]
step = "work"
depends_on = "prepare"
type = "blocks"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write retryable drain formula: %v", err)
	}
	return path
}

func mustFindDrainItemStepByRef(t *testing.T, store beads.Store, rootID, ref string) beads.Bead {
	t.Helper()
	items := mustListDrainWorkflow(t, store, rootID)
	for _, bead := range items {
		if bead.Ref == ref || bead.Metadata[beadmeta.StepRefMetadataKey] == ref ||
			strings.HasSuffix(bead.Ref, "."+ref) || strings.HasSuffix(bead.Metadata[beadmeta.StepRefMetadataKey], "."+ref) {
			return bead
		}
	}
	t.Fatalf("workflow %s has no step ref %q; items=%+v", rootID, ref, items)
	return beads.Bead{}
}

func mustListDrainWorkflow(t *testing.T, store beads.Store, rootID string) []beads.Bead {
	t.Helper()
	items, err := listByWorkflowRoot(store, rootID)
	if err != nil {
		t.Fatalf("list workflow %s: %v", rootID, err)
	}
	return items
}

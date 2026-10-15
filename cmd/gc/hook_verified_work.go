// Managed work claims carry revision-checked incarnation and attempt fences.
package main

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/rollout/gate"
	"github.com/google/uuid"
)

const (
	verifiedHookIncarnationKey = beadmeta.ClaimIncarnationMetadataKey
	verifiedHookAttemptKey     = beadmeta.ClaimAttemptMetadataKey
)

func (v *VerifiedSession) workIncarnation() string {
	return fmt.Sprintf("%d:%s", v.Generation, v.InstanceTokenSHA256)
}

// Use the configured work leg, or the class residence already proved by the
// claim route. Never rediscover a different ledger when compensating a claim.
var openVerifiedHookWorkStore = func(ctx context.Context, env []string, v *VerifiedSession, route *hookClaimClassRoute, dir, id string) (beads.Store, error) {
	if route != nil && route.knownResident(id) {
		return route.class, nil
	}
	for _, leg := range v.AllowedWorkStores {
		if leg.dir == dir {
			var commandEnv map[string]string
			if env != nil {
				commandEnv = hookClaimEnvMap(env, dir, v.assignee())
			}
			result, err := openStoreResultAtForCityContext(ctx, dir, v.cityPath, nil, gate.ModeUnset, false, false, false, commandEnv)
			return result.Store, err
		}
	}
	return nil, fmt.Errorf("work store is outside verified scope")
}

// fenceVerifiedHookWork is mandatory for both claims and adoptions. Even an
// adoption whose display metadata is unchanged advances the work revision.
// The attempt ID also fences an undelivered invocation from a later adoption
// or same-assignee reacquisition within the SAME session incarnation.
func fenceVerifiedHookWork(bead beads.Bead, opts hookClaimOptions, ops hookClaimOps, dir string) (beads.Bead, error) {
	v := opts.Verified
	if err := v.mayClaim(); err != nil {
		return beads.Bead{}, err
	}
	store, err := openVerifiedHookWorkStore(context.Background(), opts.Env, v, ops.ClassRoute, dir, bead.ID)
	if err != nil {
		return beads.Bead{}, err
	}
	live, err := store.Get(bead.ID)
	if err != nil {
		return beads.Bead{}, err
	}
	if live.ID != bead.ID || live.Status != "in_progress" || !slices.Contains(v.identifiers(), live.Assignee) {
		return beads.Bead{}, &beads.PreconditionFailedError{ID: bead.ID}
	}
	writer, ok := beads.RequiredConditionalWriterFor(store)
	if !ok {
		return beads.Bead{}, beads.ErrConditionalWriteUnsupported
	}
	patch := hookClaimIdentityPatch(live, opts, ops, dir)
	// Control kinds intentionally carry no execution-session back-references.
	// Their claim authority is the assignee plus the incarnation/attempt CAS.
	if !beadmeta.IsControlKind(strings.TrimSpace(live.Metadata[beadmeta.KindMetadataKey])) {
		patch[beadmeta.SessionIDMetadataKey] = v.ID
		patch[beadmeta.SessionNameMetadataKey] = v.SessionName
	}
	patch[verifiedHookIncarnationKey] = v.workIncarnation()
	patch[verifiedHookAttemptKey] = uuid.NewString()
	update := beads.UpdateOpts{Metadata: patch}
	// Read the work row BEFORE this last session check. A successor adopting
	// after it must change that row's revision, so the write below loses.
	if err := v.mayClaim(); err != nil {
		return beads.Bead{}, err
	}
	if err := writer.UpdateIfMatch(live.ID, live.Revision, update); err != nil {
		return beads.Bead{}, err
	}
	if live.Metadata == nil {
		live.Metadata = make(map[string]string)
	}
	maps.Copy(live.Metadata, patch)
	return live, nil
}

func restampVerifiedHookWork(v *VerifiedSession, route *hookClaimClassRoute, dir, id, from, to string) (bool, error) {
	if err := v.mayClaim(); err != nil {
		return false, err
	}
	if route != nil && route.knownResident(id) {
		return false, errRestampGraphResident
	}
	store, err := openVerifiedHookWorkStore(context.Background(), nil, v, route, dir, id)
	if err != nil {
		return false, err
	}
	live, err := store.Get(id)
	if err != nil {
		return false, err
	}
	if live.ID != id || live.Status != "in_progress" || live.Assignee != from ||
		!slices.Contains(v.identifiers(), from) || to != v.assignee() {
		return false, nil
	}
	writer, ok := beads.RequiredConditionalWriterFor(store)
	if !ok {
		return false, beads.ErrConditionalWriteUnsupported
	}
	if err := v.mayClaim(); err != nil {
		return false, err
	}
	err = writer.UpdateIfMatch(id, live.Revision, beads.UpdateOpts{Assignee: &to})
	if beads.IsPreconditionFailed(err) {
		return false, nil
	}
	return err == nil, err
}

func verifiedHookOwnsWork(v *VerifiedSession, b beads.Bead) bool {
	ownerMatches := b.Metadata[beadmeta.SessionIDMetadataKey] == v.ID ||
		beadmeta.IsControlKind(strings.TrimSpace(b.Metadata[beadmeta.KindMetadataKey]))
	return b.Status == "in_progress" && slices.Contains(v.identifiers(), b.Assignee) &&
		ownerMatches &&
		b.Metadata[verifiedHookIncarnationKey] == v.workIncarnation() &&
		b.Metadata[verifiedHookAttemptKey] != ""
}

// A pending unwind may run after the session has rolled over. Its authority is
// only the exact claim attempt it fenced, never an assignee or a fresh own-row
// lookup. Missing markers (including a claim whose fence failed) stay parked
// for controller recovery; there is no unconditional release fallback.
func releaseVerifiedHookPending(bead beads.Bead, opts hookClaimOptions, ops hookClaimOps, dir string) (bool, error) {
	v := opts.Verified
	if !verifiedHookOwnsWork(v, bead) {
		return false, nil
	}
	store, err := openVerifiedHookWorkStore(context.Background(), opts.Env, v, ops.ClassRoute, dir, bead.ID)
	if err != nil {
		return false, err
	}
	live, err := store.Get(bead.ID)
	if err != nil {
		return false, err
	}
	if live.ID != bead.ID || !verifiedHookOwnsWork(v, live) ||
		live.Metadata[verifiedHookAttemptKey] != bead.Metadata[verifiedHookAttemptKey] {
		return false, nil
	}
	writer, ok := beads.RequiredConditionalWriterFor(store)
	if !ok {
		return false, beads.ErrConditionalWriteUnsupported
	}
	err = writer.UpdateIfMatch(live.ID, live.Revision, verifiedHookReleasePatch())
	if beads.IsPreconditionFailed(err) {
		return false, nil
	}
	return err == nil, err
}

func verifiedHookReleasePatch() beads.UpdateOpts {
	status, empty := "open", ""
	return beads.UpdateOpts{Status: &status, Assignee: &empty, Metadata: map[string]string{
		beadmeta.SessionAffinityMetadataKey: "", beadmeta.ContinuationGroupMetadataKey: "",
		verifiedHookIncarnationKey: "", verifiedHookAttemptKey: "",
	}}
}

// pinVerifiedHookContinuation uses the same revision-fenced write boundary as
// managed claims, so proxy generation changes cannot bypass read-after-write checks.
func pinVerifiedHookContinuation(ctx context.Context, store beads.Store, sibling beads.Bead, opts hookClaimOptions, assignee string) error {
	v := opts.Verified
	if err := ctx.Err(); err != nil {
		return err
	}
	live, err := store.Get(sibling.ID)
	if err != nil {
		return err
	}
	if live.ID != sibling.ID || live.Status != "open" || live.Assignee != "" ||
		live.Metadata[beadmeta.RootBeadIDMetadataKey] != sibling.Metadata[beadmeta.RootBeadIDMetadataKey] ||
		live.Metadata[beadmeta.ContinuationGroupMetadataKey] != sibling.Metadata[beadmeta.ContinuationGroupMetadataKey] ||
		!hookClaimMatchesRoute(live, opts.RouteTargets) {
		return &beads.PreconditionFailedError{ID: sibling.ID}
	}
	writer, ok := beads.RequiredConditionalWriterFor(store)
	if !ok {
		return beads.ErrConditionalWriteUnsupported
	}
	if err := v.mayClaim(); err != nil {
		return err
	}
	return writer.UpdateIfMatch(live.ID, live.Revision, beads.UpdateOpts{Assignee: &assignee})
}

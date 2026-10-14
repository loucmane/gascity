// Managed work claims carry revision-checked incarnation and attempt fences.
package main

import (
	"fmt"
	"maps"
	"slices"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/google/uuid"
)

const (
	verifiedHookIncarnationKey = "gc.claim_incarnation"
	verifiedHookAttemptKey     = "gc.claim_attempt"
)

func (v *VerifiedSession) workIncarnation() string {
	return fmt.Sprintf("%d:%s", v.Generation, v.InstanceTokenSHA256)
}

// Use the configured work leg, or the class residence already proved by the
// claim route. Never rediscover a different ledger when compensating a claim.
var openVerifiedHookWorkStore = func(v *VerifiedSession, route *hookClaimClassRoute, dir, id string) (beads.Store, error) {
	if route != nil && route.knownResident(id) {
		return route.class, nil
	}
	for _, leg := range v.AllowedWorkStores {
		if leg.dir == dir {
			return openStoreAtForCity(dir, v.cityPath)
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
	store, err := openVerifiedHookWorkStore(v, ops.ClassRoute, dir, bead.ID)
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
	patch[beadmeta.SessionIDMetadataKey] = v.ID
	patch[beadmeta.SessionNameMetadataKey] = v.SessionName
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
	store, err := openVerifiedHookWorkStore(v, route, dir, id)
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
	return b.Status == "in_progress" && slices.Contains(v.identifiers(), b.Assignee) &&
		b.Metadata[beadmeta.SessionIDMetadataKey] == v.ID &&
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
	store, err := openVerifiedHookWorkStore(v, ops.ClassRoute, dir, bead.ID)
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

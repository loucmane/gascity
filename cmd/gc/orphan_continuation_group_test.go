package main

import (
	"testing"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/session"
)

func TestReleaseOrphanedPoolAssignmentsWhenSnapshotsComplete_ClearsOpenUnassignedContinuationGroup(t *testing.T) {
	store := beads.NewMemStore()
	first := createOpenContinuationGroupWork(t, store, "ga-first", "worker")
	second := createOpenContinuationGroupWork(t, store, "ga-second", "reviewer")

	reconciled := releaseOrphanedPoolAssignmentsWhenSnapshotsComplete(
		store,
		continuationGroupTestConfig(),
		"",
		nil,
		continuationGroupSnapshotResult(store, first, second),
		nil,
	)
	if len(reconciled) != 2 {
		t.Fatalf("reconciled = %v, want both orphaned group members", reconciled)
	}

	for _, want := range []beads.Bead{first, second} {
		got, err := store.Get(want.ID)
		if err != nil {
			t.Fatalf("Get(%s): %v", want.ID, err)
		}
		if got.Status != "open" || got.Assignee != "" {
			t.Errorf("%s status/assignee = %q/%q, want open/unassigned", got.ID, got.Status, got.Assignee)
		}
		if got.Metadata[beadmeta.ContinuationGroupMetadataKey] != "" {
			t.Errorf("%s continuation group = %q, want empty-value clear", got.ID, got.Metadata[beadmeta.ContinuationGroupMetadataKey])
		}
		if got.Metadata[beadmeta.RootBeadIDMetadataKey] != "ga-root" {
			t.Errorf("%s root = %q, want preserved", got.ID, got.Metadata[beadmeta.RootBeadIDMetadataKey])
		}
		if got.Metadata[beadmeta.RoutedToMetadataKey] != want.Metadata[beadmeta.RoutedToMetadataKey] {
			t.Errorf("%s route = %q, want %q", got.ID, got.Metadata[beadmeta.RoutedToMetadataKey], want.Metadata[beadmeta.RoutedToMetadataKey])
		}
		if got.Metadata["custom"] != "preserve-me" {
			t.Errorf("%s custom metadata = %q, want preserved", got.ID, got.Metadata["custom"])
		}
		if len(got.Labels) != 1 || got.Labels[0] != "priority:p1" {
			t.Errorf("%s labels = %v, want preserved", got.ID, got.Labels)
		}
		if len(got.Dependencies) != 1 || got.Dependencies[0].DependsOnID != "ga-blocker" {
			t.Errorf("%s dependencies = %v, want preserved", got.ID, got.Dependencies)
		}
	}
}

func TestReleaseOrphanedPoolAssignmentsWhenSnapshotsComplete_FreshRoutedWorkerClaimsClearedWorkExactlyOnce(t *testing.T) {
	store := beads.NewMemStore()
	work := createOpenContinuationGroupWork(t, store, "ga-work", "worker")

	reconciled := releaseOrphanedPoolAssignmentsWhenSnapshotsComplete(
		store,
		continuationGroupTestConfig(),
		"",
		nil,
		continuationGroupSnapshotResult(store, work),
		nil,
	)
	if len(reconciled) != 1 || reconciled[0].ID != work.ID {
		t.Fatalf("reconciled = %v, want [%s]", reconciled, work.ID)
	}

	ready, err := store.Ready()
	if err != nil {
		t.Fatalf("Ready: %v", err)
	}
	if len(ready) != 1 || ready[0].ID != work.ID || ready[0].Metadata[beadmeta.RoutedToMetadataKey] != "worker" {
		t.Fatalf("ready routed work = %#v, want exactly %s routed to worker", ready, work.ID)
	}

	writer, ok := beads.ConditionalWriterFor(store)
	if !ok {
		t.Fatal("MemStore missing conditional writer")
	}
	inProgress, assignee := "in_progress", "worker-fresh"
	claim := beads.UpdateOpts{Status: &inProgress, Assignee: &assignee}
	if err := writer.UpdateIfMatch(work.ID, ready[0].Revision, claim); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := writer.UpdateIfMatch(work.ID, ready[0].Revision, claim); !beads.IsPreconditionFailed(err) {
		t.Fatalf("second claim with the same fence = %v, want precondition failure", err)
	}
	claimed, err := store.Get(work.ID)
	if err != nil {
		t.Fatalf("Get claimed work: %v", err)
	}
	if claimed.Status != "in_progress" || claimed.Assignee != assignee {
		t.Fatalf("claimed work status/assignee = %q/%q, want in_progress/%q", claimed.Status, claimed.Assignee, assignee)
	}
}

func TestClearOrphanedContinuationGroupCandidate_StaleRevisionDoesNotSplitGroup(t *testing.T) {
	store := beads.NewMemStore()
	work := createOpenContinuationGroupWork(t, store, "ga-work", "worker")
	plans, complete := planOrphanedContinuationGroupClears(
		store,
		continuationGroupTestConfig(),
		"",
		nil,
		continuationGroupSnapshotResult(store, work),
	)
	if !complete || len(plans) != 1 || len(plans[0].candidates) != 1 {
		t.Fatalf("plan = %v complete=%v, want one complete candidate", plans, complete)
	}
	changedTitle := "claimed concurrently"
	if err := store.Update(work.ID, beads.UpdateOpts{Title: &changedTitle}); err != nil {
		t.Fatalf("concurrent update: %v", err)
	}
	if clearOrphanedContinuationGroupCandidate(plans[0].candidates[0]) {
		t.Fatal("stale candidate cleared after its revision changed")
	}
	got, err := store.Get(work.ID)
	if err != nil {
		t.Fatalf("Get work: %v", err)
	}
	if got.Metadata[beadmeta.ContinuationGroupMetadataKey] != "review" {
		t.Fatalf("continuation group = %q, want preserved after stale fence", got.Metadata[beadmeta.ContinuationGroupMetadataKey])
	}
}

func TestContinuationGroupHasLiveSession_CatchesOwnerCreatedAfterDemandSnapshot(t *testing.T) {
	store := beads.NewMemStore()
	waiting := createOpenContinuationGroupWork(t, store, "ga-waiting", "reviewer")
	plans, complete := planOrphanedContinuationGroupClears(
		store,
		continuationGroupTestConfig(),
		"",
		nil,
		continuationGroupSnapshotResult(store, waiting),
	)
	if !complete || len(plans) != 1 {
		t.Fatalf("plan = %v complete=%v, want one orphan candidate group", plans, complete)
	}

	live := createOpenContinuationGroupWork(t, store, "ga-live", "worker")
	if err := store.Update(live.ID, beads.UpdateOpts{
		Status:   stringPtr("in_progress"),
		Assignee: stringPtr("worker-live"),
	}); err != nil {
		t.Fatalf("assign late live group member: %v", err)
	}
	liveSession := []session.Info{{
		ID:                  "session-live",
		Template:            "worker",
		SessionNameMetadata: "worker-live",
	}}
	groupLive, revalidationComplete := continuationGroupHasLiveSession(
		plans[0], continuationGroupTestConfig(), "", liveSession,
	)
	if !revalidationComplete || !groupLive {
		t.Fatalf("live=%v complete=%v, want late owner detected by targeted live revalidation", groupLive, revalidationComplete)
	}
}

func TestReleaseOrphanedPoolAssignmentsWhenSnapshotsComplete_PreservesGroupWithLiveSession(t *testing.T) {
	store := beads.NewMemStore()
	waiting := createOpenContinuationGroupWork(t, store, "ga-waiting", "reviewer")
	live := createOpenContinuationGroupWork(t, store, "ga-live", "worker")
	if err := store.Update(live.ID, beads.UpdateOpts{
		Status:   stringPtr("in_progress"),
		Assignee: stringPtr("worker-live"),
	}); err != nil {
		t.Fatalf("assign live group member: %v", err)
	}
	live, err := store.Get(live.ID)
	if err != nil {
		t.Fatalf("reload live group member: %v", err)
	}

	reconciled := releaseOrphanedPoolAssignmentsWhenSnapshotsComplete(
		store,
		continuationGroupTestConfig(),
		"",
		[]session.Info{{ID: "session-live", Template: "worker", SessionNameMetadata: "worker-live"}},
		DesiredStateResult{
			AssignedWorkBeads:                 []beads.Bead{live},
			AssignedWorkStores:                []beads.Store{store},
			AssignedWorkStoreRefs:             []string{""},
			OpenUnassignedRoutedWorkBeads:     []beads.Bead{waiting},
			OpenUnassignedRoutedWorkStores:    []beads.Store{store},
			OpenUnassignedRoutedWorkStoreRefs: []string{"city:test"},
		},
		nil,
	)
	if len(reconciled) != 0 {
		t.Fatalf("reconciled = %v, want live continuation group preserved", reconciled)
	}
	got, err := store.Get(waiting.ID)
	if err != nil {
		t.Fatalf("Get waiting member: %v", err)
	}
	if got.Metadata[beadmeta.ContinuationGroupMetadataKey] != "review" {
		t.Fatalf("waiting continuation group = %q, want preserved while sibling session is live", got.Metadata[beadmeta.ContinuationGroupMetadataKey])
	}
}

func TestReleaseOrphanedPoolAssignmentsWhenSnapshotsComplete_PartialSnapshotPreservesUnassignedGroup(t *testing.T) {
	store := beads.NewMemStore()
	waiting := createOpenContinuationGroupWork(t, store, "ga-waiting", "worker")

	reconciled := releaseOrphanedPoolAssignmentsWhenSnapshotsComplete(
		store,
		continuationGroupTestConfig(),
		"",
		nil,
		DesiredStateResult{StoreQueryPartial: true},
		nil,
	)
	if len(reconciled) != 0 {
		t.Fatalf("reconciled = %v, want none from partial snapshot", reconciled)
	}
	got, err := store.Get(waiting.ID)
	if err != nil {
		t.Fatalf("Get waiting member: %v", err)
	}
	if got.Metadata[beadmeta.ContinuationGroupMetadataKey] != "review" {
		t.Fatalf("continuation group = %q, want preserved on partial snapshot", got.Metadata[beadmeta.ContinuationGroupMetadataKey])
	}
}

func TestReleaseOrphanedPoolAssignmentsWhenSnapshotsComplete_MisalignedStoreSnapshotPreservesEveryGroup(t *testing.T) {
	store := beads.NewMemStore()
	waiting := createOpenContinuationGroupWork(t, store, "ga-city", "worker")

	reconciled := releaseOrphanedPoolAssignmentsWhenSnapshotsComplete(
		store,
		continuationGroupTestConfig(),
		"",
		nil,
		DesiredStateResult{
			OpenUnassignedRoutedWorkBeads:     []beads.Bead{waiting},
			OpenUnassignedRoutedWorkStoreRefs: []string{"city:test"},
		},
		nil,
	)
	if len(reconciled) != 0 {
		t.Fatalf("reconciled = %v, want none from an index-misaligned store snapshot", reconciled)
	}
	got, err := store.Get(waiting.ID)
	if err != nil {
		t.Fatalf("Get city member: %v", err)
	}
	if got.Metadata[beadmeta.ContinuationGroupMetadataKey] != "review" {
		t.Fatalf("city continuation group = %q, want preserved after malformed snapshot", got.Metadata[beadmeta.ContinuationGroupMetadataKey])
	}
}

func createOpenContinuationGroupWork(t *testing.T, store beads.Store, id, route string) beads.Bead {
	t.Helper()
	created, err := store.Create(beads.Bead{
		ID:     id,
		Title:  "continuation group work",
		Labels: []string{"priority:p1"},
		Metadata: map[string]string{
			beadmeta.RoutedToMetadataKey:          route,
			beadmeta.RootBeadIDMetadataKey:        "ga-root",
			beadmeta.ContinuationGroupMetadataKey: "review",
			"gc.session_affinity":                 "require",
			"custom":                              "preserve-me",
		},
		Dependencies: []beads.Dep{{IssueID: id, DependsOnID: "ga-blocker", Type: "blocks"}},
	})
	if err != nil {
		t.Fatalf("Create(%s): %v", id, err)
	}
	return created
}

func continuationGroupTestConfig() *config.City {
	return &config.City{Agents: []config.Agent{
		{Name: "worker", MinActiveSessions: intPtr(0), MaxActiveSessions: intPtr(2)},
		{Name: "reviewer", MinActiveSessions: intPtr(0), MaxActiveSessions: intPtr(2)},
		{Name: "worker", Dir: "repo", MinActiveSessions: intPtr(0), MaxActiveSessions: intPtr(2)},
	}}
}

func continuationGroupSnapshotResult(store beads.Store, work ...beads.Bead) DesiredStateResult {
	stores := make([]beads.Store, len(work))
	refs := make([]string, len(work))
	for i := range work {
		stores[i] = store
		refs[i] = "city:test"
	}
	return DesiredStateResult{
		OpenUnassignedRoutedWorkBeads:     work,
		OpenUnassignedRoutedWorkStores:    stores,
		OpenUnassignedRoutedWorkStoreRefs: refs,
	}
}

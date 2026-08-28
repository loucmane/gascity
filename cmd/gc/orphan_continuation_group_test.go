package main

import (
	"errors"
	"testing"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/session"
)

func TestReleaseOrphanedPoolAssignmentsWhenSnapshotsComplete_ClearsOpenUnassignedContinuationGroup(t *testing.T) {
	store := beads.NewMemStore()
	first := createOpenContinuationGroupWork(t, store, "ga-first", "worker", "ga-root")
	second := createOpenContinuationGroupWork(t, store, "ga-second", "reviewer", "ga-root")

	reconciled := releaseOrphanedPoolAssignmentsWhenSnapshotsComplete(
		store,
		continuationGroupTestConfig(),
		"",
		nil,
		DesiredStateResult{},
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

func TestReleaseOrphanedPoolAssignmentsWhenSnapshotsComplete_PreservesGroupWithLiveSession(t *testing.T) {
	store := beads.NewMemStore()
	waiting := createOpenContinuationGroupWork(t, store, "ga-waiting", "reviewer", "ga-root")
	live := createOpenContinuationGroupWork(t, store, "ga-live", "worker", "ga-root")
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
			AssignedWorkBeads:     []beads.Bead{live},
			AssignedWorkStores:    []beads.Store{store},
			AssignedWorkStoreRefs: []string{""},
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
	waiting := createOpenContinuationGroupWork(t, store, "ga-waiting", "worker", "ga-root")

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

func TestReleaseOrphanedPoolAssignmentsWhenSnapshotsComplete_StoreReadFailurePreservesEveryGroup(t *testing.T) {
	cityStore := beads.NewMemStore()
	waiting := createOpenContinuationGroupWork(t, cityStore, "ga-city", "worker", "ga-root")
	rigBacking := beads.NewMemStore()
	createOpenContinuationGroupWork(t, rigBacking, "repo-rig", "repo/worker", "repo-root")
	rigStore := continuationGroupListErrorStore{Store: rigBacking}
	cfg := continuationGroupTestConfig()
	cfg.Rigs = []config.Rig{{Name: "repo", Path: "/repo"}}

	reconciled := releaseOrphanedPoolAssignmentsWhenSnapshotsComplete(
		cityStore,
		cfg,
		"",
		nil,
		DesiredStateResult{},
		map[string]beads.Store{"repo": rigStore},
	)
	if len(reconciled) != 0 {
		t.Fatalf("reconciled = %v, want none when any candidate store read fails", reconciled)
	}
	got, err := cityStore.Get(waiting.ID)
	if err != nil {
		t.Fatalf("Get city member: %v", err)
	}
	if got.Metadata[beadmeta.ContinuationGroupMetadataKey] != "review" {
		t.Fatalf("city continuation group = %q, want preserved after rig-store read failure", got.Metadata[beadmeta.ContinuationGroupMetadataKey])
	}
}

func createOpenContinuationGroupWork(t *testing.T, store beads.Store, id, route, root string) beads.Bead {
	t.Helper()
	created, err := store.Create(beads.Bead{
		ID:     id,
		Title:  "continuation group work",
		Labels: []string{"priority:p1"},
		Metadata: map[string]string{
			beadmeta.RoutedToMetadataKey:          route,
			beadmeta.RootBeadIDMetadataKey:        root,
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

type continuationGroupListErrorStore struct {
	beads.Store
}

func (continuationGroupListErrorStore) List(beads.ListQuery) ([]beads.Bead, error) {
	return nil, errors.New("injected continuation-group list failure")
}

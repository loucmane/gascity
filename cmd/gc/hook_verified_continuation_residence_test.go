package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
)

func TestVerifiedContinuationMixedResidence(t *testing.T) {
	for _, classFirst := range []bool{true, false} {
		name := "work-first"
		if classFirst {
			name = "class-first"
		}
		t.Run(name, func(t *testing.T) {
			clearGCEnv(t)
			t.Setenv("GC_BEADS", "file")
			v, _ := newVerifiedHookFixture(t)
			v.cityPath = t.TempDir()
			if err := ensureScopedFileStoreLayout(v.cityPath); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(v.cityPath, "city.toml"), []byte("[workspace]\nname = \"test\"\n[beads]\nprovider = \"file\"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(v.cityPath, "rig")
			if err := ensurePersistedScopeLocalFileStore(dir); err != nil {
				t.Fatal(err)
			}
			v.AllowedWorkStores = []hookStore{{dir: dir}}
			store, err := openVerifiedHookWorkStore(context.Background(), nil, v, nil, dir, "")
			if err != nil {
				t.Fatal(err)
			}
			var rows []beads.Bead
			for range 4 {
				row, err := store.Create(beads.Bead{Title: "sibling", Type: "task", Metadata: map[string]string{
					"gc.root_bead_id": "root", "gc.continuation_group": "group", "gc.routed_to": "rig-a/worker",
				}})
				if err != nil {
					t.Fatal(err)
				}
				rows = append(rows, row)
			}
			class := &continuationClosingStore{MemStore: beads.NewMemStoreFrom(1, rows[:2], nil)}
			route := &hookClaimClassRoute{class: class, resident: map[string]bool{rows[0].ID: true, rows[1].ID: true}}
			siblings := slices.Clone(rows)
			if !classFirst {
				slices.Reverse(siblings)
			}
			opts := hookClaimOptions{Verified: v, Assignee: v.ID, RouteTargets: []string{"rig-a/worker"}}
			ops := hookClaimOps{ClassRoute: route, ListContinuation: func(context.Context, string, []string, string, string) ([]beads.Bead, error) {
				return siblings, nil
			}}
			assigned, err := preassignHookContinuationGroup(beads.Bead{ID: "claimed", Metadata: rows[0].Metadata}, opts, ops, dir)
			if err != nil || len(assigned) != len(rows) {
				t.Errorf("partial continuation pin: assigned=%v err=%v", assigned, err)
			}
			observer, err := openVerifiedHookWorkStore(context.Background(), nil, v, nil, dir, "")
			if err != nil {
				t.Fatal(err)
			}
			for i, row := range rows {
				workRow, err := observer.Get(row.ID)
				if err != nil {
					t.Fatal(err)
				}
				if i < 2 {
					if workRow.Assignee != "" || workRow.Revision != row.Revision {
						t.Errorf("stale work copy changed: %+v", workRow)
					}
					canonical, err := class.Get(row.ID)
					if err != nil || canonical.Assignee != continuationPinAssignee(opts) {
						t.Errorf("class sibling not pinned: %+v err=%v", canonical, err)
					}
				} else if workRow.Assignee != continuationPinAssignee(opts) {
					t.Errorf("work sibling not pinned: %+v", workRow)
				}
			}
			if class.closes != 0 {
				t.Fatal("closed borrowed class store")
			}
		})
	}
}

package main

import (
	"errors"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
)

type verifiedClosingStore struct {
	*beads.MemStore
	closes   int
	closeErr error
}

func (s *verifiedClosingStore) CloseStore() error {
	s.closes++
	return s.closeErr
}

func TestVerifiedWorkStoreOwnership(t *testing.T) {
	for _, operation := range []string{"fence", "restamp", "release"} {
		for _, outcome := range []string{"success", "missing", "close-error", "borrowed"} {
			t.Run(operation+"/"+outcome, func(t *testing.T) {
				v, _ := newVerifiedHookFixture(t)
				mem, row := verifiedWorkFixture(t, v)
				if err := mem.Update(row.ID, beads.UpdateOpts{Metadata: map[string]string{
					verifiedHookIncarnationKey: v.workIncarnation(), verifiedHookAttemptKey: "attempt",
				}}); err != nil {
					t.Fatal(err)
				}
				row, _ = mem.Get(row.ID)
				store := &verifiedClosingStore{MemStore: mem}
				wrapped := wrapStoreWithBeadPolicies(store, nil)
				installVerifiedWorkStore(t, v, wrapped)
				var route *hookClaimClassRoute
				switch outcome {
				case "missing":
					row.ID = "absent"
				case "close-error":
					store.closeErr = errors.New("close failed")
				case "borrowed":
					route = &hookClaimClassRoute{class: wrapped, resident: map[string]bool{row.ID: true}}
				}
				opts := hookClaimOptions{Verified: v, Assignee: v.assignee()}
				ops := hookClaimOps{ClassRoute: route}
				var err error
				switch operation {
				case "fence":
					_, err = fenceVerifiedHookWork(row, opts, ops, "")
				case "restamp":
					_, err = restampVerifiedHookWork(v, route, "", row.ID, v.assignee(), v.assignee())
				case "release":
					_, err = releaseVerifiedHookPending(row, opts, ops, "")
				}
				wantCloses := 1
				if outcome == "borrowed" {
					wantCloses = 0
				}
				if store.closes != wantCloses {
					t.Errorf("closed %d times, want %d", store.closes, wantCloses)
				}
				if outcome == "close-error" && !errors.Is(err, store.closeErr) {
					t.Errorf("lost close error: %v", err)
				}
				if outcome == "missing" && !errors.Is(err, beads.ErrNotFound) {
					t.Errorf("lost operation error: %v", err)
				}
				if outcome == "success" && err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

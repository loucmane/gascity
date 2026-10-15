package main

import (
	"bytes"
	"errors"
	"log"
	"strings"
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
				var closeLog bytes.Buffer
				oldLog := log.Writer()
				log.SetOutput(&closeLog)
				defer log.SetOutput(oldLog)
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
					var fenced beads.Bead
					fenced, err = fenceVerifiedHookWork(row, opts, ops, "")
					if err == nil && (fenced.Metadata[verifiedHookAttemptKey] == "" || fenced.Metadata[verifiedHookAttemptKey] == "attempt") {
						t.Fatal("successful fence did not return its landed attempt")
					}
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
				if outcome == "close-error" {
					if err != nil {
						t.Errorf("landed write changed to failure by close: %v", err)
					}
					if !strings.Contains(closeLog.String(), store.closeErr.Error()) {
						t.Errorf("close error not logged separately: %s", closeLog.String())
					}
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

func TestVerifiedReleaseCloseErrorEmitsClaimReleased(t *testing.T) {
	v, _ := newVerifiedHookFixture(t)
	mem, row := verifiedWorkFixture(t, v)
	if err := mem.Update(row.ID, beads.UpdateOpts{Metadata: map[string]string{
		verifiedHookIncarnationKey: v.workIncarnation(), verifiedHookAttemptKey: "attempt",
	}}); err != nil {
		t.Fatal(err)
	}
	row, _ = mem.Get(row.ID)
	store := &verifiedClosingStore{MemStore: mem, closeErr: errors.New("release close failed")}
	installVerifiedWorkStore(t, v, wrapStoreWithBeadPolicies(store, nil))
	var emitted []hookClaimReleaseRecord
	ops := hookClaimOps{EmitClaimReleased: func(record hookClaimReleaseRecord) {
		emitted = append(emitted, record)
	}}
	var stderr bytes.Buffer
	unwindUndeliveredHookClaim("fixture", "delivery failed", row,
		hookClaimOptions{Verified: v, Assignee: v.assignee()}, ops, "", &stderr)
	live, err := mem.Get(row.ID)
	if err != nil || live.Status != "open" || live.Assignee != "" {
		t.Fatalf("release did not land: %+v, %v", live, err)
	}
	if len(emitted) != 1 || emitted[0].BeadID != row.ID {
		t.Fatalf("landed release lost its event: %+v; %s", emitted, stderr.String())
	}
	if store.closes != 1 {
		t.Fatalf("closed %d times", store.closes)
	}
}

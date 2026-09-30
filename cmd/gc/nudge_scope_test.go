package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/nudgequeue"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/session"
	"github.com/gastownhall/gascity/internal/worker"
)

// A session-scoped delivery must not expire, repair, claim or prune another
// session's history, including unfenced and prior-epoch entries for its alias.
func TestNudgeSessionScopePreservesForeignHistory(t *testing.T) {
	for _, operation := range []string{"claim", "status"} {
		t.Run(operation, func(t *testing.T) {
			dir, target, foreign, store, before := scopedNudgeFixture(t)
			if operation == "claim" {
				claimed, err := claimDueQueuedNudgesForTarget(dir, target, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				if len(claimed) != 1 || claimed[0].ID != "owned" {
					t.Errorf("claimed IDs = %v; want only owned", queuedNudgeIDs(claimed))
				}
			} else {
				if _, _, _, err := listQueuedNudgesForTarget(dir, target, time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			assertForeignNudgesUnchanged(t, dir, foreign, store, before)
		})
	}
}

func TestNudgeSessionScopeAllTransitionsPreserveForeignHistory(t *testing.T) {
	for _, operation := range []string{"enqueue", "supersede", "ack", "release", "failure", "dead-letter", "rollback", "blocked", "withdraw", "missing-session"} {
		t.Run(operation, func(t *testing.T) {
			dir, target, foreign, store, before := scopedNudgeFixture(t)
			scope, err := target.queueScope()
			if err != nil {
				t.Fatal(err)
			}
			if operation == "supersede" {
				// Every entry shares a reference. Only the owned generation may
				// be superseded, even for the same agent and source.
				ref := &nudgeReference{Kind: "bead", ID: "shared-work"}
				if err := withNudgeQueueState(dir, func(s *nudgeQueueState) error {
					for _, bucket := range [][]queuedNudge{s.Pending, s.InFlight, s.Dead} {
						for i := range bucket {
							bucket[i].Reference = ref
						}
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				for _, bucket := range [][]queuedNudge{foreign.Pending, foreign.InFlight, foreign.Dead} {
					for i := range bucket {
						bucket[i].Reference = ref
					}
				}
			}
			if operation != "enqueue" && operation != "supersede" && operation != "missing-session" {
				if _, err := claimDueQueuedNudgesForTarget(dir, target, time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			// Deliberately include a foreign ID in mutator input: selection is
			// enforced by the typed transaction, not by trusting caller lists.
			ids := []string{"owned", "foreign-inflight", "foreign-expired"}
			switch operation {
			case "enqueue", "supersede":
				item := newQueuedNudgeWithOptions("worker", "next", "session", time.Now(), queuedNudgeOptions{ID: "new-owned", SessionID: target.sessionID, ContinuationEpoch: target.continuationEpoch})
				if operation == "supersede" {
					item.Reference = &nudgeReference{Kind: "bead", ID: "shared-work"}
				}
				err = enqueueQueuedNudgeWithStore(dir, beads.NudgesStore{Store: store}, item, scope)
			case "ack":
				err = ackQueuedNudges(dir, ids, scope)
			case "release":
				err = releaseQueuedNudgeClaims(dir, ids, scope)
			case "failure":
				err = recordQueuedNudgeFailureWithStore(dir, beads.NudgesStore{Store: store}, ids, errors.New("temporary"), time.Now(), scope)
			case "dead-letter":
				err = recordQueuedNudgeFailureWithStore(dir, beads.NudgesStore{Store: store}, ids, errNudgeSessionFenceMismatch, time.Now(), scope)
			case "blocked":
				err = terminalizeBlockedQueuedNudges(dir, map[string][]queuedNudge{"blocked": {{ID: "owned"}, {ID: "foreign-inflight"}}}, scope)
			case "withdraw":
				err = scope.WithdrawWaitNudges(store, dir, ids)
			case "rollback":
				state, loadErr := nudgequeue.LoadState(dir)
				if loadErr != nil {
					t.Fatal(loadErr)
				}
				for _, item := range state.InFlight {
					if item.ID == "owned" {
						err = rollbackQueuedNudge(dir, nudgequeue.NewScopedStore(beads.NudgesStore{Store: store}, scope), item, "wake failed", scope)
					}
				}
			case "missing-session":
				if !shouldKeepNudgePollerAlive(target, time.Time{}, time.Now()) {
					t.Fatal("owned pending item should keep startup grace alive")
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			assertForeignNudgesUnchanged(t, dir, foreign, store, before)
			state, err := nudgequeue.LoadState(dir)
			if err != nil {
				t.Fatal(err)
			}
			if operation == "enqueue" || operation == "supersede" {
				if !queuedNudgeExists(&state, "new-owned") {
					t.Fatal("positive enqueue missing")
				}
			}
			if operation == "ack" || operation == "blocked" || operation == "withdraw" {
				if queuedNudgeExists(&state, "owned") {
					t.Fatal("owned item was not terminalized")
				}
			}
			if operation == "dead-letter" || operation == "supersede" || operation == "rollback" {
				found := false
				for _, item := range state.Dead {
					if item.ID == "owned" {
						found = true
					}
				}
				if !found {
					t.Fatal("owned dead-letter transition missing")
				}
			}
		})
	}
}

func TestNudgeSessionScopeEnqueueRefusesForeignIDBeforeBeadWrite(t *testing.T) {
	for _, id := range []string{"foreign-expired", "foreign-inflight", "foreign-dead"} {
		t.Run(id, func(t *testing.T) {
			dir, target, foreign, store, before := scopedNudgeFixture(t)
			scope, _ := target.queueScope()
			item := newQueuedNudgeWithOptions("worker", "collision", "session", time.Now(), queuedNudgeOptions{ID: id, SessionID: target.sessionID, ContinuationEpoch: target.continuationEpoch})
			prior, err := store.List(beads.ListQuery{Label: "gc:nudge", IncludeClosed: true})
			if err != nil {
				t.Fatal(err)
			}
			if err := enqueueQueuedNudgeWithStore(dir, beads.NudgesStore{Store: store}, item, scope); err == nil {
				t.Fatal("cross-generation ID accepted")
			}
			after, err := store.List(beads.ListQuery{Label: "gc:nudge", IncludeClosed: true})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(prior, after) {
				t.Fatal("refused enqueue changed backing store")
			}
			assertForeignNudgesUnchanged(t, dir, foreign, store, before)
		})
	}
}

func TestNudgeSessionScopePollDeliveryAndGenerationNegatives(t *testing.T) {
	for _, mode := range []string{"current", "missing-id", "missing-epoch", "old-epoch", "other-session", "runtime-gone"} {
		t.Run(mode, func(t *testing.T) {
			dir, target, foreign, store, before := scopedNudgeFixture(t)
			target.sessionName = "sess-scoped"
			target.resolved = &config.ResolvedProvider{Name: "codex"}
			fake := runtime.NewFake()
			if err := fake.Start(context.Background(), target.sessionName, runtime.Config{}); err != nil {
				t.Fatal(err)
			}
			id, epoch := target.sessionID, target.continuationEpoch
			switch mode {
			case "missing-id":
				id = ""
			case "missing-epoch":
				epoch = ""
			case "old-epoch":
				epoch = "1"
			case "other-session":
				id = "another-session"
			}
			if err := fake.SetMeta(target.sessionName, "GC_SESSION_ID", id); err != nil {
				t.Fatal(err)
			}
			if err := fake.SetMeta(target.sessionName, "GC_CONTINUATION_EPOCH", epoch); err != nil {
				t.Fatal(err)
			}
			if mode == "runtime-gone" {
				if err := fake.Stop(target.sessionName); err != nil {
					t.Fatal(err)
				}
			}
			idle := time.Now().Add(-time.Minute)
			obs := worker.LiveObservation{Running: true, LastActivity: &idle}
			// No real session/process or model is launched. The existing fake
			// transport proves the production dispatch/ack path and receipt.
			delivered, err := tryDeliverQueuedNudgesByPoller(target, nil, nil, fake, time.Second, obs)
			if err != nil {
				t.Fatal(err)
			}
			if delivered != (mode == "current") {
				t.Fatalf("delivered=%v for %s", delivered, mode)
			}
			calls := fake.CountCalls("Nudge", target.sessionName)
			if mode == "current" {
				if calls != 1 {
					t.Fatalf("Nudge calls=%d", calls)
				}
				shadow, ok, err := nudgequeue.NewStore(beads.NudgesStore{Store: store}).FindIncludingTerminal("owned")
				if err != nil || !ok {
					t.Fatalf("receipt missing: %v", err)
				}
				if shadow.State != "injected" || shadow.CommitBoundary != "provider-nudge-return" || shadow.Open {
					t.Fatalf("bad durable receipt: %+v", shadow)
				}
			} else if calls != 0 {
				t.Fatalf("negative invoked transport %d times", calls)
			}
			assertForeignNudgesUnchanged(t, dir, foreign, store, before)
		})
	}
}

func TestNudgeSessionScopeConfigurationCannotDowngrade(t *testing.T) {
	for _, mode := range []string{"session-epoch", "global", "unknown", "missing"} {
		t.Run(mode, func(t *testing.T) {
			dir, target, foreign, store, before := scopedNudgeFixture(t)
			scope, _ := target.queueScope()
			if mode != "missing" {
				data := "[workspace]\nname = 'scoped-test'\n[session]\nnudge_queue_scope = '" + mode + "'\n"
				if err := os.WriteFile(filepath.Join(dir, "city.toml"), []byte(data), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			err := validateNudgePollScope(target, scope)
			if (err == nil) != (mode == "session-epoch") {
				t.Fatalf("validation for %s: %v", mode, err)
			}
			assertForeignNudgesUnchanged(t, dir, foreign, store, before)
		})
	}
}

func TestNudgeSessionScopeInvalidIdentityRefusesWithoutQueueMutation(t *testing.T) {
	for _, value := range []string{"session", "epoch", "mode"} {
		t.Run(value, func(t *testing.T) {
			dir, target, foreign, store, before := scopedNudgeFixture(t)
			switch value {
			case "session":
				target.sessionID = ""
			case "epoch":
				target.continuationEpoch = ""
			case "mode":
				target.cfg.Session.NudgeQueueScope = "typo"
			}
			if _, err := claimDueQueuedNudgesForTarget(dir, target, time.Now()); err == nil {
				t.Fatal("invalid scope accepted")
			}
			assertForeignNudgesUnchanged(t, dir, foreign, store, before)
		})
	}
}

func TestNudgeSessionScopeManagedWakePreservesOldEpochWait(t *testing.T) {
	dir, target, foreign, store, before := scopedNudgeFixture(t)
	// The queue fixture already contains a same-session old-generation item.
	// A wake cancels session waits across epochs and returns all their nudge
	// IDs. Implicit withdrawal must still retain the foreign queue/shadow.
	sessions := beads.NewMemStoreFrom(1, []beads.Bead{{ID: target.sessionID, Status: "open", Type: session.BeadType, Labels: []string{session.LabelSession}, Metadata: map[string]string{"state": "suspended", "continuation_epoch": "2"}}}, nil)
	wait := createTestWaitBeadForSession(t, sessions, target.sessionID, waitStatePending)
	if err := sessions.SetMetadataBatch(wait.ID, map[string]string{"nudge_id": "foreign-old-epoch", "registered_epoch": "1"}); err != nil {
		t.Fatal(err)
	}
	item := newQueuedNudgeWithOptions("worker", "new work", "session", time.Now(), queuedNudgeOptions{ID: "new-owned", SessionID: target.sessionID, ContinuationEpoch: target.continuationEpoch})
	scope, _ := target.queueScope()
	if err := enqueueQueuedNudgeWithStore(dir, beads.NudgesStore{Store: store}, item, scope); err != nil {
		t.Fatal(err)
	}
	if err := requestManagedNudgeWake(target, session.NewStore(beads.SessionStore{Store: sessions})); err != nil {
		t.Fatal(err)
	}
	afterWait, err := sessions.Get(wait.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterWait.Status != "closed" {
		t.Fatal("positive wake did not cancel the wait")
	}
	assertForeignNudgesUnchanged(t, dir, foreign, store, before)
}

func scopedNudgeFixture(t *testing.T) (string, nudgeTarget, nudgeQueueState, *beads.MemStore, []beads.Bead) {
	t.Helper()
	dir := t.TempDir()
	var cfg config.City
	if _, err := toml.Decode("[session]\nnudge_queue_scope = 'session-epoch'\n", &cfg); err != nil {
		t.Fatal(err)
	}
	target := nudgeTarget{cityPath: dir, cfg: &cfg, agent: config.Agent{Name: "worker"}, sessionID: "session-current", continuationEpoch: "2"}
	now := time.Now().UTC()
	item := func(id, sessionID, epoch string) queuedNudge {
		return newQueuedNudgeWithOptions("worker", id, "session", now.Add(-time.Minute), queuedNudgeOptions{ID: id, SessionID: sessionID, ContinuationEpoch: epoch})
	}
	expired := item("foreign-expired", "session-other", "2")
	expired.ExpiresAt = now.Add(-time.Minute)
	inFlight := item("foreign-inflight", "session-other", "2")
	inFlight.ClaimedAt = now.Add(-time.Hour)
	inFlight.LeaseUntil = now.Add(-time.Minute)
	dead := item("foreign-dead", "session-other", "2")
	dead.DeadAt = now.Add(-2 * defaultQueuedNudgeDeadRetention)
	dead.LastError = "failed"
	foreign := nudgeQueueState{
		Pending:  []queuedNudge{expired, item("foreign-old-epoch", target.sessionID, "1"), item("foreign-unfenced", "", "")},
		InFlight: []queuedNudge{inFlight}, Dead: []queuedNudge{dead},
	}
	store := beads.NewMemStore()
	previous := openNudgeBeadStore
	openNudgeBeadStore = func(string) beads.NudgesStore { return beads.NudgesStore{Store: store} }
	t.Cleanup(func() { openNudgeBeadStore = previous })
	front := nudgequeue.NewStore(beads.NudgesStore{Store: store})
	var before []beads.Bead
	for _, bucket := range [][]queuedNudge{foreign.Pending, foreign.InFlight, foreign.Dead} {
		for i := range bucket {
			id, _, err := front.Save(bucket[i])
			if err != nil {
				t.Fatal(err)
			}
			bucket[i].BeadID = id
			b, err := store.Get(id)
			if err != nil {
				t.Fatal(err)
			}
			before = append(before, b)
		}
	}
	nudgequeue.SortState(&foreign)
	owned := item("owned", target.sessionID, target.continuationEpoch)
	id, _, err := front.Save(owned)
	if err != nil {
		t.Fatal(err)
	}
	owned.BeadID = id
	if err := withNudgeQueueState(dir, func(state *nudgeQueueState) error {
		state.Pending = append(append([]queuedNudge(nil), foreign.Pending...), owned)
		state.InFlight = append([]queuedNudge(nil), foreign.InFlight...)
		state.Dead = append([]queuedNudge(nil), foreign.Dead...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return dir, target, foreign, store, before
}

func assertForeignNudgesUnchanged(t *testing.T, dir string, want nudgeQueueState, store *beads.MemStore, before []beads.Bead) {
	t.Helper()
	state, err := nudgequeue.LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	foreign := func(items []queuedNudge) []queuedNudge {
		var result []queuedNudge
		for _, item := range items {
			if item.ID != "owned" && item.ID != "new-owned" {
				result = append(result, item)
			}
		}
		return result
	}
	got := nudgeQueueState{Pending: foreign(state.Pending), InFlight: foreign(state.InFlight), Dead: foreign(state.Dead)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("foreign queue history changed\ngot: %+v\nwant: %+v", got, want)
	}
	for _, b := range before {
		after, err := store.Get(b.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(after, b) {
			t.Errorf("foreign backing bead %s changed", b.ID)
		}
	}
}

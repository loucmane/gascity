package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/session"
)

func TestPoolClaimGuardUnreadableCandidateKeepsHealthySibling(t *testing.T) {
	for _, failed := range []string{"session-a", "work-a"} {
		for _, preferred := range []bool{false, true} {
			t.Run(failed+map[bool]string{false: "/ordinary", true: "/preferred"}[preferred], func(t *testing.T) {
				bp, owner, now := claimGuardFixture(t, 5*time.Minute, 3)
				sibling := mustGetTestBead(t, bp.beadStore, owner.ID)
				sibling.ID = "session-b"
				sibling.CreatedAt = now
				sibling.Metadata["pool_slot"] = "2"
				sibling.Metadata["agent_name"] = "worker-2"
				sibling.Metadata["session_name"] = "worker-b"
				sibling.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey] = ""
				sibling, err := bp.beadStore.Create(sibling)
				if err != nil {
					t.Fatal(err)
				}
				store := &ownershipFailingGetStore{Store: bp.beadStore}
				store.arm(failed)
				bp.beadStore = store
				bp.sessionBeads = newSessionBeadSnapshot([]beads.Bead{owner, sibling})
				var first *session.Info
				if preferred {
					info := session.ReconcileRowsFromBeads([]beads.Bead{owner})[0].Info
					first = &info
				}
				info, _, plan, err := selectOrPlanPoolSessionBead(bp, &bp.city.Agents[0], "worker", first,
					SessionRequest{WorkBeadID: "work-b"}, now, map[string]bool{}, map[int]bool{})
				if err != nil || plan != nil || info.ID != sibling.ID {
					t.Fatalf("healthy sibling stalled: id=%q plan=%v err=%v", info.ID, plan, err)
				}
			})
		}
	}
}

func TestPoolClaimGuardWriteDiagnostics(t *testing.T) {
	for _, kind := range []string{"held", "unsupported", "precondition", "transport"} {
		t.Run(kind, func(t *testing.T) {
			bp, owner, _ := claimGuardFixture(t, 5*time.Minute, 2)
			mem := bp.beadStore.(*beads.MemStore)
			if kind != "held" {
				if err := mem.SetMetadata(owner.ID, beadmeta.CurrentClaimBeadIDMetadataKey, ""); err != nil {
					t.Fatal(err)
				}
			}
			switch kind {
			case "unsupported":
				bp.beadStore = struct{ beads.Store }{mem}
			case "precondition":
				bp.beadStore = &poolWriteErrorStore{MemStore: mem, err: &beads.PreconditionFailedError{ID: owner.ID}}
			case "transport":
				bp.beadStore = &poolWriteErrorStore{MemStore: mem, err: errors.New("transport unavailable")}
			}
			var stderr bytes.Buffer
			bp.stderr = &stderr
			for range 3 {
				if err := bp.poolClaims().write(owner.ID, "work-b", map[string]string{beadmeta.TriggerBeadIDMetadataKey: "work-b"}); err == nil {
					t.Fatal("write accepted")
				}
			}
			if kind == "precondition" {
				if stderr.Len() != 0 {
					t.Fatalf("contention was noisy: %s", &stderr)
				}
				return
			}
			needle := map[string]string{"held": "held claim", "unsupported": "conditional writes unsupported", "transport": "transport unavailable"}[kind]
			if strings.Count(stderr.String(), "\n") != 1 || !strings.Contains(stderr.String(), needle) {
				t.Fatalf("want one distinct %s diagnostic: %q", kind, stderr.String())
			}
		})
	}
}

type poolWriteErrorStore struct {
	*beads.MemStore
	err error
}

func (s *poolWriteErrorStore) UpdateIfMatch(string, int64, beads.UpdateOpts) error { return s.err }

func TestPoolClaimGuardInvalidatesOwnWriteMemo(t *testing.T) {
	bp, owner, _ := claimGuardFixture(t, 5*time.Minute, 2)
	if err := bp.beadStore.Close("work-a"); err != nil {
		t.Fatal(err)
	}
	g := bp.poolClaims()
	if _, conflict, err := g.check(owner.ID, "work-b", true); conflict || err != nil {
		t.Fatalf("released claim blocked selection: %v", err)
	}
	if err := g.write(owner.ID, "work-b", map[string]string{beadmeta.TriggerBeadIDMetadataKey: "work-b"}); err != nil {
		t.Fatal(err)
	}
	row, _, err := g.check(owner.ID, "work-b", true)
	if err != nil || row.Metadata[beadmeta.TriggerBeadIDMetadataKey] != "work-b" {
		t.Fatalf("own write left stale selection memo: %+v %v", row.Metadata, err)
	}
}

func TestPoolClaimGuardProductionChain(t *testing.T) {
	for _, operation := range []string{"bind", "idle-marker"} {
		t.Run(operation, func(t *testing.T) {
			bp, owner, now := claimGuardFixture(t, 5*time.Minute, 2)
			mem := bp.beadStore.(*beads.MemStore)
			if err := mem.Close("work-a"); err != nil {
				t.Fatal(err)
			}
			if err := mem.SetMetadataBatch(owner.ID, map[string]string{"instance_token": "fixture", "generation": "1"}); err != nil {
				t.Fatal(err)
			}
			raced := &reacquireBeforeBindStore{MemStore: mem}
			cache := beads.NewCachingStoreForTest(raced, nil)
			store := wrapStoreWithBeadPolicies(cache, bp.city)
			bp.beadStore = store
			front := session.NewStore(beads.SessionStore{Store: store})
			v, err := front.VerifySession(owner.ID, "fixture", 1)
			if err != nil {
				t.Fatal(err)
			}
			raced.beforeBind = func() {
				status := "in_progress"
				if err := mem.Update("work-a", beads.UpdateOpts{Status: &status}); err != nil {
					t.Fatal(err)
				}
				if err := front.CompareAndSetCurrentClaim(v, "work-a", "work-a"); err != nil {
					t.Fatal(err)
				}
			}
			info := bp.sessionBeads.OpenInfos()[0]
			write := func() bool {
				if operation == "idle-marker" {
					return writeIdleClaimMarker(store, &owner, "work-b", 1, now, io.Discard, bp.poolClaims())
				}
				_, err := bindPoolSessionTriggerBead(bp, &bp.city.Agents[0], "worker-1", info, SessionRequest{WorkBeadID: "work-b"})
				return err == nil
			}
			if write() {
				t.Error("production wrapper chain admitted a write after reacquisition")
			}
			assertClaimGuardOwner(t, mem, owner)
			// Successful writes must also traverse the chain and invalidate both
			// the cache and this tick's ownership memo before another selection.
			if err := store.Close("work-a"); err != nil {
				t.Fatal(err)
			}
			if !write() {
				t.Error("production wrapper chain refused released work")
			}
			row := mustGetTestBead(t, store, owner.ID)
			key := beadmeta.TriggerBeadIDMetadataKey
			if operation == "idle-marker" {
				key = idleClaimNudgeTriggerKey
			}
			if row.Metadata[key] != "work-b" {
				t.Fatal("cache did not expose successful conditional write")
			}
		})
	}
}

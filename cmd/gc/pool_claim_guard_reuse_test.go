package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/session"
)

func TestPoolClaimGuardStaleStampReuse(t *testing.T) {
	for _, mode := range []string{"finished", "successor", "one-shot-asleep", "reassigned"} {
		t.Run(mode, func(t *testing.T) {
			bp, owner, now := claimGuardFixture(t, 5*time.Minute, 1)
			if err := bp.beadStore.Close("work-a"); err != nil {
				t.Fatal(err)
			}
			if mode == "reassigned" {
				status, assignee := "in_progress", "different-session"
				if err := bp.beadStore.Update("work-a", beads.UpdateOpts{Status: &status, Assignee: &assignee}); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "one-shot-asleep" {
				bp.city.Agents[0].Lifecycle = config.AgentLifecycleOneShot
				if err := bp.beadStore.SetMetadataBatch(owner.ID, map[string]string{"state": "asleep", "sleep_reason": "idle"}); err != nil {
					t.Fatal(err)
				}
			}
			owner = mustGetTestBead(t, bp.beadStore, owner.ID)
			bp.sessionBeads = newSessionBeadSnapshot([]beads.Bead{owner})
			var preferred *session.Info
			req := SessionRequest{Tier: "new", WorkBeadID: "work-b"}
			if mode == "successor" {
				assignee := owner.ID
				if err := bp.beadStore.Update("work-b", beads.UpdateOpts{Assignee: &assignee}); err != nil {
					t.Fatal(err)
				}
				bp.assignedWorkBeads = []beads.Bead{mustGetTestBead(t, bp.beadStore, "work-b")}
				states := computePoolDesiredStatesAt(bp.city, bp.assignedWorkBeads, bp.sessionBeads.OpenInfos(), nil, nil, now, nil)
				if len(states) != 1 || len(states[0].Requests) != 1 {
					t.Fatalf("successor requests: %+v", states)
				}
				req = states[0].Requests[0]
				if req.Tier != "resume" || req.SessionBeadID != owner.ID {
					t.Fatalf("not own assigned resume: %+v", req)
				}
				info := bp.sessionBeads.OpenInfos()[0]
				preferred = &info
			}
			info, _, plan, err := selectOrPlanPoolSessionBead(bp, &bp.city.Agents[0], "worker", preferred, req, now, map[string]bool{}, map[int]bool{})
			if err != nil || plan != nil || info.ID != owner.ID {
				t.Fatalf("reuse=%s plan=%v err=%v", info.ID, plan, err)
			}
			if _, err := bindPoolSessionTriggerBead(bp, &bp.city.Agents[0], "worker", info, req); err != nil {
				t.Fatal(err)
			}
			after := mustGetTestBead(t, bp.beadStore, owner.ID)
			if after.Metadata[beadmeta.TriggerBeadIDMetadataKey] != "work-b" {
				t.Error("successor was not bound")
			}
			if after.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey] != "work-a" {
				t.Error("reuse cleared the stale stamp")
			}
		})
	}
}

func TestPoolClaimGuardMemoizesSelectionAndSkipsEmptyBind(t *testing.T) {
	bp, owner, now := claimGuardFixture(t, 5*time.Minute, 1)
	store := &claimAfterReadStore{MemStore: bp.beadStore.(*beads.MemStore), sessionID: owner.ID}
	bp.beadStore = store
	for range 100 {
		_, _, _, err := selectOrPlanPoolSessionBead(bp, &bp.city.Agents[0], "worker", nil, SessionRequest{WorkBeadID: "work-b"}, now, map[string]bool{}, map[int]bool{})
		if !errors.Is(err, errPoolSessionNameUnavailable) {
			t.Fatalf("full pool error=%v", err)
		}
	}
	if store.reads != 1 || store.workReads != 1 {
		t.Errorf("selection reads session=%d work=%d, want 1 each", store.reads, store.workReads)
	}
	info := bp.sessionBeads.OpenInfos()[0]
	if _, err := bindPoolSessionTriggerBead(bp, &bp.city.Agents[0], "worker", info, SessionRequest{WorkBeadID: "work-a"}); err != nil {
		t.Fatal(err)
	}
	if store.reads != 1 {
		t.Errorf("empty bind read session: %d", store.reads)
	}
}

func TestPoolClaimGuardReadErrorDefersAndLogsOnce(t *testing.T) {
	for _, failingID := range []string{"session-a", "work-a"} {
		t.Run(failingID, func(t *testing.T) {
			bp, owner, now := claimGuardFixture(t, 5*time.Minute, 2)
			store := &ownershipFailingGetStore{Store: bp.beadStore}
			store.arm(failingID)
			bp.beadStore = store
			var stderr bytes.Buffer
			bp.stderr = &stderr
			desired := map[string]TemplateParams{}
			realizePoolDesiredSessionsAt(bp, &bp.city.Agents[0], PoolDesiredState{Requests: []SessionRequest{{WorkBeadID: "work-b"}, {WorkBeadID: "work-b"}}}, now, desired, &stderr)
			if len(desired) != 0 {
				t.Error("unreadable ownership planned capacity")
			}
			sp := runtime.NewFake()
			if err := sp.Start(context.Background(), "worker-a", runtime.Config{}); err != nil {
				t.Fatal(err)
			}
			owner.Metadata[beadmeta.TriggerBeadIDMetadataKey] = "work-b"
			var stdout bytes.Buffer
			nudgeStalledPoolClaims(sp, bp.city, store, []beads.Bead{owner}, []beads.Bead{{ID: "work-b", Status: "open"}}, nil, now, &stdout, bp.poolClaims())
			if strings.Count(stderr.String(), "\n") != 1 || stdout.Len() != 0 {
				t.Fatalf("stderr=%q stdout=%q", stderr.String(), stdout.String())
			}
			if sp.CountCalls("Nudge", "worker-a") != 0 {
				t.Error("unreadable ownership nudged")
			}
		})
	}
}

func TestPoolClaimGuardUnsupportedWritesRefuse(t *testing.T) {
	bp, owner, now := claimGuardFixture(t, 5*time.Minute, 2)
	if err := bp.beadStore.SetMetadata(owner.ID, beadmeta.CurrentClaimBeadIDMetadataKey, ""); err != nil {
		t.Fatal(err)
	}
	owner = mustGetTestBead(t, bp.beadStore, owner.ID)
	// Embedding only Store deliberately hides the optional conditional writer.
	bp.beadStore = struct{ beads.Store }{bp.beadStore}
	info := bp.sessionBeads.OpenInfos()[0]
	if _, err := bindPoolSessionTriggerBead(bp, &bp.city.Agents[0], "worker", info, SessionRequest{WorkBeadID: "work-b"}); err == nil {
		t.Error("unfenced bind accepted")
	}
	if writeIdleClaimMarker(bp.beadStore, &owner, "work-b", 1, now, io.Discard) {
		t.Error("unfenced marker accepted")
	}
	assertClaimGuardOwner(t, bp.beadStore, owner)
}

func TestPoolClaimGuardSelectionStopsAtFirstUsableCandidate(t *testing.T) {
	bp, owner, now := claimGuardFixture(t, 5*time.Minute, 3)
	if err := bp.beadStore.SetMetadata(owner.ID, beadmeta.CurrentClaimBeadIDMetadataKey, ""); err != nil {
		t.Fatal(err)
	}
	sibling := mustGetTestBead(t, bp.beadStore, owner.ID)
	sibling.CreatedAt = now
	sibling.Metadata["pool_slot"] = "2"
	sibling.Metadata["agent_name"] = "worker-2"
	sibling.Metadata["session_name"] = "worker-b"
	sibling, err := bp.beadStore.Create(sibling)
	if err != nil {
		t.Fatal(err)
	}
	store := &claimAfterReadStore{MemStore: bp.beadStore.(*beads.MemStore), sessionID: sibling.ID}
	bp.beadStore = store
	bp.sessionBeads = newSessionBeadSnapshot([]beads.Bead{owner, sibling})
	info, _, plan, err := selectOrPlanPoolSessionBead(bp, &bp.city.Agents[0], "worker", nil, SessionRequest{WorkBeadID: "work-b"}, now, map[string]bool{}, map[int]bool{})
	if err != nil || plan != nil || info.ID != owner.ID {
		t.Fatalf("first usable candidate=%s plan=%v err=%v", info.ID, plan, err)
	}
	if store.reads != 0 {
		t.Errorf("read unused sibling %d times", store.reads)
	}
}

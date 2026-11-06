package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/session"
)

func claimGuardFixture(t *testing.T, age time.Duration, capacity int) (*agentBuildParams, beads.Bead, time.Time) {
	t.Helper()
	now := time.Date(2026, 11, 5, 19, 0, 0, 0, time.UTC)
	root := t.TempDir()
	cfg := &config.City{Agents: []config.Agent{{
		Name: "worker", StartCommand: "true", MinActiveSessions: intPtr(0), MaxActiveSessions: intPtr(capacity),
	}}}
	owner := protectedPoolSessionBeadAt("session-a", now.Add(-age))
	owner.Labels = []string{sessionBeadLabel, "template:worker"}
	owner.Metadata["template"] = "worker"
	owner.Metadata["session_name"] = "worker-a"
	owner.Metadata["agent_name"] = "worker-1"
	owner.Metadata["pool_slot"] = "1"
	if capacity == 1 {
		owner.Metadata["agent_name"] = "worker"
		owner.Metadata["alias"] = "worker"
		delete(owner.Metadata, "pool_slot")
	}
	owner.Metadata[beadmeta.TriggerBeadIDMetadataKey] = "work-a"
	owner.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey] = "work-a"
	owner.Metadata[beadmeta.WorkDirMetadataKey] = filepath.Join(root, "work-a")
	owner.Metadata[beadmeta.LegacyWorkDirMetadataKey] = filepath.Join(root, "work-a")
	workA := workBead("work-a", "worker", owner.ID, "in_progress", 0)
	workA.Metadata[beadmeta.SessionIDMetadataKey] = owner.ID
	workB := workBead("work-b", "worker", "", "open", 0)
	store := beads.NewMemStoreFrom(10, []beads.Bead{owner, workA, workB}, nil)
	// NewMemStoreFrom only copies the bead slice; Get detaches metadata so
	// writes cannot mutate the expected pre-bind owner used by assertions.
	owner = mustGetTestBead(t, store, owner.ID)
	bp := newAgentBuildParams("test-city", root, cfg, runtime.NewFake(), now, store, io.Discard)
	bp.attemptWorkStore = func(_, _ string, use func(beads.Store) error) error { return use(store) }
	bp.sessionBeads = newSessionBeadSnapshot([]beads.Bead{owner})
	// ga-gwav: the live session owns A, but the cached assigned-work view
	// omitted it in both journal incidents. No processing marker is present.
	bp.assignedWorkBeads = nil
	return bp, owner, now
}

func assertClaimGuardOwner(t *testing.T, store beads.Store, before beads.Bead) {
	t.Helper()
	after := mustGetTestBead(t, store, before.ID)
	for _, key := range []string{
		beadmeta.TriggerBeadIDMetadataKey, beadmeta.WorkDirMetadataKey,
		beadmeta.LegacyWorkDirMetadataKey, beadmeta.CurrentClaimBeadIDMetadataKey,
	} {
		if after.Metadata[key] != before.Metadata[key] {
			t.Errorf("owner %s changed from %q to %q", key, before.Metadata[key], after.Metadata[key])
		}
	}
	if got := after.Metadata[idleClaimNudgeTriggerKey]; got == "work-b" {
		t.Error("claim holder acquired idle-claim marker for work-b")
	}
}

func testPoolClaimGuardRoute(t *testing.T, age time.Duration) {
	t.Helper()
	for _, capacity := range []int{2, 1} {
		name := "room"
		if capacity == 1 {
			name = "at-cap"
		}
		t.Run(name, func(t *testing.T) {
			bp, owner, now := claimGuardFixture(t, age, capacity)
			states := computePoolDesiredStatesAt(bp.city, nil, bp.sessionBeads.OpenInfos(),
				map[string]int{"worker": 1}, map[string]scaleCheckDemand{
					"worker": {Count: 1, WorkBeadIDs: []string{"work-b"}},
				}, now, nil)
			if len(states) != 1 || len(states[0].Requests) != 1 {
				t.Fatalf("requests = %+v, want one request for B", states)
			}
			req := states[0].Requests[0]
			var preferred *session.Info
			if req.SessionBeadID != "" {
				info := bp.sessionBeads.OpenInfos()[0]
				preferred = &info
			}
			if (preferred != nil) != (age < postCreateProtectionTimeout) {
				t.Fatalf("wrong allocation route: preferred=%v age=%s", preferred != nil, age)
			}
			chosen, _, plan, err := selectOrPlanPoolSessionBead(bp, &bp.city.Agents[0], "worker",
				preferred, req, now, map[string]bool{}, map[int]bool{})
			if err == nil && plan != nil {
				chosen, err = executePlannedPoolSessionBeadCreate(bp, &bp.city.Agents[0], "worker", *plan)
			}
			if err == nil {
				_, err = bindPoolSessionTriggerBead(bp, &bp.city.Agents[0], "worker", chosen, req)
			}
			if capacity == 1 {
				if !errors.Is(err, errPoolSessionNameUnavailable) {
					t.Errorf("at-cap selection = %q, err=%v; want wait for capacity", chosen.ID, err)
				}
			} else if err != nil || chosen.ID == "" || chosen.ID == owner.ID {
				t.Errorf("selection = %q, err=%v; want another session for B", chosen.ID, err)
			} else if got := mustGetTestBead(t, bp.beadStore, chosen.ID).Metadata[beadmeta.TriggerBeadIDMetadataKey]; got != "work-b" {
				t.Errorf("new session trigger = %q, want work-b", got)
			}
			sp := runtime.NewFake()
			if err := sp.Start(context.Background(), owner.Metadata["session_name"], runtime.Config{}); err != nil {
				t.Fatal(err)
			}
			nudgeStalledPoolClaims(sp, bp.city, bp.beadStore,
				[]beads.Bead{mustGetTestBead(t, bp.beadStore, owner.ID)},
				[]beads.Bead{mustGetTestBead(t, bp.beadStore, "work-b")}, nil, now, io.Discard)
			assertClaimGuardOwner(t, bp.beadStore, owner)
		})
	}
}

func TestPoolClaimGuardProtectedCapacity(t *testing.T) {
	testPoolClaimGuardRoute(t, 30*time.Second)
}

func TestPoolClaimGuardOrdinaryReuse(t *testing.T) {
	testPoolClaimGuardRoute(t, 5*time.Minute)
}

func TestPoolClaimGuardBindingRechecksLiveClaim(t *testing.T) {
	for _, readFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "claim-after-selection", true: "read-failure"}[readFailure], func(t *testing.T) {
			bp, owner, now := claimGuardFixture(t, 5*time.Minute, 2)
			if err := bp.beadStore.SetMetadata(owner.ID, beadmeta.CurrentClaimBeadIDMetadataKey, ""); err != nil {
				t.Fatal(err)
			}
			reads := &ownershipFailingGetStore{Store: bp.beadStore}
			cache := beads.NewCachingStoreForTest(reads, nil)
			if err := cache.Prime(context.Background()); err != nil {
				t.Fatal(err)
			}
			bp.beadStore = cache
			req := SessionRequest{Tier: "new", WorkBeadID: "work-b"}
			info, _, plan, err := selectOrPlanPoolSessionBead(bp, &bp.city.Agents[0], "worker", nil,
				req, now, map[string]bool{}, map[int]bool{})
			if err != nil || plan != nil || info.ID != owner.ID {
				t.Fatalf("unclaimed selection = %q, plan=%v, err=%v", info.ID, plan, err)
			}
			if err := reads.SetMetadata(owner.ID, beadmeta.CurrentClaimBeadIDMetadataKey, "work-a"); err != nil {
				t.Fatal(err)
			}
			if readFailure {
				reads.arm(owner.ID)
			}
			bound, err := bindPoolSessionTriggerBead(bp, &bp.city.Agents[0], "worker", info, req)
			if err == nil {
				t.Error("binding accepted a conflicting or unreadable live claim")
			}
			if !reflect.DeepEqual(bound, info) {
				t.Error("refused binding changed returned session info")
			}
			assertClaimGuardOwner(t, reads.Store, owner)
		})
	}
}

func TestPoolClaimGuardIdleBackstop(t *testing.T) {
	for _, attempts := range []int{-1, 0, 1} {
		t.Run(map[int]string{-1: "observe", 0: "after-grace", 1: "retry"}[attempts], func(t *testing.T) {
			bp, owner, now := claimGuardFixture(t, 5*time.Minute, 2)
			owner.Metadata[beadmeta.TriggerBeadIDMetadataKey] = "work-b"
			if err := bp.beadStore.SetMetadataBatch(owner.ID, owner.Metadata); err != nil {
				t.Fatal(err)
			}
			if attempts >= 0 {
				// Seed the pre-existing crossed marker directly: the guarded
				// writer must refuse to create it once A has a live claim.
				if err := bp.beadStore.SetMetadataBatch(owner.ID, map[string]string{
					idleClaimNudgeTriggerKey: "work-b",
					idleClaimNudgeCountKey:   strconv.Itoa(attempts),
					idleClaimNudgeAtKey:      now.Add(-time.Hour).Format(time.RFC3339),
				}); err != nil {
					t.Fatal(err)
				}
				owner = mustGetTestBead(t, bp.beadStore, owner.ID)
			}
			// Even a stale session snapshot must not authorize an observation
			// or delivery while the live session bead records another claim.
			delete(owner.Metadata, beadmeta.CurrentClaimBeadIDMetadataKey)
			sp := runtime.NewFake()
			if err := sp.Start(context.Background(), "worker-a", runtime.Config{}); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			nudgeStalledPoolClaims(sp, bp.city, bp.beadStore, []beads.Bead{owner},
				[]beads.Bead{{ID: "work-b", Status: "open"}}, nil, now, &out)
			if got := sp.CountCalls("Nudge", "worker-a"); got != 0 {
				t.Errorf("nudges = %d, want none for conflicting claim", got)
			}
			after := mustGetTestBead(t, bp.beadStore, owner.ID)
			if after.Metadata[idleClaimNudgeTriggerKey] != "" {
				t.Error("conflicting claim left an idle marker for B")
			}
			if after.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey] != "work-a" {
				t.Error("backstop changed the claim")
			}
		})
	}
}

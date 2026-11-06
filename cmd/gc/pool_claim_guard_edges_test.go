package main

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/session"
)

func TestPoolClaimGuardSelectionControls(t *testing.T) {
	for _, concrete := range []bool{false, true} {
		for _, tc := range []struct {
			name, claim, target string
			closeClaim          bool
			wantReuse           bool
		}{
			{name: "same-claim", claim: "work-a", target: "work-a", wantReuse: true},
			{name: "empty-claim", target: "work-b", wantReuse: true},
			{name: "different-claim", claim: "work-a", target: "work-b"},
			{name: "anonymous-demand", claim: "work-a"},
			{name: "closed-work-stale-stamp", claim: "work-a", target: "work-b", closeClaim: true, wantReuse: true},
			{name: "missing-work-stale-stamp", claim: "missing", target: "work-b", wantReuse: true},
		} {
			name := "ordinary/"
			if concrete {
				name = "preferred/"
			}
			t.Run(name+tc.name, func(t *testing.T) {
				bp, owner, now := claimGuardFixture(t, 5*time.Minute, 2)
				if err := bp.beadStore.SetMetadata(owner.ID, beadmeta.CurrentClaimBeadIDMetadataKey, tc.claim); err != nil {
					t.Fatal(err)
				}
				if tc.closeClaim {
					if err := bp.beadStore.Close("work-a"); err != nil {
						t.Fatal(err)
					}
				}
				var preferred *session.Info
				req := SessionRequest{WorkBeadID: tc.target, Tier: "new"}
				if concrete {
					info := bp.sessionBeads.OpenInfos()[0]
					preferred = &info
					req.SessionBeadID = info.ID
					req.Tier = "resume"
				}
				info, _, plan, err := selectOrPlanPoolSessionBead(bp, &bp.city.Agents[0], "worker", preferred,
					req, now, map[string]bool{}, map[int]bool{})
				if err != nil {
					t.Fatal(err)
				}
				if tc.wantReuse {
					if plan != nil || info.ID != owner.ID {
						t.Fatalf("reuse = %q, plan=%v; want original session", info.ID, plan)
					}
					if _, err := bindPoolSessionTriggerBead(bp, &bp.city.Agents[0], "worker", info, req); err != nil {
						t.Fatalf("legitimate bind: %v", err)
					}
				} else if plan == nil || info.ID != "" {
					t.Errorf("reuse = %q, plan=%v; want a fresh session", info.ID, plan)
				}
			})
		}
	}
}

func TestPoolClaimGuardReleaseAllowsReuse(t *testing.T) {
	bp, owner, now := claimGuardFixture(t, 5*time.Minute, 2)
	req := SessionRequest{WorkBeadID: "work-b", Tier: "new"}
	if err := clearSessionCurrentClaim(bp.beadStore, owner.ID); err != nil {
		t.Fatal(err)
	}
	info, _, plan, err := selectOrPlanPoolSessionBead(bp, &bp.city.Agents[0], "worker", nil,
		req, now, map[string]bool{}, map[int]bool{})
	if err != nil || plan != nil || info.ID != owner.ID {
		t.Fatalf("released selection = %q, plan=%v, err=%v", info.ID, plan, err)
	}
}

func TestPoolClaimGuardSelectionBypassesCache(t *testing.T) {
	bp, owner, now := claimGuardFixture(t, 5*time.Minute, 2)
	backing := bp.beadStore
	if err := backing.SetMetadata(owner.ID, beadmeta.CurrentClaimBeadIDMetadataKey, ""); err != nil {
		t.Fatal(err)
	}
	cache := beads.NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := backing.SetMetadata(owner.ID, beadmeta.CurrentClaimBeadIDMetadataKey, "work-a"); err != nil {
		t.Fatal(err)
	}
	// A class wrapper must not hide the cache's authoritative read handle.
	bp.beadStore = beads.SessionStore{Store: cache}
	info, _, plan, err := selectOrPlanPoolSessionBead(bp, &bp.city.Agents[0], "worker", nil,
		SessionRequest{WorkBeadID: "work-b"}, now, map[string]bool{}, map[int]bool{})
	if err != nil || plan == nil || info.ID != "" {
		t.Fatalf("stale-cache selection = %q, plan=%v, err=%v; want fresh capacity", info.ID, plan, err)
	}
}

func TestPoolClaimGuardMarkerRechecksBeforeWrite(t *testing.T) {
	bp, owner, now := claimGuardFixture(t, 5*time.Minute, 2)
	// Reservation is also the final gate before runtime delivery. A claim
	// arriving after target resolution must prevent both observation and retry.
	for _, attempts := range []int{0, 1} {
		if writeIdleClaimMarker(bp.beadStore, &owner, "work-b", attempts, now, io.Discard) {
			t.Errorf("marker write accepted conflicting claim at attempt %d", attempts)
		}
	}
	assertClaimGuardOwner(t, bp.beadStore, owner)
}

func TestPoolClaimGuardReusesAvailableSibling(t *testing.T) {
	bp, owner, now := claimGuardFixture(t, 5*time.Minute, 2)
	sibling := mustGetTestBead(t, bp.beadStore, owner.ID)
	sibling.ID = "session-b"
	sibling.Metadata["session_name"] = "worker-b"
	sibling.Metadata["agent_name"] = "worker-2"
	sibling.Metadata["pool_slot"] = "2"
	sibling.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey] = ""
	var err error
	if sibling, err = bp.beadStore.Create(sibling); err != nil {
		t.Fatal(err)
	}
	bp.sessionBeads = newSessionBeadSnapshot([]beads.Bead{owner, sibling})
	info, _, plan, err := selectOrPlanPoolSessionBead(bp, &bp.city.Agents[0], "worker", nil,
		SessionRequest{WorkBeadID: "work-b"}, now, map[string]bool{}, map[int]bool{})
	if err != nil || plan != nil || info.ID != sibling.ID {
		t.Fatalf("available sibling selection = %q, plan=%v, err=%v", info.ID, plan, err)
	}
	assertClaimGuardOwner(t, bp.beadStore, owner)
}

func TestPoolClaimGuardReadFailureSkipsReuseAndIdleMarker(t *testing.T) {
	bp, owner, now := claimGuardFixture(t, 5*time.Minute, 2)
	reads := &ownershipFailingGetStore{Store: bp.beadStore}
	reads.arm(owner.ID)
	bp.beadStore = reads
	var out bytes.Buffer
	bp.stderr = &out
	info, _, plan, err := selectOrPlanPoolSessionBead(bp, &bp.city.Agents[0], "worker", nil,
		SessionRequest{WorkBeadID: "work-b"}, now, map[string]bool{}, map[int]bool{})
	if err == nil || plan != nil || info.ID != "" {
		t.Fatalf("unreadable claim selection = %q, plan=%v, err=%v", info.ID, plan, err)
	}
	if out.Len() == 0 {
		t.Error("selection suppressed claim read failure")
	}
	sp := runtime.NewFake()
	if err := sp.Start(context.Background(), "worker-a", runtime.Config{}); err != nil {
		t.Fatal(err)
	}
	owner.Metadata[beadmeta.TriggerBeadIDMetadataKey] = "work-b"
	out.Reset()
	nudgeStalledPoolClaims(sp, bp.city, reads, []beads.Bead{owner},
		[]beads.Bead{{ID: "work-b", Status: "open"}}, nil, now, &out, &poolClaimGuard{store: reads, cfg: bp.city, stderr: &out})
	if out.Len() == 0 || sp.CountCalls("Nudge", "worker-a") != 0 {
		t.Error("backstop must report read failure and withhold delivery")
	}
	if got := mustGetTestBead(t, reads.Store, owner.ID).Metadata[idleClaimNudgeTriggerKey]; got != "" {
		t.Errorf("marker = %q after claim read failure", got)
	}
}

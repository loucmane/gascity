package main

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/runtime"
)

// Stamp after returning the observed row, so only a revision-fenced mutation
// can detect the intervening claim. MemStore supplies the real CAS semantics.
type claimAfterReadStore struct {
	*beads.MemStore
	sessionID          string
	stampOnRead, reads int
	workReads          int
}

func (s *claimAfterReadStore) Get(id string) (beads.Bead, error) {
	b, err := s.MemStore.Get(id)
	if id == "work-a" {
		s.workReads++
	}
	if err == nil && id == s.sessionID {
		s.reads++
		if s.reads == s.stampOnRead {
			err = s.SetMetadata(id, beadmeta.CurrentClaimBeadIDMetadataKey, "work-a")
		}
	}
	return b, err
}

func TestPoolClaimGuardBindingRace(t *testing.T) {
	bp, owner, _ := claimGuardFixture(t, 5*time.Minute, 2)
	store := &claimAfterReadStore{MemStore: bp.beadStore.(*beads.MemStore), sessionID: owner.ID, stampOnRead: 1}
	if err := store.SetMetadata(owner.ID, beadmeta.CurrentClaimBeadIDMetadataKey, ""); err != nil {
		t.Fatal(err)
	}
	bp.beadStore = store
	info := bp.sessionBeads.OpenInfos()[0]
	bound, err := bindPoolSessionTriggerBead(bp, &bp.city.Agents[0], "worker-1", info, SessionRequest{Tier: "new", WorkBeadID: "work-b"})
	if err == nil {
		t.Error("bind accepted claim committed after its read")
	}
	if bound.TriggerBeadID != info.TriggerBeadID {
		t.Error("refused bind changed returned trigger")
	}
	assertClaimGuardOwner(t, store.MemStore, owner)
}

func TestPoolClaimGuardIdleMarkerRace(t *testing.T) {
	for _, attempts := range []int{-1, 0, 1} {
		t.Run(map[int]string{-1: "observation", 0: "reservation", 1: "retry"}[attempts], func(t *testing.T) {
			bp, owner, now := claimGuardFixture(t, 5*time.Minute, 2)
			store := &claimAfterReadStore{MemStore: bp.beadStore.(*beads.MemStore), sessionID: owner.ID, stampOnRead: 2}
			if err := store.SetMetadataBatch(owner.ID, map[string]string{
				beadmeta.CurrentClaimBeadIDMetadataKey: "", beadmeta.TriggerBeadIDMetadataKey: "work-b",
			}); err != nil {
				t.Fatal(err)
			}
			owner = mustGetTestBead(t, store.MemStore, owner.ID)
			if attempts >= 0 {
				if !writeIdleClaimMarker(store.MemStore, &owner, "work-b", attempts, now.Add(-time.Hour), io.Discard) {
					t.Fatal("seed marker")
				}
			}
			before := mustGetTestBead(t, store.MemStore, owner.ID)
			sp := runtime.NewFake()
			if err := sp.Start(context.Background(), "worker-a", runtime.Config{}); err != nil {
				t.Fatal(err)
			}
			nudgeStalledPoolClaims(sp, bp.city, store, []beads.Bead{owner}, []beads.Bead{{ID: "work-b", Status: "open"}}, nil, now, io.Discard)
			if store.reads < 2 {
				t.Fatal("did not reach marker read")
			}
			if sp.CountCalls("Nudge", "worker-a") != 0 {
				t.Error("nudged after concurrent claim")
			}
			after := mustGetTestBead(t, store.MemStore, owner.ID)
			for _, key := range []string{idleClaimNudgeTriggerKey, idleClaimNudgeCountKey, idleClaimNudgeAtKey} {
				if after.Metadata[key] != before.Metadata[key] {
					t.Errorf("racing marker wrote %s", key)
				}
			}
			if after.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey] != "work-a" {
				t.Error("claim lost")
			}
		})
	}
}

func TestPoolClaimGuardRealizationRaceLeavesNothingDesired(t *testing.T) {
	bp, owner, now := claimGuardFixture(t, 5*time.Minute, 2)
	store := &claimAfterReadStore{MemStore: bp.beadStore.(*beads.MemStore), sessionID: owner.ID, stampOnRead: 2}
	if err := store.SetMetadata(owner.ID, beadmeta.CurrentClaimBeadIDMetadataKey, ""); err != nil {
		t.Fatal(err)
	}
	bp.beadStore = store
	states := computePoolDesiredStatesAt(bp.city, nil, bp.sessionBeads.OpenInfos(), map[string]int{"worker": 1}, map[string]scaleCheckDemand{"worker": {Count: 1, WorkBeadIDs: []string{"work-b"}}}, now, nil)
	desired := map[string]TemplateParams{}
	realizePoolDesiredSessionsAt(bp, &bp.city.Agents[0], states[0], now, desired, io.Discard)
	if store.reads < 2 {
		t.Fatal("did not reach bind read")
	}
	if len(desired) != 0 {
		t.Errorf("conflict left %d desired sessions", len(desired))
	}
	assertClaimGuardOwner(t, store.MemStore, owner)
}

type claimDuringIdleWait struct {
	*runtime.Fake
	claim func() error
}

func (s *claimDuringIdleWait) WaitForIdle(context.Context, string, time.Duration) error {
	return s.claim()
}

func TestPoolClaimGuardWarmNudgeRechecksAfterWait(t *testing.T) {
	bp, owner, _ := claimGuardFixture(t, 5*time.Minute, 2)
	if err := bp.beadStore.SetMetadataBatch(owner.ID, map[string]string{
		beadmeta.CurrentClaimBeadIDMetadataKey: "", beadmeta.TriggerBeadIDMetadataKey: "work-b",
	}); err != nil {
		t.Fatal(err)
	}
	owner = mustGetTestBead(t, bp.beadStore, owner.ID)
	sp := &claimDuringIdleWait{Fake: runtime.NewFake(), claim: func() error {
		return bp.beadStore.SetMetadata(owner.ID, beadmeta.CurrentClaimBeadIDMetadataKey, "work-a")
	}}
	probe := buildWarmClaimTriggerProbe(bp.beadStore.Get, io.Discard, bp.poolClaims())
	deliverWarmBindClaimNudge(context.Background(), sp, bp.beadStore, &owner, "claim work", probe)
	if sp.CountCalls("Nudge", "worker-a") != 0 {
		t.Error("warm nudge delivered after claim during wait")
	}
	if mustGetTestBead(t, bp.beadStore, owner.ID).Metadata[warmBindNudgedForTriggerKey] != "" {
		t.Error("refused warm nudge marked delivered")
	}
}

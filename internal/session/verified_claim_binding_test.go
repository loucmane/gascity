package session

import (
	"errors"
	"testing"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
)

func TestVerifiedClaimBindingFence(t *testing.T) {
	for _, race := range []bool{false, true} {
		t.Run(map[bool]string{false: "binding-before-read", true: "binding-before-CAS"}[race], func(t *testing.T) {
			_, mem, v := verifiedClaimFixture(t)
			bind := func() {
				if err := mem.SetMetadata(v.ID, beadmeta.TriggerBeadIDMetadataKey, "other"); err != nil {
					t.Fatal(err)
				}
			}
			raced := &claimRaceStore{MemStore: mem}
			if race {
				raced.beforeWrite = bind
			} else {
				bind()
			}
			s := NewStore(beads.SessionStore{Store: raced})
			if err := s.CompareAndSetCurrentClaim(v, "", "work"); err == nil {
				t.Error("stamp accepted incompatible binding")
			}
			row, _ := mem.Get(v.ID)
			if row.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey] != "" {
				t.Error("refused stamp published work")
			}
			if raced.writes > 1 {
				t.Error("retry failed to recheck binding")
			}
		})
	}
}

type claimContentionStore struct {
	*beads.MemStore
	writes int
}

func (s *claimContentionStore) UpdateIfMatch(id string, _ int64, _ beads.UpdateOpts) error {
	s.writes++
	return &beads.PreconditionFailedError{ID: id}
}

func TestVerifiedClaimSameIDContentionIsBounded(t *testing.T) {
	_, mem, v := verifiedClaimFixture(t)
	if err := mem.SetMetadata(v.ID, beadmeta.CurrentClaimBeadIDMetadataKey, "work"); err != nil {
		t.Fatal(err)
	}
	raced := &claimContentionStore{MemStore: mem}
	s := NewStore(beads.SessionStore{Store: raced})
	if err := s.CompareAndSetCurrentClaim(v, "work", "work"); !errors.Is(err, ErrClaimPrecondition) || raced.writes != 4 {
		t.Fatalf("same-ID contention: writes=%d err=%v, want four attempts", raced.writes, err)
	}
}

func TestVerifiedClaimSameIDInvalidatesBindingRevision(t *testing.T) {
	s, mem, v := verifiedClaimFixture(t)
	if err := mem.SetMetadata(v.ID, beadmeta.CurrentClaimBeadIDMetadataKey, "work"); err != nil {
		t.Fatal(err)
	}
	before, _ := mem.Get(v.ID)
	if err := s.CompareAndSetCurrentClaim(v, "work", "work"); err != nil {
		t.Fatal(err)
	}
	first, _ := mem.Get(v.ID)
	if err := s.CompareAndSetCurrentClaim(v, "work", "work"); err != nil {
		t.Fatal(err)
	}
	second, _ := mem.Get(v.ID)
	if first.Metadata["current_claim_stamp"] == "" || first.Metadata["current_claim_stamp"] == second.Metadata["current_claim_stamp"] {
		t.Error("same-ID publication did not change metadata; a no-op backend could preserve its revision")
	}
	if err := mem.UpdateIfMatch(v.ID, before.Revision, beads.UpdateOpts{Metadata: map[string]string{
		beadmeta.TriggerBeadIDMetadataKey: "other",
	}}); !beads.IsPreconditionFailed(err) {
		t.Fatalf("same-ID reacquisition left bind revision usable: %v", err)
	}
}

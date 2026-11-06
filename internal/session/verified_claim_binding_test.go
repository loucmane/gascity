package session

import (
	"errors"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
)

func managedClaimWork(dir string) beads.Bead {
	return beads.Bead{ID: "work", Metadata: map[string]string{
		beadmeta.WorkDirMetadataKey: dir, beadmeta.WorkBranchMetadataKey: "branch",
		beadmeta.WorktreeRepoMetadataKey: "/repo", beadmeta.WorktreeRootMetadataKey: "/trees",
		beadmeta.WorktreeBaseRefMetadataKey: "main", beadmeta.WorktreeBaseSHAMetadataKey: strings.Repeat("a", 40),
		beadmeta.WorktreeCreatorMetadataKey: "gc-sling", beadmeta.WorktreeOwnerMetadataKey: "gc-sling",
		beadmeta.WorktreeGenerationMetadataKey: "1", beadmeta.WorktreeLifecycleMetadataKey: "active",
	}}
}

func TestVerifiedClaimRound4Compatibility(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  map[string]string
		work beads.Bead
		want bool
	}{
		{"different-trigger-same-tree", map[string]string{"gc.trigger_bead_id": "other"}, managedClaimWork("/a"), true},
		{"work-dir-only", nil, beads.Bead{ID: "work", Metadata: map[string]string{beadmeta.WorkDirMetadataKey: "/b"}}, true},
		{"legacy-work-dir-only", nil, beads.Bead{ID: "work", Metadata: map[string]string{beadmeta.LegacyWorkDirMetadataKey: "/b"}}, true},
		{"non-pool", map[string]string{"pool_managed": "false"}, managedClaimWork("/b"), true},
		{"managed-different-tree", nil, managedClaimWork("/b"), false},
		{"managed-relative-paths", map[string]string{"work_dir": "relative"}, managedClaimWork("relative"), false},
		{"managed-unknown-session-dir", map[string]string{"work_dir": ""}, managedClaimWork("/a"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, mem, v := verifiedClaimFixture(t)
			meta := map[string]string{"pool_managed": "true", "work_dir": "/a"}
			for k, value := range tc.row {
				meta[k] = value
			}
			if err := mem.SetMetadataBatch(v.ID, meta); err != nil {
				t.Fatal(err)
			}
			_, err := s.CompareAndSetCurrentClaimForWork(v, "", tc.work)
			if (err == nil) != tc.want {
				t.Fatalf("publication error=%v, want compatible=%v", err, tc.want)
			}
		})
	}
}

func TestVerifiedClaimBindingFence(t *testing.T) {
	for _, race := range []bool{false, true} {
		t.Run(map[bool]string{false: "binding-before-read", true: "binding-before-CAS"}[race], func(t *testing.T) {
			_, mem, v := verifiedClaimFixture(t)
			bind := func() {
				if err := mem.SetMetadataBatch(v.ID, map[string]string{"pool_managed": "true", "work_dir": "/b", beadmeta.TriggerBeadIDMetadataKey: "other"}); err != nil {
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
			if _, err := s.CompareAndSetCurrentClaimForWork(v, "", managedClaimWork("/a")); err == nil {
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

func TestVerifiedClaimClearPublicationRechecksStamp(t *testing.T) {
	for _, race := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-read", true: "before-CAS"}[race], func(t *testing.T) {
			s, mem, v := verifiedClaimFixture(t)
			stamp, err := s.CompareAndSetCurrentClaimForWork(v, "", beads.Bead{ID: "work"})
			if err != nil {
				t.Fatal(err)
			}
			var nextStamp string
			publish := func() {
				var err error
				nextStamp, err = s.CompareAndSetCurrentClaimForWork(v, "work", beads.Bead{ID: "work"})
				if err != nil {
					t.Fatal(err)
				}
			}
			raced := &claimRaceStore{MemStore: mem}
			if race {
				raced.beforeWrite = publish
			} else {
				publish()
			}
			front := NewStore(beads.SessionStore{Store: raced})
			if err := front.ClearPublishedClaim(v, "work", stamp); !errors.Is(err, ErrClaimPrecondition) {
				t.Fatalf("old compensation error=%v", err)
			}
			if err := front.ClearPublishedClaim(v, "work", nextStamp); err != nil {
				t.Fatal(err)
			}
			row, err := mem.Get(v.ID)
			if err != nil || row.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey] != "" {
				t.Fatalf("current publication did not clear: %v", err)
			}
		})
	}
}

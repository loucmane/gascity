package session

import (
	"errors"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
)

func verifiedClaimFixture(t *testing.T) (*Store, *beads.MemStore, VerifiedSession) {
	t.Helper()
	b := sessionBeadFixture("s-own", "open", map[string]string{
		"state": "active", "instance_token": "fixture-token", "generation": "7",
		"session_name": "runtime-own", "template": "rig/worker", "alias": "rig/worker-1",
	})
	b.Revision = 1
	mem := beads.NewMemStoreFrom(1, []beads.Bead{b}, nil)
	s := NewStore(beads.SessionStore{Store: mem})
	v, err := s.VerifySession(b.ID, "fixture-token", 7)
	if err != nil {
		t.Fatal(err)
	}
	return s, mem, v
}

func TestVerifiedClaimStampAndClear(t *testing.T) {
	s, mem, v := verifiedClaimFixture(t)
	if err := s.CompareAndSetCurrentClaim(v, "", "work-1"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.CurrentClaimVerified(v); err != nil || got != "work-1" {
		t.Fatalf("claim = %q, %v", got, err)
	}
	if err := s.CompareAndSetCurrentClaim(v, "other", ""); !errors.Is(err, ErrClaimPrecondition) {
		t.Fatalf("wrong prior: %v", err)
	}
	if err := s.CompareAndSetCurrentClaim(v, "work-1", ""); err != nil {
		t.Fatal(err)
	}
	b, _ := mem.Get(v.ID)
	if b.Metadata["instance_token"] != "fixture-token" || b.Metadata["generation"] != "7" || b.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey] != "" {
		t.Fatal("stamp changed identity or clear failed")
	}
}

func TestVerifiedClaimRejectsChangedIncarnationAndClosedRows(t *testing.T) {
	for _, key := range []string{"instance_token", "generation", "session_name", "template", "alias", "state", "closed", "type"} {
		t.Run(key, func(t *testing.T) {
			s, mem, v := verifiedClaimFixture(t)
			if err := s.CompareAndSetCurrentClaim(v, "", "work-1"); err != nil {
				t.Fatal(err)
			}
			var err error
			switch key {
			case "closed":
				err = mem.Close(v.ID)
			case "type":
				typ := "task"
				err = mem.Update(v.ID, beads.UpdateOpts{Type: &typ})
			default:
				err = mem.SetMetadata(v.ID, key, "successor")
			}
			if err != nil {
				t.Fatal(err)
			}
			before, _ := mem.Get(v.ID)
			for _, next := range []string{"work-2", ""} {
				if err := s.CompareAndSetCurrentClaim(v, "work-1", next); !errors.Is(err, ErrStaleSession) {
					t.Fatalf("next %q: %v", next, err)
				}
			}
			if _, err := s.CurrentClaimVerified(v); !errors.Is(err, ErrStaleSession) {
				t.Fatalf("current: %v", err)
			}
			after, _ := mem.Get(v.ID)
			if after.Revision != before.Revision {
				t.Fatal("mutated successor")
			}
		})
	}
}

type claimRaceStore struct {
	*beads.MemStore
	beforeWrite func()
	writes      int
}

func (s *claimRaceStore) UpdateIfMatch(id string, rev int64, opts beads.UpdateOpts) error {
	s.writes++
	if s.beforeWrite != nil {
		f := s.beforeWrite
		s.beforeWrite = nil
		f()
	}
	return s.MemStore.UpdateIfMatch(id, rev, opts)
}

func TestVerifiedClaimCASRetryRechecksEveryPrecondition(t *testing.T) {
	for _, change := range []string{"unrelated", "generation", "instance_token", "prior", "closed"} {
		t.Run(change, func(t *testing.T) {
			_, mem, v := verifiedClaimFixture(t)
			raced := &claimRaceStore{MemStore: mem}
			raced.beforeWrite = func() {
				key, value := change, "changed"
				if change == "prior" {
					key = beadmeta.CurrentClaimBeadIDMetadataKey
				}
				if change == "closed" {
					if err := mem.Close(v.ID); err != nil {
						t.Fatal(err)
					}
					return
				}
				if err := mem.SetMetadata(v.ID, key, value); err != nil {
					t.Fatal(err)
				}
			}
			s := NewStore(beads.SessionStore{Store: raced})
			err := s.CompareAndSetCurrentClaim(v, "", "work-1")
			b, _ := mem.Get(v.ID)
			if change == "unrelated" {
				if err != nil || b.Metadata["unrelated"] != "changed" || b.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey] != "work-1" {
					t.Fatalf("unrelated merge: %v", err)
				}
			} else {
				if err == nil || b.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey] == "work-1" || raced.writes != 1 {
					t.Fatalf("successor write: %v, attempts %d", err, raced.writes)
				}
			}
		})
	}
}

// Embedding only Store deliberately hides the optional conditional capability.
type claimNoCASStore struct{ beads.Store }

func TestVerifiedClaimNoAtomicBackendFailsClosed(t *testing.T) {
	_, mem, v := verifiedClaimFixture(t)
	s := NewStore(beads.SessionStore{Store: claimNoCASStore{mem}})
	if err := s.CompareAndSetCurrentClaim(v, "", "work-1"); !beads.IsConditionalWriteUnsupported(err) {
		t.Fatalf("unsupported backend: %v", err)
	}
	b, _ := mem.Get(v.ID)
	if b.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey] != "" {
		t.Fatal("unconditional fallback")
	}
}

func TestVerifiedClaimNewIncarnationCannotReadPriorStamp(t *testing.T) {
	s, _, v := verifiedClaimFixture(t)
	if err := s.CompareAndSetCurrentClaim(v, "", "old-work"); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyPatch(v.ID, PreWakePatch(PreWakePatchInput{Generation: 8, InstanceToken: "new-fixture", Now: time.Unix(1, 0)})); err != nil {
		t.Fatal(err)
	}
	next, err := s.VerifySession(v.ID, "new-fixture", 8)
	if err != nil {
		t.Fatal(err)
	}
	if claim, err := s.CurrentClaimVerified(next); err != nil || claim != "" {
		t.Fatalf("new incarnation inherited claim %q: %v", claim, err)
	}
}

func TestVerifiedClaimSQLiteInitialZeroRevisionIsStillAtomic(t *testing.T) {
	store, err := beads.OpenSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closer, ok := store.(interface{ CloseStore() error }); ok {
			_ = closer.CloseStore()
		}
	})
	b, err := store.Create(beads.Bead{Title: "session", Type: BeadType, Metadata: map[string]string{
		"state": string(StateActive), "instance_token": "sqlite-fixture", "generation": "1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if b.Revision != 0 {
		t.Fatalf("fixture revision = %d, want initial SQLite revision 0", b.Revision)
	}
	front := NewStore(beads.SessionStore{Store: store})
	v, err := front.VerifySession(b.ID, "sqlite-fixture", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := front.CompareAndSetCurrentClaim(v, "", "work"); err != nil {
		t.Fatal(err)
	}
	if err := front.CompareAndSetCurrentClaim(v, "work", ""); err != nil {
		t.Fatal(err)
	}
	current, err := store.Get(b.ID)
	if err != nil || current.Revision != 2 || current.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey] != "" {
		t.Fatalf("conditional stamp/clear = %+v, %v", current, err)
	}
}

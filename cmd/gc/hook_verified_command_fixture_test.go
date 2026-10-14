package main

import (
	"context"
	"fmt"
	"maps"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
)

// The command fixtures' shell spies deliberately model the pinned bd CLI,
// which has no revision CAS. Pair their claim/actor protocol with MemStore's
// real revision semantics for the managed work fence. This is TEST wiring,
// never an emulation used by production: unsupported production backends must
// still refuse, as TestVerifiedHookWorkFenceRequiresAtomicBackendAndCanonicalOwner proves.
func installCommandVerifiedWorkStore(t *testing.T) {
	t.Helper()
	old := openVerifiedHookWorkStore
	t.Cleanup(func() { openVerifiedHookWorkStore = old })
	stores := map[string]*commandWorkCASFixture{}
	openVerifiedHookWorkStore = func(v *VerifiedSession, route *hookClaimClassRoute, dir, id string) (beads.Store, error) {
		if route != nil && route.knownResident(id) {
			return route.class, nil
		}
		allowed := false
		var legEnv []string
		for _, leg := range v.AllowedWorkStores {
			if leg.dir == dir {
				allowed = true
				legEnv = leg.env
			}
		}
		if !allowed {
			t.Fatalf("foreign work store %q", dir)
		}
		key := dir + "/" + id
		if existing := stores[key]; existing != nil {
			return existing, nil
		}
		backing := hookClaimBdStoreContext(context.Background(), dir, legEnv, v.assignee())
		row, err := backing.Get(id)
		if err != nil {
			return nil, err
		}
		row.Revision = 1
		store := &commandWorkCASFixture{MemStore: beads.NewMemStoreFrom(1, []beads.Bead{row}, nil), backing: backing}
		stores[key] = store
		return store, nil
	}
}

type commandWorkCASFixture struct {
	*beads.MemStore
	backing *beads.BdStore
}

func (s *commandWorkCASFixture) Get(id string) (beads.Bead, error) {
	row, err := s.backing.Get(id)
	if err != nil {
		return beads.Bead{}, err
	}
	current, err := s.MemStore.Get(id)
	if err != nil {
		return beads.Bead{}, err
	}
	if row.Status != current.Status || row.Assignee != current.Assignee {
		if err := s.Update(id, beads.UpdateOpts{Status: &row.Status, Assignee: &row.Assignee}); err != nil {
			return beads.Bead{}, err
		}
		current, err = s.MemStore.Get(id)
		if err != nil {
			return beads.Bead{}, err
		}
	}
	if current.Metadata == nil {
		current.Metadata = map[string]string{}
	}
	maps.Copy(current.Metadata, row.Metadata)
	return current, nil
}

func (s *commandWorkCASFixture) UpdateIfMatch(id string, revision int64, patch beads.UpdateOpts) error {
	current, err := s.MemStore.Get(id)
	if err != nil {
		return err
	}
	if current.Revision != revision {
		return &beads.PreconditionFailedError{ID: id}
	}
	if patch.Assignee != nil {
		moved, err := s.backing.TransferIfCurrent(id, current.Assignee, *patch.Assignee)
		if err != nil {
			return err
		}
		if !moved {
			return &beads.PreconditionFailedError{ID: id}
		}
	}
	if len(patch.Metadata) > 0 {
		if err := s.backing.Update(id, beads.UpdateOpts{Metadata: patch.Metadata}); err != nil {
			return fmt.Errorf("fixture metadata write: %w", err)
		}
	}
	return s.MemStore.UpdateIfMatch(id, revision, patch)
}

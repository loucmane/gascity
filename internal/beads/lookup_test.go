package beads

import (
	"errors"
	"testing"
)

func TestLookupByIDNativeExactBothTiers(t *testing.T) {
	store, native, bd := newSplitFixture(t)
	for _, row := range []Bead{
		{ID: "prx-1234", Type: "session", Status: "closed"},
		{ID: "prx-wisp-5678", Type: "session", Ephemeral: true},
		{ID: "alias-shaped-id", Type: "session"},
	} {
		writer := newNativeDoltStoreForTest(native.storage)
		created, err := writer.Create(row)
		if err != nil {
			t.Fatal(err)
		}
		got, err := LookupByID(store, row.ID)
		if err != nil || got.ID != created.ID || got.Ephemeral != created.Ephemeral || got.Status != created.Status {
			t.Fatalf("lookup %q = %+v, %v; want %+v", row.ID, got, err, created)
		}
	}
	if _, err := LookupByID(store, "missing-name"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if calls := bd.took(); len(calls) != 0 {
		t.Fatalf("native lookups reached bd: %v", calls)
	}
	cache := NewCachingStoreForTest(store, nil)
	if err := cache.Prime(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"prx-1234", "prx-wisp-5678", "alias-shaped-id"} {
		if got, err := LookupByID(cache, id); err != nil || got.ID != id {
			t.Fatalf("cached lookup %q = %+v, %v", id, got, err)
		}
	}
	if _, err := LookupByID(cache, "missing-name"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if calls := bd.took(); len(calls) != 0 {
		t.Fatalf("cached identity reads reached bd: %v", calls)
	}
}

func TestLookupByIDErrorsAreNotAbsence(t *testing.T) {
	store, _, bd := newSplitFixture(t)
	failure := errors.New("infrastructure not found")
	for _, err := range []error{failure, &PartialResultError{Op: "lookup", Err: failure}} {
		wrapped := &lookupListErrorStore{ProxiedStore: store, err: err}
		if _, got := LookupByID(wrapped, "alias"); !errors.Is(got, failure) || errors.Is(got, ErrNotFound) || IsPartialResult(got) != IsPartialResult(err) {
			t.Fatalf("lookup error = %v, want %v", got, err)
		}
	}
	if calls := bd.took(); len(calls) != 0 {
		t.Fatalf("failed lookups reached bd: %v", calls)
	}
}

func TestLookupByIDKeepsPlainAndDemotedGet(t *testing.T) {
	failure := errors.New("plain store unavailable")
	plain := &lookupGetErrorStore{err: failure}
	if _, err := LookupByID(plain, "name"); !errors.Is(err, failure) {
		t.Fatalf("plain lookup = %v, want original Get error", err)
	}
	store, _, bd := newSplitFixture(t)
	store.standDown(NewSchemaSkewVerdictError(ProxiedSkewLaneMain, ProxiedSkewDirAhead, "test"))
	if _, err := LookupByID(store, "missing-name"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("demoted lookup = %v", err)
	}
	if calls := bd.took(); len(calls) != 1 || calls[0] != "Get" {
		t.Fatalf("demoted calls = %v, want [Get]", calls)
	}
}

type lookupGetErrorStore struct {
	Store
	err error
}

func (s *lookupGetErrorStore) Get(string) (Bead, error) { return Bead{}, s.err }

type lookupListErrorStore struct {
	*ProxiedStore
	err error
}

func (s *lookupListErrorStore) List(ListQuery) ([]Bead, error) {
	return []Bead{{ID: "alias"}}, s.err
}

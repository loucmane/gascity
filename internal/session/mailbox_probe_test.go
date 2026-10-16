package session

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
)

// Mailbox probes must narrow both tiers before metadata filtering and must not
// hydrate dependencies that the address projection cannot use.
func TestMailboxMetadataProbesAreSessionBounded(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(fmt.Sprint(closed), func(t *testing.T) {
			rec := &mailboxProbeStore{Store: beads.NewMemStore()}
			_, err := NewStore(beads.SessionStore{Store: rec}).ResolveMailboxAddress("rig/missing", closed)
			if !errors.Is(err, ErrSessionNotFound) {
				t.Fatalf("resolve missing mailbox: %v", err)
			}
			wantStatus := ""
			if closed {
				wantStatus = "closed"
			}
			seen := make(map[string]int)
			for _, q := range rec.queries {
				if q.Type != BeadType && q.Label != LabelSession {
					t.Errorf("unrestricted mailbox metadata probe: %+v", q)
				}
				if !q.SkipDependencies || q.TierMode != beads.TierBoth || q.Status != wantStatus || q.Limit != 0 {
					t.Errorf("mailbox probe lost hydration/tier/status/completeness contract: %+v", q)
				}
				for k, v := range q.Metadata {
					if v != "rig/missing" {
						t.Errorf("metadata selector = %q", v)
					}
					seen[k+":"+q.Type+":"+q.Label]++
				}
			}
			want := map[string]int{"alias:session:": 1, "alias::gc:session": 1, "session_name:session:": 1, "session_name::gc:session": 1}
			if !reflect.DeepEqual(seen, want) {
				t.Errorf("probe coverage = %v, want %v", seen, want)
			}
		})
	}
}

type mailboxProbeStore struct {
	beads.Store
	queries []beads.ListQuery
}

func (s *mailboxProbeStore) List(q beads.ListQuery) ([]beads.Bead, error) {
	s.queries = append(s.queries, q)
	rows, err := s.Store.List(q)
	if q.SkipDependencies {
		for i := range rows {
			rows[i].Dependencies = nil
			rows[i].ParentID = ""
		}
	}
	return rows, err
}

// Compare the full Info and error contract, plus persisted repairs, to the
// unrestricted metadata algorithm on the same mixed corpus.
func TestMailboxMetadataProbesMatchUnrestrictedResolution(t *testing.T) {
	corpus := []beads.Bead{
		{ID: "typed", Type: BeadType, Status: "open", Metadata: map[string]string{"alias": "rig/typed", "session_name": "runtime-typed", "alias_history": "past"}},
		{ID: "repair", Labels: []string{LabelSession}, Status: "open", Metadata: map[string]string{"alias": "rig/repair", "session_name": "runtime-repair"}},
		{ID: "dual", Type: BeadType, Labels: []string{LabelSession}, Status: "open", Metadata: map[string]string{"alias": "rig/dual", "session_name": "rig/dual"}},
		{ID: "task", Type: "task", Labels: []string{LabelSession}, Status: "open", Metadata: map[string]string{"alias": "rig/typed", "session_name": "runtime-repair"}},
		{ID: "empty", Status: "open", Metadata: map[string]string{"alias": "rig/typed"}},
		{ID: "closed", Type: BeadType, Status: "closed", Metadata: map[string]string{"alias": "rig/typed", "session_name": "retired"}},
		{ID: "closed-repair", Labels: []string{LabelSession}, Status: "closed", Metadata: map[string]string{"alias": "rig/closed-repair"}},
		{ID: "busy", Type: BeadType, Status: "in_progress", Metadata: map[string]string{"alias": "rig/busy"}},
		{ID: "wisp", Type: BeadType, Status: "open", Ephemeral: true, Metadata: map[string]string{"alias": "rig/wisp"}},
		{ID: "no-history", Labels: []string{LabelSession}, Status: "open", NoHistory: true, Metadata: map[string]string{"alias": "rig/no-history"}},
		{ID: "ambiguous-a", Type: BeadType, Status: "open", Metadata: map[string]string{"alias": "rig/ambiguous"}},
		{ID: "ambiguous-b", Labels: []string{LabelSession}, Status: "open", Metadata: map[string]string{"session_name": "rig/ambiguous"}},
		{ID: "id-collision", Type: BeadType, Status: "open", Metadata: map[string]string{"alias": "dual"}},
		{ID: "closed-a", Type: BeadType, Status: "closed", Metadata: map[string]string{"alias": "rig/closed-ambiguous"}},
		{ID: "closed-b", Type: BeadType, Status: "closed", Metadata: map[string]string{"alias": "rig/closed-ambiguous"}},
	}
	selectors := []string{"", "  ", "missing", "past", "rig/typed", "runtime-typed", "  rig/repair  ", "runtime-repair", "rig/dual", "retired", "rig/closed-repair", "rig/busy", "rig/wisp", "rig/no-history", "rig/ambiguous", "rig/closed-ambiguous"}
	for _, b := range corpus {
		selectors = append(selectors, b.ID)
	}
	for backend, wrap := range map[string]func(beads.Store) beads.Store{
		"memory": func(s beads.Store) beads.Store { return s },
		"loose":  func(s beads.Store) beads.Store { return looseMailboxProbeStore{Store: s} },
		"cache": func(s beads.Store) beads.Store {
			cache := beads.NewCachingStoreForTest(s, nil)
			if err := cache.Prime(t.Context()); err != nil {
				t.Fatal(err)
			}
			return cache
		},
	} {
		for _, selector := range selectors {
			for _, closed := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/closed=%t", backend, selector, closed), func(t *testing.T) {
					gotStore := beads.NewMemStoreFrom(len(corpus), corpus, nil)
					wantStore := beads.NewMemStoreFrom(len(corpus), corpus, nil)
					got, gotErr := NewStore(beads.SessionStore{Store: wrap(gotStore)}).ResolveMailboxAddress(selector, closed)
					want, wantErr := unrestrictedMailboxResolution(wrap(wantStore), selector, closed)
					if !reflect.DeepEqual(got, want) || fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
						t.Fatalf("resolve = (%+v, %v), unrestricted = (%+v, %v)", got, gotErr, want, wantErr)
					}
					for _, sentinel := range []error{ErrSessionNotFound, ErrAmbiguous} {
						if errors.Is(gotErr, sentinel) != errors.Is(wantErr, sentinel) {
							t.Fatalf("error classification differs for %v", sentinel)
						}
					}
					for _, b := range corpus {
						gotBead, _ := gotStore.Get(b.ID)
						wantBead, _ := wantStore.Get(b.ID)
						if gotBead.Type != wantBead.Type {
							t.Errorf("repair of %s = %q, want %q", b.ID, gotBead.Type, wantBead.Type)
						}
					}
				})
			}
		}
	}
}

// Some stores over-return metadata/status matches; the session and exact-value
// checks must still reject those rows after database-side session narrowing.
type looseMailboxProbeStore struct{ beads.Store }

func (s looseMailboxProbeStore) List(q beads.ListQuery) ([]beads.Bead, error) {
	q.Metadata = nil
	q.Status = ""
	q.IncludeClosed = true
	q.AllowScan = true
	return s.Store.List(q)
}

// This oracle retains the pre-optimization unrestricted metadata read.
func unrestrictedMailboxResolution(store beads.Store, selector string, closed bool) (Info, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return Info{}, newAddressDirectoryError("address not found", ErrSessionNotFound)
	}
	var matches []beads.Bead
	if !strings.Contains(selector, "/") {
		b, err := store.Get(selector)
		if err == nil && IsSessionBeadOrRepairable(b) && mailboxStatusMatches(b, closed) {
			RepairEmptyType(store, &b)
			matches = appendUniqueMailboxMatch(matches, b)
		} else if err != nil && !errors.Is(err, beads.ErrNotFound) {
			return Info{}, newAddressDirectoryError("direct lookup failed", err)
		}
	}
	for _, key := range []string{"alias", "session_name"} {
		q := beads.ListQuery{Metadata: map[string]string{key: selector}, TierMode: beads.TierBoth}
		if closed {
			q.Status = "closed"
		}
		rows, err := store.List(q)
		if err != nil {
			return Info{}, newAddressDirectoryError(key+" lookup failed", err)
		}
		for _, b := range rows {
			if !IsSessionBeadOrRepairable(b) {
				continue
			}
			RepairEmptyType(store, &b)
			if mailboxStatusMatches(b, closed) && strings.TrimSpace(b.Metadata[key]) == selector {
				matches = appendUniqueMailboxMatch(matches, b)
			}
		}
	}
	switch len(matches) {
	case 0:
		return Info{}, newAddressDirectoryError("address not found", ErrSessionNotFound)
	case 1:
		return infoFromPersistedBead(matches[0]), nil
	default:
		return Info{}, addressAmbiguity(matches)
	}
}

// All consumers of listAllBeads project only scalar fields/metadata. Dropping
// dependencies must preserve those projections and keep the raw helper opt-in.
func TestSessionAddressListingsSkipDependencies(t *testing.T) {
	corpus := listAllCorpus()
	for i := range corpus {
		corpus[i].Dependencies = []beads.Dep{{DependsOnID: "parent", Type: "parent-child"}}
		corpus[i].ParentID = "parent"
	}
	for _, opts := range []ListAllOptions{{}, {IncludeClosed: true}, {Live: true}, {CacheFirst: true, Sort: beads.SortCreatedDesc}} {
		for name, list := range map[string]func(*Store) (any, error){
			"addresses": func(s *Store) (any, error) { return s.ListAddresses(opts.IncludeClosed) },
			"all":       func(s *Store) (any, error) { return s.ListAll(opts) },
			"responses": func(s *Store) (any, error) { return s.ListAllWithResponses(opts) },
			"reconcile": func(s *Store) (any, error) { return s.ListAllForReconcile(opts) },
			"fingerprint": func(s *Store) (any, error) {
				rows, fingerprint, err := s.ListAllForReconcileWithFingerprint(opts)
				return struct {
					Rows        []ReconcileSession
					Fingerprint string
				}{rows, fingerprint}, err
			},
		} {
			t.Run(fmt.Sprintf("%s/%+v", name, opts), func(t *testing.T) {
				mem := beads.NewMemStoreFrom(len(corpus), corpus, nil)
				rec := &mailboxProbeStore{Store: mem}
				got, err := list(NewStore(beads.SessionStore{Store: rec}))
				want, wantErr := list(NewStore(beads.SessionStore{Store: mem}))
				if err != nil || wantErr != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("projection changed without dependencies: got (%+v, %v), want (%+v, %v)", got, err, want, wantErr)
				}
				if len(rec.queries) != 2 {
					t.Fatalf("list queries = %d, want 2", len(rec.queries))
				}
				for _, q := range rec.queries {
					if !q.SkipDependencies {
						t.Errorf("address listing hydrates unused dependencies: %+v", q)
					}
				}
			})
		}
	}
	rec := &mailboxProbeStore{Store: beads.NewMemStoreFrom(len(corpus), corpus, nil)}
	rows, err := ListAllSessionBeads(rec, beads.ListQuery{})
	if err != nil || len(rows) == 0 || rows[0].ParentID != "parent" || len(rows[0].Dependencies) != 1 {
		t.Fatalf("raw session-bead listing lost dependencies: %+v, %v", rows, err)
	}
}

func TestSessionAddressCachedListingSkipsDependencies(t *testing.T) {
	_, mem := newCorpusStore(t)
	rec := &recordingCacheStore{Store: mem, cachedTypeOK: true, cachedLabelOK: true}
	front := NewStore(beads.SessionStore{Store: rec})
	if _, err := front.ListAll(ListAllOptions{CacheFirst: true, Sort: beads.SortCreatedDesc}); err != nil {
		t.Fatal(err)
	}
	if len(rec.listCalls) != 0 || len(rec.cachedCalls) != 2 {
		t.Fatalf("cache hit reads: backing=%d cached=%d", len(rec.listCalls), len(rec.cachedCalls))
	}
	for _, q := range rec.cachedCalls {
		if !q.SkipDependencies {
			t.Errorf("cache address projection requires unused dependencies: %+v", q)
		}
	}
}

func TestMailboxMetadataProbeErrorsFailClosed(t *testing.T) {
	for _, key := range []string{"alias", "session_name"} {
		for _, labelLeg := range []bool{false, true} {
			for _, partial := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/label=%t/partial=%t", key, labelLeg, partial), func(t *testing.T) {
					backendErr := errors.New("backend detail")
					probeErr := backendErr
					if partial {
						probeErr = &beads.PartialResultError{Op: "probe", Err: backendErr}
					}
					mem := beads.NewMemStoreFrom(1, []beads.Bead{{ID: "s", Type: BeadType, Status: "open", Labels: []string{LabelSession}, Metadata: map[string]string{"alias": "rig/worker", "session_name": "rig/worker"}}}, nil)
					store := &mailboxErrorStore{Store: mem, key: key, labelLeg: labelLeg, err: probeErr}
					got, err := NewStore(beads.SessionStore{Store: store}).ResolveMailboxAddress("rig/worker", false)
					if got.ID != "" || !errors.Is(err, backendErr) || beads.IsPartialResult(err) != partial {
						t.Fatalf("resolve with probe failure = (%+v, %v)", got, err)
					}
					requireRedactedAddressError(t, err, "backend detail", "rig/worker")
				})
			}
		}
	}
}

type mailboxErrorStore struct {
	beads.Store
	key      string
	labelLeg bool
	err      error
}

func (s *mailboxErrorStore) List(q beads.ListQuery) ([]beads.Bead, error) {
	rows, err := s.Store.List(q)
	if q.Metadata[s.key] != "" && (q.Label == LabelSession) == s.labelLeg {
		return rows, s.err
	}
	return rows, err
}

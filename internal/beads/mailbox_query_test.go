package beads

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	beadslib "github.com/steveyegge/beads"
)

// Session narrowing must reach the native storage search, not only the
// in-memory filter after the metadata scan and dependency hydration.
func TestMailboxQueryNativePushdown(t *testing.T) {
	for _, key := range []string{"alias", "session_name"} {
		for _, status := range []string{"", "closed"} {
			for _, labelLeg := range []bool{false, true} {
				q := ListQuery{Metadata: map[string]string{key: "rig/worker"}, Status: status, TierMode: TierBoth, SkipDependencies: true}
				if labelLeg {
					q.Label = "gc:session"
				} else {
					q.Type = "session"
				}
				var got beadslib.IssueFilter
				calls := 0
				storage := &nativeDoltStorageSpy{searchIssues: func(_ context.Context, _ string, f beadslib.IssueFilter) ([]*beadslib.Issue, error) {
					calls++
					got = f
					return nil, nil
				}}
				if _, err := newNativeDoltStoreForTest(storage).List(q); err != nil {
					t.Fatal(err)
				}
				if calls != 1 || got.IncludeDependencies || got.Ephemeral != nil || !reflect.DeepEqual(got.MetadataFields, q.Metadata) {
					t.Fatalf("query %+v reached backing %d times with filter %+v", q, calls, got)
				}
				if labelLeg {
					if !reflect.DeepEqual(got.Labels, []string{q.Label}) || got.IssueType != nil {
						t.Fatalf("label restriction lost: %+v", got)
					}
				} else if got.IssueType == nil || string(*got.IssueType) != q.Type || len(got.Labels) != 0 {
					t.Fatalf("type restriction lost: %+v", got)
				}
				if status == "closed" {
					if got.Status == nil || string(*got.Status) != status {
						t.Fatalf("closed restriction lost: %+v", got)
					}
				} else if !reflect.DeepEqual(got.ExcludeStatus, []beadslib.Status{beadslib.StatusClosed}) {
					t.Fatalf("live restriction lost: %+v", got)
				}
			}
		}
	}
}

// BdStore must narrow both its list and wisp-query subprocesses. The latter
// cannot push metadata, so the session predicate is especially important.
func TestMailboxQueryBdPushdown(t *testing.T) {
	for _, labelLeg := range []bool{false, true} {
		q := ListQuery{Metadata: map[string]string{"alias": "rig/worker"}, Status: "closed", TierMode: TierBoth, SkipDependencies: true}
		predicate := "type=session"
		if labelLeg {
			q.Label = "gc:session"
			predicate = "label=gc:session"
		} else {
			q.Type = "session"
		}
		var calls []string
		store := NewBdStore("/unused", func(_, name string, args ...string) ([]byte, error) {
			if name != "bd" {
				t.Fatalf("command = %q", name)
			}
			calls = append(calls, strings.Join(args, " "))
			return []byte(`[]`), nil
		})
		if _, err := store.List(q); err != nil {
			t.Fatal(err)
		}
		if len(calls) != 2 {
			t.Fatalf("commands = %v, want list and wisp query", calls)
		}
		if !strings.HasPrefix(calls[0], "list ") || !strings.Contains(calls[0], "--"+predicate) || !strings.Contains(calls[0], "--metadata-field alias=rig/worker") {
			t.Fatalf("list predicate lost: %s", calls[0])
		}
		if !strings.HasPrefix(calls[1], "query ") || !strings.Contains(calls[1], "ephemeral=true") || !strings.Contains(calls[1], predicate) || !strings.Contains(calls[1], "status=closed") {
			t.Fatalf("wisp predicate lost: %s", calls[1])
		}
	}
}

// Dependency-free reads must retain all predicates through CachingStore and
// leave its complete projection available for callers that need parent edges.
func TestMailboxQueryCacheForwardsWithoutPoisoning(t *testing.T) {
	dep := Dep{IssueID: "s", DependsOnID: "parent", Type: "parent-child"}
	mem := NewMemStoreFrom(1, []Bead{{ID: "s", Type: "session", Status: "open", Labels: []string{"gc:session"}, Metadata: map[string]string{"alias": "rig/worker"}, ParentID: "parent", Dependencies: []Dep{dep}}}, nil)
	backing := &mailboxQueryBacking{Store: dependencySkippingStore{mem}}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(t.Context()); err != nil {
		t.Fatal(err)
	}
	backing.queries = nil
	for _, q := range []ListQuery{
		{Type: "session", Metadata: map[string]string{"alias": "rig/worker"}, TierMode: TierBoth, SkipDependencies: true, Live: true},
		{Label: "gc:session", Metadata: map[string]string{"alias": "rig/worker"}, TierMode: TierBoth, SkipDependencies: true, Live: true},
	} {
		rows, err := cache.List(q)
		if err != nil || len(rows) != 1 || rows[0].ID != "s" {
			t.Fatalf("cache list = %+v, %v", rows, err)
		}
		if got := backing.queries[len(backing.queries)-1]; !reflect.DeepEqual(got, q) {
			t.Fatalf("cache forwarded %+v, want %+v", got, q)
		}
	}
	rows, err := cache.List(ListQuery{Type: "session", Status: "open"})
	if err != nil || len(rows) != 1 || rows[0].ParentID != "parent" || !reflect.DeepEqual(rows[0].Dependencies, []Dep{dep}) {
		t.Fatalf("complete cache projection lost dependencies: %+v, %v", rows, err)
	}
}

// SkipDependencies is a hydration hint, not an instruction to bypass a valid
// cache snapshot or discard its partial-history error behavior.
func TestMailboxQueryCachePreservesSnapshot(t *testing.T) {
	for _, includeClosed := range []bool{false, true} {
		t.Run(strconv.FormatBool(includeClosed), func(t *testing.T) {
			mem := NewMemStoreFrom(1, []Bead{{ID: "s", Type: "session", Status: "open", Metadata: map[string]string{"alias": "rig/worker"}}}, nil)
			backing := &mailboxQueryBacking{Store: mem}
			cache := NewCachingStoreForTest(backing, nil)
			if err := cache.Prime(t.Context()); err != nil {
				t.Fatal(err)
			}
			backing.queries = nil
			backing.err = errors.New("history unavailable")
			q := ListQuery{Type: "session", IncludeClosed: includeClosed, SkipDependencies: true}
			rows, err := cache.List(q)
			if len(rows) != 1 || rows[0].ID != "s" {
				t.Fatalf("snapshot lost: %+v, %v", rows, err)
			}
			if includeClosed {
				if !IsPartialResult(err) || !errors.Is(err, backing.err) || len(backing.queries) != 1 || !backing.queries[0].SkipDependencies {
					t.Fatalf("history read lost partial-result/skip contract: %v, %+v", err, backing.queries)
				}
			} else if err != nil || len(backing.queries) != 0 {
				t.Fatalf("cached read reached backing: %v, %+v", err, backing.queries)
			}
		})
	}
}

type mailboxQueryBacking struct {
	Store
	queries []ListQuery
	err     error
}

func (s *mailboxQueryBacking) List(q ListQuery) ([]Bead, error) {
	s.queries = append(s.queries, q)
	if s.err != nil {
		return nil, s.err
	}
	return s.Store.List(q)
}

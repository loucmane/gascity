package beads

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"testing"
	"time"

	beadslib "github.com/steveyegge/beads"
)

// An exact identity lookup must restrict the backing search, not hydrate the
// corpus and discard it in ApplyListQuery.
func TestNativeIssueFilterPreservesIDs(t *testing.T) {
	for _, ids := range [][]string{nil, {}, {""}, {"attention-fable", "", "attention-fable"}} {
		filter := nativeIssueFilterFromListQuery(ListQuery{IDs: ids, IncludeClosed: true, TierMode: TierBoth})
		if !reflect.DeepEqual(filter.IDs, ids) {
			t.Errorf("IDs = %#v, want %#v", filter.IDs, ids)
		}
	}
}

func TestNativeDoltStoreListIDsChunkAndMerge(t *testing.T) {
	const chunkSize = 500
	base := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	var issues []*beadslib.Issue
	var ids []string
	for i := 0; i < chunkSize+7; i++ {
		id := fmt.Sprintf("gc-%04d", i)
		if i == 0 {
			id = ""
		}
		issue := &beadslib.Issue{
			ID: id, Title: id, Status: beadslib.StatusOpen, Priority: i % 5,
			CreatedAt: base.Add(time.Duration(i%7) * time.Second), Ephemeral: i%2 == 1,
			Labels: []string{"fixture"}, Dependencies: []*beadslib.Dependency{{DependsOnID: "target", Type: beadslib.DepBlocks}},
		}
		if i%3 == 1 {
			issue.Status = beadslib.StatusClosed
		}
		issues, ids = append(issues, issue), append(ids, id)
	}
	// Independent oracle: the pinned library's default order before Go filtering.
	sort.Slice(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.After(b.CreatedAt)
		}
		return a.ID < b.ID
	})
	var all []Bead
	for _, issue := range issues {
		b, err := beadFromNativeIssue(issue)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, b)
	}
	ids = append(ids, "missing", ids[0], ids[chunkSize], ids[1])
	slices.Reverse(ids)
	for _, order := range []SortOrder{SortDefault, SortCreatedAsc, SortCreatedDesc} {
		for _, seek := range []*SeekBoundary{nil, {CreatedAt: base.Add(3 * time.Second), ID: "gc-0300"}} {
			if order == SortDefault && seek != nil {
				continue
			}
			q := ListQuery{
				IDs: ids, IncludeClosed: true, TierMode: TierBoth, Sort: order,
				Limit: 9, SeekAfter: seek, AllowBackingCreatedLimit: true,
			}
			var calls int
			requested := make(map[string]bool)
			storage := &nativeDoltStorageSpy{searchIssues: func(_ context.Context, _ string, f beadslib.IssueFilter) ([]*beadslib.Issue, error) {
				calls++
				if len(f.IDs) == 0 || len(f.IDs) > chunkSize {
					t.Errorf("backing IDs length = %d, want 1..%d", len(f.IDs), chunkSize)
				}
				if f.Limit != 0 || f.AfterCreatedAt != nil || f.AfterID != "" {
					t.Errorf("chunk applied a page boundary: %+v", f)
				}
				for _, id := range f.IDs {
					if requested[id] {
						t.Errorf("requested ID %q more than once", id)
					}
					requested[id] = true
				}
				var found []*beadslib.Issue
				for _, issue := range issues {
					if slices.Contains(f.IDs, issue.ID) {
						found = append(found, issue)
					}
				}
				return found, nil
			}}
			got, err := newNativeDoltStoreForTest(storage).List(q)
			if err != nil {
				t.Fatal(err)
			}
			if want := ApplyListQuery(all, q); !reflect.DeepEqual(got, want) {
				t.Errorf("order=%v seek=%v: rows/order/hydration differ from scan + post-filter", order, seek)
			}
			if calls != 2 || len(requested) != chunkSize+8 {
				t.Errorf("calls=%d distinct IDs=%d, want 2/%d", calls, len(requested), chunkSize+8)
			}
		}
	}
}

func TestNativeDoltStoreListIDsChunkFailure(t *testing.T) {
	ids := make([]string, 501)
	for i := range ids {
		ids[i] = fmt.Sprintf("gc-%d", i)
	}
	failure := errors.New("second chunk failed")
	calls := 0
	storage := &nativeDoltStorageSpy{searchIssues: func(context.Context, string, beadslib.IssueFilter) ([]*beadslib.Issue, error) {
		calls++
		if calls == 2 {
			return nil, failure
		}
		return []*beadslib.Issue{{ID: ids[0], Status: beadslib.StatusOpen}}, nil
	}}
	got, err := newNativeDoltStoreForTest(storage).List(ListQuery{IDs: ids})
	if !errors.Is(err, failure) || got != nil {
		t.Fatalf("List = %v, %v; want no partial rows and chunk error", got, err)
	}
}

func TestNativeDoltStoreListIDsChunkBoundaries(t *testing.T) {
	for _, count := range []int{0, 1, 500, 501, 1000, 1001} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			ids := make([]string, count)
			for i := range ids {
				ids[i] = fmt.Sprintf("gc-%d", i)
			}
			original := slices.Clone(ids)
			var sizes []int
			storage := &nativeDoltStorageSpy{searchIssues: func(_ context.Context, _ string, f beadslib.IssueFilter) ([]*beadslib.Issue, error) {
				sizes = append(sizes, len(f.IDs))
				return nil, nil
			}}
			if _, err := newNativeDoltStoreForTest(storage).List(ListQuery{IDs: ids, AllowScan: true}); err != nil {
				t.Fatal(err)
			}
			if len(sizes) != max(1, (count+499)/500) {
				t.Fatalf("chunk sizes = %v", sizes)
			}
			total := 0
			for _, size := range sizes {
				if size > 500 || (count > 0 && size == 0) {
					t.Fatalf("chunk sizes = %v", sizes)
				}
				total += size
			}
			if total != count || !slices.Equal(ids, original) {
				t.Fatal("lost or mutated requested IDs")
			}
		})
	}
}

func TestNativeDoltStoreListIDsRepeatedEmptyAndOverlappingRows(t *testing.T) {
	for _, repeatedEmpty := range []bool{false, true} {
		ids := make([]string, 501)
		if !repeatedEmpty {
			for i := range ids {
				ids[i] = fmt.Sprintf("gc-%d", i)
			}
		}
		calls := 0
		storage := &nativeDoltStorageSpy{searchIssues: func(_ context.Context, _ string, f beadslib.IssueFilter) ([]*beadslib.Issue, error) {
			calls++
			if len(f.IDs) == 0 || (repeatedEmpty && !slices.Equal(f.IDs, []string{""})) {
				t.Fatalf("empty IDs lost: %#v", f.IDs)
			}
			// Model a row matched in multiple chunks by database collation.
			return []*beadslib.Issue{{ID: ids[0], Status: beadslib.StatusOpen}}, nil
		}}
		got, err := newNativeDoltStoreForTest(storage).List(ListQuery{IDs: ids})
		if err != nil || len(got) != 1 || got[0].ID != ids[0] {
			t.Fatalf("List = %+v, %v", got, err)
		}
		wantCalls := 2
		if repeatedEmpty {
			wantCalls = 1
		}
		if calls != wantCalls {
			t.Fatalf("calls=%d, want %d", calls, wantCalls)
		}
	}
}

func TestLookupByIDKeepsIDsThroughCacheAndProxy(t *testing.T) {
	var queries []beadslib.IssueFilter
	storage := &nativeDoltStorageSpy{searchIssues: func(_ context.Context, _ string, f beadslib.IssueFilter) ([]*beadslib.Issue, error) {
		queries = append(queries, f)
		// Deliberately include a nonmatch: ApplyListQuery remains authoritative.
		return []*beadslib.Issue{{ID: "other", Status: beadslib.StatusOpen}, {ID: "attention-fable", Status: beadslib.StatusClosed, Ephemeral: true}}, nil
	}}
	native := newNativeDoltStoreForTest(storage, WithProxiedReadOnly())
	bd := &recordingLeaf{Store: NewMemStore()}
	proxy, err := NewProxiedStore(native, bd, PinForTest("/scope", "", "beads"))
	if err != nil {
		t.Fatal(err)
	}
	cache := NewCachingStoreForTest(proxy, nil)
	if err := cache.Prime(t.Context()); err != nil {
		t.Fatal(err)
	}
	queries = nil
	for _, id := range []string{"attention-fable", "missing"} {
		b, err := LookupByID(cache, id)
		if id == "missing" {
			if !errors.Is(err, ErrNotFound) {
				t.Fatal(err)
			}
		} else if err != nil || b.ID != id {
			t.Fatalf("LookupByID = %+v, %v", b, err)
		}
		if len(queries) != 1 || !slices.Equal(queries[0].IDs, []string{id}) || queries[0].Ephemeral != nil || len(queries[0].ExcludeStatus) != 0 {
			t.Fatalf("wrapper widened the exact closed-inclusive both-tier query: %+v", queries)
		}
		queries = nil
	}
	if calls := bd.took(); len(calls) != 0 {
		t.Fatalf("lookup used CLI leaf: %v", calls)
	}
}

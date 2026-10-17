package beads

import (
	"context"
	"reflect"
	"testing"

	beadslib "github.com/steveyegge/beads"
)

// Retention's reverse walk must use the resolved ParentID. SQL's broader
// dotted-ID fallback is not the final Store.List parent-child relation.
func TestNativeRetentionParentProjection(t *testing.T) {
	dep := func(child, parent string) *beadslib.Dependency {
		return &beadslib.Dependency{IssueID: child, DependsOnID: parent, Type: beadslib.DepParentChild}
	}
	issues := []*beadslib.Issue{
		{ID: "root.1", Status: beadslib.StatusOpen},
		{ID: "root.2", Status: beadslib.StatusOpen, Dependencies: []*beadslib.Dependency{dep("root.2", "other")}},
		{ID: "root.3", Status: beadslib.StatusOpen, Dependencies: []*beadslib.Dependency{dep("root.3", "root")}},
		{ID: "wisp", Status: beadslib.StatusBlocked, Ephemeral: true, Dependencies: []*beadslib.Dependency{dep("wisp", "root")}},
		{ID: "foreign", Status: beadslib.StatusDeferred, NoHistory: true, Dependencies: []*beadslib.Dependency{dep("foreign", "external:scope:root")}},
		{ID: "multiple", Status: beadslib.StatusOpen, Dependencies: []*beadslib.Dependency{dep("multiple", "other"), dep("multiple", "root")}},
	}
	storage := &nativeDoltStorageSpy{searchIssues: func(_ context.Context, _ string, filter beadslib.IssueFilter) ([]*beadslib.Issue, error) {
		if !filter.IncludeDependencies || filter.Ephemeral != nil || filter.Limit != 0 {
			t.Fatalf("incomplete native retention filter: %+v", filter)
		}
		if filter.ParentID == nil && len(filter.IDs) == 0 && !reflect.DeepEqual(filter.ExcludeStatus, []beadslib.Status{beadslib.StatusClosed}) {
			t.Fatalf("non-closed scan must push status != closed to SQL: %+v", filter)
		}
		// Return a superset, including SQL dotted-ID candidates and the second
		// parent of a multiply parented row; the adapter owns the final filter.
		return issues, nil
	}}
	store := newNativeDoltStoreForTest(storage)
	handles := HandlesFor(store)
	if handles.Parents != ParentFirstDependency || !handles.ListDependenciesComplete {
		t.Fatalf("native read capabilities: %+v", handles)
	}
	all, err := store.List(ListQuery{AllowScan: true, TierMode: TierBoth})
	if err != nil {
		t.Fatal(err)
	}
	for parent, want := range map[string][]string{
		"root": {"root.3", "wisp"}, "other": {"root.2", "multiple"}, "external:scope:root": {"foreign"},
	} {
		q := ListQuery{ParentID: parent, IncludeClosed: true, TierMode: TierBoth}
		children, err := store.List(q)
		if err != nil {
			t.Fatal(err)
		}
		var forward, reverse []string
		for _, b := range children {
			forward = append(forward, b.ID)
		}
		for _, b := range all {
			if b.ParentID == parent {
				reverse = append(reverse, b.ID)
			}
			got, err := store.Get(b.ID)
			if err != nil || got.ParentID != b.ParentID {
				t.Fatalf("Get(%s) parent=%s, %v; List parent=%s", b.ID, got.ParentID, err, b.ParentID)
			}
		}
		if !reflect.DeepEqual(forward, want) || !reflect.DeepEqual(reverse, want) {
			t.Fatalf("parent=%s forward=%v reverse=%v want=%v", parent, forward, reverse, want)
		}
	}
}

func TestRetentionParentProjectionFollowsReadLeaf(t *testing.T) {
	native := newNativeDoltStoreForTest(&nativeDoltStorageSpy{})
	for _, store := range []Store{native, &BdStore{}, &ProxiedStore{native: native, bd: NewMemStore()}, NewCachingStoreForTest(native, nil)} {
		if HandlesFor(store).Parents != ParentFirstDependency {
			t.Fatalf("%T lost dependency parent projection", store)
		}
	}
	for _, store := range []Store{NewMemStore(), &ProxiedStore{bd: NewMemStore()}, NewCachingStoreForTest(NewMemStore(), nil)} {
		if HandlesFor(store).Parents != ParentField {
			t.Fatalf("%T lost stored parent projection", store)
		}
	}
}

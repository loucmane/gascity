package main

import (
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/orders"
)

func retentionGuardRow(id, status, parent, root string) beads.Bead {
	return beads.Bead{ID: id, Status: status, ParentID: parent, Metadata: map[string]string{beadmeta.RootBeadIDMetadataKey: root}}
}

func compareRetentionGuard(t *testing.T, rows []beads.Bead, deps []beads.Dep, candidates []string) {
	t.Helper()
	for _, projection := range []beads.ParentProjection{beads.ParentField, beads.ParentFirstDependency, beads.ParentAllDependencies} {
		store := beads.Store(beads.NewMemStoreFrom(1000, rows, deps))
		if projection != beads.ParentField {
			store = &retentionParentStore{Store: store, projection: projection}
		}
		compareRetentionGuardStore(t, store, candidates)
	}
}

func compareRetentionGuardStore(t *testing.T, store beads.Store, candidates []string) {
	t.Helper()
	guard, err := newOrderRetentionGuard(store)
	if err != nil {
		t.Fatal(err)
	}
	for _, deleting := range []map[string]struct{}{nil, {"r": {}, "open": {}, "n1": {}}} {
		for _, id := range candidates {
			want := assertWorkflowDeleteLeavesNothingStranded(store, id, deleting)
			got := guard.check(id, deleting)
			if (got == nil) != (want == nil) || errors.Is(got, errWorkflowDeleteLiveDescendants) != errors.Is(want, errWorkflowDeleteLiveDescendants) {
				t.Fatalf("root=%s deleting=%v: batch=%v legacy=%v", id, deleting, got, want)
			}
		}
	}
	// Compare decisions again as successful deletions change the graph.
	for _, id := range candidates {
		b, err := store.Get(id)
		if err != nil || b.Status != "closed" {
			continue
		}
		want := assertWorkflowDeleteLeavesNothingStranded(store, id, nil)
		got := guard.delete(store, id)
		if (got == nil) != (want == nil) || errors.Is(got, errWorkflowDeleteLiveDescendants) != errors.Is(want, errWorkflowDeleteLiveDescendants) {
			t.Fatalf("sequential root=%s: batch=%v legacy=%v", id, got, want)
		}
	}
}

// Model the two dependency-backed parent projections at the Store boundary.
// The real native adapter's projection is independently pinned in beads tests.
type retentionParentStore struct {
	beads.Store
	projection beads.ParentProjection
}

func (s *retentionParentStore) ParentProjection() beads.ParentProjection { return s.projection }

func (s *retentionParentStore) Get(id string) (beads.Bead, error) {
	b, err := s.Store.Get(id)
	if err != nil {
		return b, err
	}
	b.Dependencies, err = s.DepList(id, "down")
	b.ParentID = ""
	for _, dep := range b.Dependencies {
		if dep.Type == "parent-child" {
			b.ParentID = dep.DependsOnID
			break
		}
	}
	return b, err
}

func (s *retentionParentStore) List(q beads.ListQuery) ([]beads.Bead, error) {
	all, err := s.Store.List(beads.ListQuery{AllowScan: true, IncludeClosed: true, TierMode: beads.TierBoth})
	if err != nil {
		return nil, err
	}
	var rows []beads.Bead
	for _, seed := range all {
		b, err := s.Get(seed.ID)
		if err != nil {
			return nil, err
		}
		if s.projection == beads.ParentAllDependencies && q.ParentID != "" {
			for _, dep := range b.Dependencies {
				if dep.Type == "parent-child" && dep.DependsOnID == q.ParentID {
					b.ParentID = q.ParentID
					break
				}
			}
		}
		rows = append(rows, b)
	}
	return beads.ApplyListQuery(rows, q), nil
}

func TestRetentionGuardReprojectsDeletedDependencyParent(t *testing.T) {
	rows := []beads.Bead{
		retentionGuardRow("a", "closed", "", ""),
		retentionGuardRow("b", "closed", "", ""),
		retentionGuardRow("child", "closed", "", ""),
		retentionGuardRow("bridge", "closed", "", "b"),
		retentionGuardRow("open", "open", "", ""),
	}
	rows[2].Metadata[beadmeta.StepRefMetadataKey] = "step"
	deps := []beads.Dep{
		{IssueID: "child", DependsOnID: "a", Type: "parent-child"},
		{IssueID: "child", DependsOnID: "b", Type: "parent-child"},
		{IssueID: "bridge", DependsOnID: "child", Type: "tracks"},
		{IssueID: "open", DependsOnID: "bridge", Type: "parent-child"},
	}
	store := &retentionParentStore{Store: beads.NewMemStoreFrom(100, rows, deps), projection: beads.ParentFirstDependency}
	guard, err := newOrderRetentionGuard(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.check("b", nil); err != nil {
		t.Fatalf("b should initially be safe: %v", err)
	}
	if err := guard.delete(store, "a"); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(assertWorkflowDeleteLeavesNothingStranded(store, "b", nil), errWorkflowDeleteLiveDescendants) {
		t.Fatal("legacy guard must see reparented live work")
	}
	if !errors.Is(guard.check("b", nil), errWorkflowDeleteLiveDescendants) {
		t.Fatal("batch guard missed reparented live work")
	}
}

func TestRetentionGuardUsesLiveStoreBehindCache(t *testing.T) {
	store := newRetentionGuardRecorder([]beads.Bead{retentionGuardRow("r", "closed", "", ""), retentionGuardRow("mid", "closed", "r", ""), retentionGuardRow("open", "open", "mid", "")}, nil)
	cache := beads.NewCachingStoreForTest(store, nil)
	guard, err := newOrderRetentionGuard(cache)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(guard.check("r", nil), errWorkflowDeleteLiveDescendants) {
		t.Fatal("closed intermediates must be read live through cache wrapper")
	}
	if len(store.queries) != 1 || !store.queries[0].Live || store.gets["mid"] != 1 {
		t.Fatalf("reads = %+v, %v", store.queries, store.gets)
	}
}

// The sparse reverse closure must give the legacy guard's decision even when
// membership is partial, graph ownership is root-specific, or paths cycle.
func TestRetentionGuardEquivalence(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows []beads.Bead
		deps []beads.Dep
	}{
		{"closed intermediate", []beads.Bead{retentionGuardRow("r", "closed", "", ""), retentionGuardRow("mid", "closed", "r", ""), retentionGuardRow("open", "blocked", "mid", "")}, nil},
		{"membership is not transitive", []beads.Bead{retentionGuardRow("r", "closed", "", ""), retentionGuardRow("mid", "closed", "", "r"), retentionGuardRow("open", "deferred", "mid", "")}, nil},
		{"root itself never blocks", []beads.Bead{retentionGuardRow("r", "open", "r", "r")}, nil},
		{"missing structural ancestor", []beads.Bead{retentionGuardRow("open", "open", "missing", "")}, nil},
		{"missing graph ancestor", []beads.Bead{retentionGuardRow("open", "open", "", "r")}, []beads.Dep{{IssueID: "open", DependsOnID: "missing", Type: "blocks"}}},
		{"dotted ID without parent", []beads.Bead{retentionGuardRow("r.1.2", "open", "", "")}, nil},
		{"dotted ID with explicit different parent", []beads.Bead{retentionGuardRow("r.1", "open", "other", "")}, nil},
		{"dotted ID with explicit parent", []beads.Bead{retentionGuardRow("r.1", "open", "r", "")}, nil},
		{"cycle", []beads.Bead{retentionGuardRow("r", "closed", "mid", ""), retentionGuardRow("mid", "closed", "r", ""), retentionGuardRow("open", "open", "mid", "")}, nil},
		{"graph closed bridge", []beads.Bead{retentionGuardRow("r", "closed", "", "r"), retentionGuardRow("mid", "closed", "", "r"), retentionGuardRow("open", "open", "mid", "")}, []beads.Dep{{IssueID: "mid", DependsOnID: "r", Type: "tracks"}}},
		{"unowned graph edge", []beads.Bead{retentionGuardRow("r", "closed", "", "r"), retentionGuardRow("mid", "closed", "", "other"), retentionGuardRow("open", "open", "mid", "")}, []beads.Dep{{IssueID: "mid", DependsOnID: "r", Type: "parent-child"}}},
		{"plain parent cannot have graph dependents", []beads.Bead{retentionGuardRow("r", "closed", "", ""), retentionGuardRow("mid", "closed", "", "r"), retentionGuardRow("open", "open", "mid", "")}, []beads.Dep{{IssueID: "mid", DependsOnID: "r", Type: "blocks"}}},
		{"unsupported edge", []beads.Bead{retentionGuardRow("r", "closed", "", "r"), retentionGuardRow("mid", "closed", "", "r"), retentionGuardRow("open", "open", "mid", "")}, []beads.Dep{{IssueID: "mid", DependsOnID: "r", Type: "related"}}},
		{"notification bridge", []beads.Bead{retentionGuardRow("r", "closed", "", ""), {ID: "mail", Status: "open", Type: "message", ParentID: "r"}, retentionGuardRow("open", "open", "mail", "")}, nil},
		{"notifications only", []beads.Bead{retentionGuardRow("r", "closed", "", ""), {ID: "mail", Status: "open", Type: "message", ParentID: "r"}, {ID: "nudge", Status: "open", Type: nudgeBeadType, Labels: []string{nudgeBeadLabel}, Metadata: map[string]string{beadmeta.RootBeadIDMetadataKey: "r"}}}, nil},
		{"shared descendant", []beads.Bead{retentionGuardRow("r", "closed", "", ""), retentionGuardRow("other", "closed", "", ""), retentionGuardRow("open", "open", "other", "r")}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i := range tc.rows {
				tc.rows[i].Ephemeral = i%2 == 0
				tc.rows[i].NoHistory = i%3 == 0
			}
			compareRetentionGuard(t, tc.rows, tc.deps, []string{"r", "mid", "open", "missing", "other"})
		})
	}
}

func TestRetentionGuardRandomizedEquivalence(t *testing.T) {
	rng := rand.New(rand.NewSource(20261017))
	for trial := range 300 {
		t.Run(fmt.Sprint(trial), func(t *testing.T) {
			ids := []string{"r", "other", "n1", "r.1", "r.1.2", "n2", "n3", "open", "r.3", "end"}
			refs := append(append([]string{"", "missing", "external:scope:root"}, ids...), "")
			var rows []beads.Bead
			var deps []beads.Dep
			statuses := []string{"open", "in_progress", "blocked", "deferred", "closed", "closed", "closed"}
			types := []string{"blocks", "parent-child", "tracks", "related", "waits-for"}
			for _, id := range ids {
				b := retentionGuardRow(id, statuses[rng.Intn(len(statuses))], refs[rng.Intn(len(refs))], refs[rng.Intn(len(refs))])
				b.Ephemeral, b.NoHistory = rng.Intn(2) == 0, rng.Intn(3) == 0
				switch rng.Intn(5) {
				case 0:
					b.Type = "message"
				case 1:
					b.Type, b.Labels = nudgeBeadType, []string{nudgeBeadLabel}
				case 2:
					b.Metadata[beadmeta.StepRefMetadataKey] = "step"
				}
				rows = append(rows, b)
				for range rng.Intn(4) {
					deps = append(deps, beads.Dep{IssueID: id, DependsOnID: refs[1+rng.Intn(len(refs)-2)], Type: types[rng.Intn(len(types))]})
				}
			}
			compareRetentionGuard(t, rows, deps, append(ids, "missing", "external:scope:root"))
		})
	}
}

type retentionGuardRecorder struct {
	beads.Store
	queries                []beads.ListQuery
	gets                   map[string]int
	deps                   map[string]int
	reads, failAt, deletes int
}

var errRetentionGuardRead = errors.New("injected retention guard read failure")

func (s *retentionGuardRecorder) read() error {
	s.reads++
	if s.reads == s.failAt {
		return errRetentionGuardRead
	}
	return nil
}

func (s *retentionGuardRecorder) List(q beads.ListQuery) ([]beads.Bead, error) {
	s.queries = append(s.queries, q)
	if err := s.read(); err != nil {
		return nil, err
	}
	return s.Store.List(q)
}

func (s *retentionGuardRecorder) Get(id string) (beads.Bead, error) {
	s.gets[id]++
	if err := s.read(); err != nil {
		return beads.Bead{}, err
	}
	return s.Store.Get(id)
}

func (s *retentionGuardRecorder) DepList(id, direction string) ([]beads.Dep, error) {
	s.deps[id+"/"+direction]++
	if err := s.read(); err != nil {
		return nil, err
	}
	return s.Store.DepList(id, direction)
}

func (s *retentionGuardRecorder) Delete(id string) error {
	s.deletes++
	return s.Store.Delete(id)
}

func (s *retentionGuardRecorder) ParentProjection() beads.ParentProjection {
	return beads.HandlesFor(s.Store).Parents
}

type retentionCompleteListStore struct {
	*retentionGuardRecorder
}

func (s *retentionCompleteListStore) Handles() beads.StoreHandles {
	h := beads.HandlesFor(s.retentionGuardRecorder)
	h.ListDependenciesComplete = true
	return h
}

func TestRetentionGuardReusesCompleteOpenDependencies(t *testing.T) {
	rows := []beads.Bead{
		retentionGuardRow("r", "closed", "", "r"),
		retentionGuardRow("open", "open", "", "r"),
	}
	deps := []beads.Dep{{IssueID: "open", DependsOnID: "r", Type: "parent-child"}}
	recorder := newRetentionGuardRecorder(rows, deps)
	recorder.Store = &retentionParentStore{Store: recorder.Store, projection: beads.ParentFirstDependency}
	store := &retentionCompleteListStore{recorder}
	guard, err := newOrderRetentionGuard(wrapStoreWithBeadPolicies(store, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(guard.check("r", nil), errWorkflowDeleteLiveDescendants) {
		t.Fatal("missing live descendant")
	}
	if len(recorder.queries) != 1 || recorder.gets["r"] != 1 || recorder.deps["open/down"] != 0 || recorder.deps["r/down"] != 1 {
		t.Fatalf("lists=%v gets=%v deps=%v", recorder.queries, recorder.gets, recorder.deps)
	}
}

func newRetentionGuardRecorder(rows []beads.Bead, deps []beads.Dep) *retentionGuardRecorder {
	return &retentionGuardRecorder{Store: beads.NewMemStoreFrom(10000, rows, deps), gets: make(map[string]int), deps: make(map[string]int)}
}

func retentionRecordedProjection(rows []beads.Bead, deps []beads.Dep, projection beads.ParentProjection) (*retentionGuardRecorder, beads.Store) {
	recorder := newRetentionGuardRecorder(rows, deps)
	if projection != beads.ParentField {
		recorder.Store = &retentionParentStore{Store: recorder.Store, projection: projection}
	}
	if projection == beads.ParentFirstDependency {
		return recorder, &retentionCompleteListStore{recorder}
	}
	return recorder, recorder
}

func TestRetentionGuardSharedAncestorsReadOnce(t *testing.T) {
	for _, count := range []int{1, 20, 200} {
		for _, projection := range []beads.ParentProjection{beads.ParentField, beads.ParentFirstDependency, beads.ParentAllDependencies} {
			t.Run(fmt.Sprintf("count=%d/projection=%d", count, projection), func(t *testing.T) {
				rows := []beads.Bead{retentionGuardRow("open", "in_progress", "root-0", "")}
				deps := []beads.Dep{{IssueID: "open", DependsOnID: "root-0", Type: "parent-child"}}
				var runs []orders.OrderRun
				for i := range count {
					id, parent, leaf := fmt.Sprintf("root-%d", i), fmt.Sprintf("root-%d", i+1), fmt.Sprintf("leaf-%d", i)
					rows = append(rows, retentionGuardRow(id, "closed", parent, id))
					rows = append(rows, retentionGuardRow(leaf, "open", "root-0", ""))
					rows = append(rows, retentionGuardRow(fmt.Sprintf("dead-%d", i), "closed", id, ""))
					deps = append(deps, beads.Dep{IssueID: id, DependsOnID: parent, Type: "parent-child"}, beads.Dep{IssueID: leaf, DependsOnID: "root-0", Type: "parent-child"})
					runs = append(runs, orders.OrderRun{ID: id})
				}
				recorder, store := retentionRecordedProjection(rows, deps, projection)
				n, err := deleteClosedOrderTrackingCandidates(store, map[string][]orders.OrderRun{"test": runs}, 0)
				if err != nil || n != 0 {
					t.Fatalf("delete = %d, %v", n, err)
				}
				if len(recorder.queries) != 1 || len(recorder.gets) != count+1 {
					t.Fatalf("lists=%d gets=%v", len(recorder.queries), recorder.gets)
				}
				for id, n := range recorder.gets {
					if n != 1 {
						t.Errorf("Get(%s) = %d calls", id, n)
					}
				}
				for id, n := range recorder.deps {
					if n != 1 {
						t.Errorf("DepList(%s) = %d calls", id, n)
					}
				}
				if projection == beads.ParentFirstDependency && len(recorder.deps) != count {
					t.Fatalf("complete open dependencies were reread: %v", recorder.deps)
				}
			})
		}
	}
}

func TestRetentionGuardReadErrorsFailClosed(t *testing.T) {
	rows := []beads.Bead{
		retentionGuardRow("safe", "closed", "", ""),
		retentionGuardRow("r", "closed", "", "r"),
		retentionGuardRow("mid", "closed", "", "r"),
		retentionGuardRow("open", "open", "mid", ""),
		retentionGuardRow("orphan", "open", "missing", ""),
	}
	deps := []beads.Dep{
		{IssueID: "mid", DependsOnID: "r", Type: "tracks"},
		{IssueID: "open", DependsOnID: "mid", Type: "parent-child"},
		{IssueID: "orphan", DependsOnID: "missing", Type: "parent-child"},
	}
	for _, projection := range []beads.ParentProjection{beads.ParentField, beads.ParentFirstDependency, beads.ParentAllDependencies} {
		baseline, baselineStore := retentionRecordedProjection(rows, deps, projection)
		if _, err := newOrderRetentionGuard(baselineStore); err != nil {
			t.Fatal(err)
		}
		for failAt := 1; failAt <= baseline.reads; failAt++ {
			t.Run(fmt.Sprintf("projection=%d/read=%d", projection, failAt), func(t *testing.T) {
				recorder, store := retentionRecordedProjection(rows, deps, projection)
				recorder.failAt = failAt
				n, err := deleteClosedOrderTrackingCandidates(store, map[string][]orders.OrderRun{"test": {{ID: "safe"}, {ID: "r"}}}, 0)
				if n != 0 || recorder.deletes != 0 || !errors.Is(err, errRetentionGuardRead) {
					t.Fatalf("read=%d: deleted=%d calls=%d err=%v", failAt, n, recorder.deletes, err)
				}
			})
		}
	}
}

func TestRetentionGuardDeletesIntermediateBeforeLaterCandidate(t *testing.T) {
	// mid is safe for its own root-specific guard, but is a graph-enabled
	// bridge in r's path to open. Deleting mid must remove that path to r.
	rows := []beads.Bead{
		retentionGuardRow("r", "closed", "", "r"),
		retentionGuardRow("mid", "closed", "r", "mid"),
		retentionGuardRow("bridge", "closed", "", "r"),
		retentionGuardRow("open", "open", "bridge", ""),
	}
	deps := []beads.Dep{{IssueID: "bridge", DependsOnID: "mid", Type: "blocks"}}
	store := beads.NewMemStoreFrom(100, rows, deps)
	guard, err := newOrderRetentionGuard(store)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(guard.check("r", nil), errWorkflowDeleteLiveDescendants) {
		t.Fatal("r must initially be retained")
	}
	for _, id := range []string{"mid", "r"} {
		if err := assertWorkflowDeleteLeavesNothingStranded(store, id, nil); err != nil {
			t.Fatal(err)
		}
		if err := guard.delete(store, id); err != nil {
			t.Fatal(err)
		}
	}
}

// Retention must not scan closed descendants or metadata once per expired root.
func TestRetentionGuardReadBound(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	for _, count := range []int{1, 20, 200} {
		for _, bounded := range []bool{false, true} {
			t.Run(fmt.Sprintf("roots=%d/bounded=%v", count, bounded), func(t *testing.T) {
				var rows []beads.Bead
				for i := range count + minClosedOrderTrackingRetained {
					rows = append(rows, beads.Bead{
						ID: fmt.Sprintf("run-%04d", i), Title: "order:busy", Status: "closed",
						CreatedAt: now.Add(-8 * 24 * time.Hour), Labels: []string{labelOrderTracking, "order-run:busy"},
						Ephemeral: i%2 == 0,
					})
				}
				store := &retentionReadRecorder{Store: beads.NewMemStoreFrom(1000, rows, nil)}
				policy := orderTrackingRetentionPolicyForConfig(nil)
				var n int
				var err error
				if bounded {
					n, err = sweepClosedOrderTrackingRetentionBounded(store, now, policy, nil, count)
				} else {
					n, err = sweepClosedOrderTrackingRetention(store, now, policy, nil)
				}
				if n != count || err != nil {
					t.Fatalf("sweep = %d, %v; want %d, nil", n, err, count)
				}
				guardLists := 0
				for _, q := range store.queries {
					if q.ParentID != "" || len(q.Metadata) != 0 {
						t.Fatalf("per-root descendant query: %+v", q)
					}
					if q.Status != "closed" {
						guardLists++
						if q.IncludeClosed || q.TierMode != beads.TierBoth || !q.Live || q.SkipDependencies {
							t.Fatalf("guard needs live, complete, non-closed rows in both tiers: %+v", q)
						}
					}
				}
				if guardLists != 1 {
					t.Fatalf("guard lists = %d, want 1", guardLists)
				}
			})
		}
	}
}

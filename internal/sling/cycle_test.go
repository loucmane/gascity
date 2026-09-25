package sling

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
)

// --- fake DepLister implementations ---

// fakeDepGraph maps a bead ID to the IDs it depends on ("down" direction).
// All edges are treated as "blocks" type.
type fakeDepGraph map[string][]string

func (g fakeDepGraph) DepList(id, direction string) ([]beads.Dep, error) {
	if direction != "down" {
		return nil, nil
	}
	var deps []beads.Dep
	for _, dep := range g[id] {
		deps = append(deps, beads.Dep{IssueID: id, DependsOnID: dep, Type: "blocks"})
	}
	return deps, nil
}

// fakeDepEdge is one labeled edge in a mixed-type dep graph.
type fakeDepEdge struct {
	to  string
	typ string
}

// fakeTypedDepGraph maps a bead ID to outgoing edges with explicit dep types.
type fakeTypedDepGraph map[string][]fakeDepEdge

func (g fakeTypedDepGraph) DepList(id, direction string) ([]beads.Dep, error) {
	if direction != "down" {
		return nil, nil
	}
	var deps []beads.Dep
	for _, e := range g[id] {
		deps = append(deps, beads.Dep{IssueID: id, DependsOnID: e.to, Type: e.typ})
	}
	return deps, nil
}

// errDepLister always returns an error from DepList.
type errDepLister struct{ err error }

func (e errDepLister) DepList(_, _ string) ([]beads.Dep, error) { return nil, e.err }

// --- DetectCycle unit tests ---

func TestDetectCycleNoCycle_Linear(t *testing.T) {
	// a → b → c (no cycle)
	g := fakeDepGraph{"a": {"b"}, "b": {"c"}, "c": nil}
	if err := DetectCycle("a", g); err != nil {
		t.Fatalf("expected no cycle, got %v", err)
	}
}

func TestDetectCycleNoCycle_Empty(t *testing.T) {
	g := fakeDepGraph{"a": nil}
	if err := DetectCycle("a", g); err != nil {
		t.Fatalf("expected no cycle for isolated node, got %v", err)
	}
}

func TestDetectCycleNoCycle_Diamond(t *testing.T) {
	// a → {b, c}, b → d, c → d — shared dep, no cycle
	g := fakeDepGraph{"a": {"b", "c"}, "b": {"d"}, "c": {"d"}, "d": nil}
	if err := DetectCycle("a", g); err != nil {
		t.Fatalf("expected no cycle in diamond graph, got %v", err)
	}
}

func TestDetectCycleDirect(t *testing.T) {
	// a → b → a
	g := fakeDepGraph{"a": {"b"}, "b": {"a"}}
	err := DetectCycle("a", g)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	var ce *CycleError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *CycleError, got %T: %v", err, err)
	}
	if len(ce.Path) < 2 {
		t.Fatalf("cycle path too short: %v", ce.Path)
	}
	if ce.Path[0] != ce.Path[len(ce.Path)-1] {
		t.Errorf("cycle path should start and end at same node; got %v", ce.Path)
	}
}

func TestDetectCycleIndirect(t *testing.T) {
	// a → b → c → a
	g := fakeDepGraph{"a": {"b"}, "b": {"c"}, "c": {"a"}}
	err := DetectCycle("a", g)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	var ce *CycleError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *CycleError, got %T: %v", err, err)
	}
	if ce.Path[0] != ce.Path[len(ce.Path)-1] {
		t.Errorf("cycle path should close; got %v", ce.Path)
	}
}

func TestDetectCycleSelfLoop(t *testing.T) {
	// a → a
	g := fakeDepGraph{"a": {"a"}}
	err := DetectCycle("a", g)
	if err == nil {
		t.Fatal("expected cycle error for self-loop, got nil")
	}
	var ce *CycleError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *CycleError, got %T: %v", err, err)
	}
}

func TestDetectCycleSkipsNonSchedulingDeps(t *testing.T) {
	// a -relates-to-> b, b -blocks-> a.
	// The "relates-to" edge from a is skipped, so DFS from "a" never enters "b",
	// and the cycle b→a is never reachable from "a".
	g := fakeTypedDepGraph{
		"a": {{to: "b", typ: "relates-to"}},
		"b": {{to: "a", typ: "blocks"}},
	}
	if err := DetectCycle("a", g); err != nil {
		t.Fatalf("relates-to dep should not trigger cycle detection; got %v", err)
	}
}

func TestDetectCycleWaitsForType(t *testing.T) {
	// waits-for is a scheduling dep and must be cycle-sensitive
	g := fakeTypedDepGraph{
		"a": {{to: "b", typ: "waits-for"}},
		"b": {{to: "a", typ: "waits-for"}},
	}
	if err := DetectCycle("a", g); err == nil {
		t.Fatal("expected cycle error for waits-for dep cycle, got nil")
	}
}

func TestDetectCycleErrorPropagation(t *testing.T) {
	dl := errDepLister{err: errors.New("store down")}
	err := DetectCycle("a", dl)
	if err == nil {
		t.Fatal("expected error from DepList failure, got nil")
	}
}

func TestCycleErrorMessage(t *testing.T) {
	g := fakeDepGraph{"x": {"y"}, "y": {"x"}}
	err := DetectCycle("x", g)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "x") || !strings.Contains(msg, "y") {
		t.Errorf("error message %q should name the cycle nodes", msg)
	}
	if !strings.Contains(msg, "→") {
		t.Errorf("error message %q should use → separator", msg)
	}
}

func TestCycleErrorImplementsError(t *testing.T) {
	ce := &CycleError{Path: []string{"a", "b", "a"}}
	// Verify it satisfies the error interface.
	var _ error = ce
	if ce.Error() == "" {
		t.Error("CycleError.Error() should return non-empty string")
	}
}

// --- execution/hierarchy family separation ---

// executionDepTypes lists every execution-family dependency type, including
// the legacy empty type.
var executionDepTypes = []string{"blocks", "waits-for", "conditional-blocks", ""}

// failingDepGraph serves graph but fails DepList for failID.
type failingDepGraph struct {
	graph  fakeTypedDepGraph
	failID string
	err    error
}

func (g failingDepGraph) DepList(id, direction string) ([]beads.Dep, error) {
	if id == g.failID {
		return nil, g.err
	}
	return g.graph.DepList(id, direction)
}

// ring links ids into one cycle of typ edges: ids[0] → ids[1] → … → ids[0].
func ring(typ string, ids ...string) fakeTypedDepGraph {
	g := fakeTypedDepGraph{}
	for i, id := range ids {
		g[id] = append(g[id], fakeDepEdge{to: ids[(i+1)%len(ids)], typ: typ})
	}
	return g
}

func depTypeName(typ string) string {
	if typ == "" {
		return "legacy-empty"
	}
	return typ
}

// requireCyclePath asserts that err is a *CycleError reporting exactly want.
func requireCyclePath(t *testing.T, err error, want ...string) {
	t.Helper()
	var ce *CycleError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *CycleError %v, got %T: %v", want, err, err)
	}
	if !slices.Equal(ce.Path, want) {
		t.Fatalf("cycle path = %v, want %v", ce.Path, want)
	}
}

func TestDetectCycleAcceptsMixedFamilyLoops(t *testing.T) {
	graphs := map[string]fakeTypedDepGraph{
		"grandparent waits on grandchild": {
			"g": {{to: "c", typ: "waits-for"}},
			"c": {{to: "p", typ: "parent-child"}},
			"p": {{to: "g", typ: "parent-child"}},
		},
		"epic waits on chained children": {
			"epic": {{to: "t2", typ: "waits-for"}},
			"t2":   {{to: "t1", typ: "blocks"}, {to: "epic", typ: "parent-child"}},
			"t1":   {{to: "epic", typ: "parent-child"}},
		},
		"parent waits on child blocked by sibling": {
			"p":  {{to: "t1", typ: "blocks"}},
			"t1": {{to: "t2", typ: "conditional-blocks"}, {to: "p", typ: "parent-child"}},
			"t2": {{to: "p", typ: "parent-child"}},
		},
	}
	for _, typ := range executionDepTypes {
		graphs["parent waits on child via "+depTypeName(typ)] = fakeTypedDepGraph{
			"parent": {{to: "child", typ: typ}},
			"child":  {{to: "parent", typ: "parent-child"}},
		}
	}
	for name, g := range graphs {
		t.Run(name, func(t *testing.T) {
			for start := range g {
				if err := DetectCycle(start, g); err != nil {
					t.Fatalf("DetectCycle(%q) = %v, want nil", start, err)
				}
			}
		})
	}
}

func TestDetectCycleRefusesSingleFamilyCycles(t *testing.T) {
	for _, typ := range append(slices.Clone(executionDepTypes), "parent-child") {
		t.Run(depTypeName(typ), func(t *testing.T) {
			requireCyclePath(t, DetectCycle("a", ring(typ, "a", "b")), "a", "b", "a")
			requireCyclePath(t, DetectCycle("a", ring(typ, "a", "b", "c")), "a", "b", "c", "a")
			requireCyclePath(t, DetectCycle("a", ring(typ, "a")), "a", "a")
		})
	}
	t.Run("mixed execution types", func(t *testing.T) {
		g := fakeTypedDepGraph{
			"a": {{to: "b", typ: "blocks"}},
			"b": {{to: "c", typ: "waits-for"}},
			"c": {{to: "d", typ: "conditional-blocks"}},
			"d": {{to: "a", typ: ""}},
		}
		requireCyclePath(t, DetectCycle("a", g), "a", "b", "c", "d", "a")
	})
}

func TestDetectCycleRefusesCyclesReachableThroughOppositeFamily(t *testing.T) {
	viaParent := fakeTypedDepGraph{
		"start": {{to: "p", typ: "parent-child"}},
		"p":     {{to: "q", typ: "blocks"}},
		"q":     {{to: "p", typ: "waits-for"}},
	}
	requireCyclePath(t, DetectCycle("start", viaParent), "p", "q", "p")

	viaBlocker := fakeTypedDepGraph{
		"start": {{to: "x", typ: "blocks"}},
		"x":     {{to: "y", typ: "parent-child"}},
		"y":     {{to: "x", typ: "parent-child"}},
	}
	requireCyclePath(t, DetectCycle("start", viaBlocker), "x", "y", "x")
}

func TestDetectCycleRefusesRealCycleInsideMixedGraph(t *testing.T) {
	execution := fakeTypedDepGraph{
		"parent":  {{to: "child", typ: "waits-for"}},
		"child":   {{to: "parent", typ: "parent-child"}, {to: "sibling", typ: "blocks"}},
		"sibling": {{to: "child", typ: "blocks"}},
	}
	requireCyclePath(t, DetectCycle("parent", execution), "child", "sibling", "child")

	hierarchy := fakeTypedDepGraph{
		"parent": {{to: "child", typ: "waits-for"}, {to: "child", typ: "parent-child"}},
		"child":  {{to: "parent", typ: "parent-child"}},
	}
	requireCyclePath(t, DetectCycle("parent", hierarchy), "parent", "child", "parent")
}

func TestDetectCycleIgnoresInformationalEdges(t *testing.T) {
	for _, typ := range []string{"relates-to", "tracks"} {
		t.Run(typ, func(t *testing.T) {
			graphs := []fakeTypedDepGraph{
				// An informational edge never closes an execution or hierarchy cycle.
				{"a": {{to: "b", typ: "blocks"}}, "b": {{to: "a", typ: typ}}},
				{"a": {{to: "b", typ: "parent-child"}}, "b": {{to: "a", typ: typ}}},
				// A cycle reachable only through an informational edge is out of scope.
				{"a": {{to: "b", typ: typ}}, "b": {{to: "c", typ: "blocks"}}, "c": {{to: "b", typ: "blocks"}}},
			}
			for i, g := range graphs {
				if err := DetectCycle("a", g); err != nil {
					t.Errorf("graph %d: DetectCycle = %v, want nil", i, err)
				}
			}
		})
	}
}

func TestDetectCycleAcceptsAcyclicDiamonds(t *testing.T) {
	for _, typ := range append(slices.Clone(executionDepTypes), "parent-child") {
		g := fakeTypedDepGraph{
			"a": {{to: "b", typ: typ}, {to: "c", typ: typ}},
			"b": {{to: "d", typ: typ}},
			"c": {{to: "d", typ: typ}},
		}
		if err := DetectCycle("a", g); err != nil {
			t.Errorf("%s diamond: DetectCycle = %v, want nil", depTypeName(typ), err)
		}
	}
	mixed := fakeTypedDepGraph{
		"a": {{to: "b", typ: "blocks"}, {to: "c", typ: "parent-child"}, {to: "d", typ: "waits-for"}, {to: "d", typ: "parent-child"}},
		"b": {{to: "d", typ: "parent-child"}},
		"c": {{to: "d", typ: "conditional-blocks"}},
	}
	if err := DetectCycle("a", mixed); err != nil {
		t.Errorf("mixed diamond: DetectCycle = %v, want nil", err)
	}
}

func TestDetectCycleFailsClosedOnReadErrors(t *testing.T) {
	errStore := errors.New("store down")
	graphs := map[string]fakeTypedDepGraph{
		"reachable only through hierarchy": {
			"start": {{to: "p", typ: "parent-child"}},
		},
		"behind a valid mixed loop": {
			"start": {{to: "child", typ: "waits-for"}},
			"child": {{to: "start", typ: "parent-child"}, {to: "p", typ: "blocks"}},
		},
	}
	for name, g := range graphs {
		t.Run(name, func(t *testing.T) {
			err := DetectCycle("start", failingDepGraph{graph: g, failID: "p", err: errStore})
			if !errors.Is(err, errStore) {
				t.Fatalf("DetectCycle = %v, want wrapped %v", err, errStore)
			}
			if !strings.Contains(err.Error(), "reading dependencies of p") {
				t.Errorf("error %q should name the unreadable bead", err)
			}
		})
	}
}

func TestDetectCycleDiagnosticsAreDeterministic(t *testing.T) {
	// Execution cycles a⇄b and a⇄c and hierarchy cycle a⇄h share bead a.
	// Whatever order the store lists dependencies in, the execution family is
	// checked first and dependencies are visited in bead ID order.
	forward := fakeTypedDepGraph{
		"a": {{to: "b", typ: "blocks"}, {to: "c", typ: "blocks"}, {to: "h", typ: "parent-child"}},
		"b": {{to: "a", typ: "blocks"}},
		"c": {{to: "a", typ: "blocks"}},
		"h": {{to: "a", typ: "parent-child"}},
	}
	reversed := fakeTypedDepGraph{}
	for id, edges := range forward {
		reversed[id] = slices.Clone(edges)
		slices.Reverse(reversed[id])
	}
	for _, g := range []fakeTypedDepGraph{forward, reversed} {
		requireCyclePath(t, DetectCycle("a", g), "a", "b", "a")
	}
}

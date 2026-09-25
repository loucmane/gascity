package sling

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gastownhall/gascity/internal/beads"
)

// DepLister can enumerate the direct dependencies of a bead.
// The "down" direction returns what the given bead depends on;
// the "up" direction returns what depends on it.
// beads.Store satisfies this interface.
type DepLister interface {
	DepList(id, direction string) ([]beads.Dep, error)
}

// CycleError is returned when a dependency cycle is detected at sling time.
// Path contains the cycle, with the first and last element being the same
// bead ID (e.g. ["a", "b", "c", "a"]).
type CycleError struct {
	Path []string
}

func (e *CycleError) Error() string {
	return fmt.Sprintf("dependency cycle detected: %s", strings.Join(e.Path, " → "))
}

// DetectCycle returns a CycleError if routing startID would depend on a
// cycle that can never make progress, or nil otherwise. It first collects
// every bead reachable from startID through execution or hierarchy
// dependencies, then searches each family for a cycle independently, so a
// cycle reachable only through the other family is still found.
//
// Execution dependencies ("blocks", "waits-for", "conditional-blocks", and
// the legacy empty type) order work; hierarchy dependencies ("parent-child")
// record structure. A loop that mixes the two families is valid: a parent may
// wait on its own child. Informational types ("relates-to", "tracks") are
// skipped. A failure to read any collected bead's dependencies is returned
// rather than treated as an acyclic graph.
//
// The reported cycle is deterministic: execution cycles are reported before
// hierarchy cycles, and dependencies are visited in bead ID order.
func DetectCycle(startID string, dl DepLister) error {
	g, err := collectRoutingDeps(startID, dl)
	if err != nil {
		return err
	}
	for _, edges := range []map[string][]string{g.execution, g.hierarchy} {
		if path := findCycle(g.nodes, edges); path != nil {
			return &CycleError{Path: path}
		}
	}
	return nil
}

// routingDeps is the dependency graph reachable from a sling target, split
// into its execution and hierarchy families.
type routingDeps struct {
	nodes     []string            // discovery order, starting with the sling target
	execution map[string][]string // bead ID → sorted execution dependency IDs
	hierarchy map[string][]string // bead ID → sorted parent IDs
}

// collectRoutingDeps walks execution and hierarchy dependencies breadth-first
// from startID, reading each reachable bead's dependencies exactly once.
func collectRoutingDeps(startID string, dl DepLister) (routingDeps, error) {
	g := routingDeps{execution: map[string][]string{}, hierarchy: map[string][]string{}}
	seen := map[string]bool{startID: true}
	queue := []string{startID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		g.nodes = append(g.nodes, id)
		deps, err := dl.DepList(id, "down")
		if err != nil {
			return routingDeps{}, fmt.Errorf("reading dependencies of %s: %w", id, err)
		}
		var reached []string
		for _, d := range deps {
			switch {
			case isExecutionDep(d.Type):
				g.execution[id] = append(g.execution[id], d.DependsOnID)
			case isHierarchyDep(d.Type):
				g.hierarchy[id] = append(g.hierarchy[id], d.DependsOnID)
			default:
				continue
			}
			reached = append(reached, d.DependsOnID)
		}
		g.execution[id] = sortedUnique(g.execution[id])
		g.hierarchy[id] = sortedUnique(g.hierarchy[id])
		for _, next := range sortedUnique(reached) {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return g, nil
}

// isExecutionDep reports whether a dependency type orders execution: the
// ready-blocking types plus the legacy empty type.
func isExecutionDep(depType string) bool {
	return depType == "" || beads.IsReadyBlockingDependencyType(depType)
}

// isHierarchyDep reports whether a dependency type links a child to its parent.
func isHierarchyDep(depType string) bool {
	return depType == "parent-child"
}

func sortedUnique(ids []string) []string {
	slices.Sort(ids)
	return slices.Compact(ids)
}

// findCycle runs a three-color depth-first search along edges from each of
// nodes in order and returns the first cycle found, or nil if edges are
// acyclic.
func findCycle(nodes []string, edges map[string][]string) []string {
	const (
		white = iota // unvisited
		gray         // on the current search path
		black        // fully explored
	)
	color := make(map[string]int, len(nodes))
	parent := map[string]string{}

	var visit func(id string) []string
	visit = func(id string) []string {
		color[id] = gray
		for _, next := range edges[id] {
			switch color[next] {
			case gray:
				return buildCyclePath(parent, id, next)
			case white:
				parent[next] = id
				if path := visit(next); path != nil {
					return path
				}
			}
		}
		color[id] = black
		return nil
	}

	for _, id := range nodes {
		if color[id] == white {
			if path := visit(id); path != nil {
				return path
			}
		}
	}
	return nil
}

// buildCyclePath reconstructs the cycle as a human-readable slice.
// It walks the parent map from cycleEntry back to cycleEntry, then appends
// cycleEntry again so the first and last elements are the same.
func buildCyclePath(parent map[string]string, fromID, cycleEntry string) []string {
	// Walk from fromID back to cycleEntry using parent pointers.
	var chain []string
	cur := fromID
	for cur != cycleEntry {
		chain = append(chain, cur)
		p, ok := parent[cur]
		if !ok {
			break
		}
		cur = p
	}
	chain = append(chain, cycleEntry)

	// Reverse so the path reads start → ... → cycleEntry.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	// Append cycleEntry again to close the loop visually.
	return append(chain, cycleEntry)
}

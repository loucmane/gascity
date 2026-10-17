package main

import (
	"errors"
	"fmt"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
)

// orderRetentionGuard is a per-sweep, live-read projection of the portion of
// the ownership graph that can reach non-closed work. Closed dead branches are
// irrelevant to the stranding decision and are never loaded. This is private
// to retention: other workflow deletes keep their existing guard and detailed
// descendant diagnostics (retention logs only the retained root IDs).
type orderRetentionGuard struct {
	nodes      map[string]beads.Bead
	members    map[string][]string
	parents    map[string][]string
	graph      map[string][]string
	parentIDs  map[string][]string
	projection beads.ParentProjection
	deleted    map[string]bool
}

// newOrderRetentionGuard performs one non-closed List across both tiers and at
// most one Get per distinct (possibly replacement) ancestor, including negative
// lookups. Stamped children need graph edges; dependency-backed parent readers
// also need structural edges. Reuse complete List dependencies when promised
// by the reader, otherwise read DepList("down") once per visited child.
// Every read completes before retention starts deleting; any error aborts the
// sweep. The projection is scoped to one run, never a closed-history cache.
func newOrderRetentionGuard(store beads.Store) (*orderRetentionGuard, error) {
	handles := beads.HandlesFor(store)
	reader := handles.Live
	open, err := reader.List(beads.ListQuery{AllowScan: true, TierMode: beads.TierBoth})
	if err != nil {
		return nil, fmt.Errorf("listing non-closed beads for retention: %w", err)
	}
	// A proxy may demote from native to CLI reads during List. Only reuse
	// inline edges when both sides of that read promise their completeness.
	completeOpenDeps := handles.ListDependenciesComplete && beads.HandlesFor(store).ListDependenciesComplete
	g := &orderRetentionGuard{
		nodes:      make(map[string]beads.Bead),
		members:    make(map[string][]string),
		parents:    make(map[string][]string),
		graph:      make(map[string][]string),
		parentIDs:  make(map[string][]string),
		projection: handles.Parents,
		deleted:    make(map[string]bool),
	}
	loaded := make(map[string]bool)
	listed := make(map[string]bool)
	var queue []beads.Bead
	for _, b := range open {
		if b.ID == "" || b.Status == "closed" || loaded[b.ID] {
			continue
		}
		loaded[b.ID] = true
		listed[b.ID] = true
		g.nodes[b.ID] = b
		queue = append(queue, b)
		root := b.Metadata[beadmeta.RootBeadIDMetadataKey]
		g.members[root] = append(g.members[root], b.ID)
	}
	load := func(id string) (beads.Bead, error) {
		if !loaded[id] {
			b, err := reader.Get(id)
			if err != nil && !errors.Is(err, beads.ErrNotFound) {
				return beads.Bead{}, fmt.Errorf("getting retention ancestor %s: %w", id, err)
			}
			loaded[id] = true
			if err == nil {
				g.nodes[id] = b
				queue = append(queue, b)
			}
		}
		return g.nodes[id], nil
	}
	for i := 0; i < len(queue); i++ {
		b := queue[i]
		var deps []beads.Dep
		if g.projection != beads.ParentField || b.Metadata[beadmeta.RootBeadIDMetadataKey] != "" {
			if listed[b.ID] && completeOpenDeps {
				for _, dep := range b.Dependencies {
					if dep.IssueID == b.ID {
						deps = append(deps, dep)
					}
				}
			} else {
				var err error
				deps, err = reader.DepList(b.ID, "down")
				if err != nil {
					return nil, fmt.Errorf("listing retention ancestor edges of %s: %w", b.ID, err)
				}
			}
		}
		// Include potential replacement parents in the read closure too.
		// Native/BdStore project one dependency parent; DoltLite matches all
		// parents in SQL. Dotted IDs alone never invent a structural edge.
		g.parentIDs[b.ID] = retentionStructuralParents(b, deps, g.projection)
		for _, parentID := range g.parentIDs[b.ID] {
			g.parents[parentID] = append(g.parents[parentID], b.ID)
			if _, err := load(parentID); err != nil {
				return nil, err
			}
		}
		if b.Metadata[beadmeta.RootBeadIDMetadataKey] == "" {
			continue
		}
		for _, dep := range deps {
			if dep.DependsOnID == "" || !isOrderWispDescendantDepType(dep.Type) {
				continue
			}
			parent, err := load(dep.DependsOnID)
			if err != nil {
				return nil, err
			}
			if parent.ID != "" && orderWispMayHaveGraphDependents(parent) {
				g.graph[parent.ID] = append(g.graph[parent.ID], b.ID)
			}
		}
	}
	return g, nil
}

func retentionStructuralParents(b beads.Bead, deps []beads.Dep, projection beads.ParentProjection) []string {
	var ids []string
	seen := make(map[string]bool)
	add := func(id string) {
		if id != "" && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	add(b.ParentID)
	if projection == beads.ParentField {
		return ids
	}
	// Inline dependencies preserve the native conversion's parent order.
	// DepList supplies the complete set on adapters with sparse list rows.
	for i, edges := range [][]beads.Dep{b.Dependencies, deps} {
		for _, dep := range edges {
			if i == 0 && dep.IssueID != b.ID {
				continue
			}
			if dep.Type == "parent-child" {
				add(dep.DependsOnID)
			}
		}
	}
	return ids
}

func (g *orderRetentionGuard) hasParent(childID, parentID string) bool {
	for _, id := range g.parentIDs[childID] {
		if g.projection != beads.ParentField && g.deleted[id] {
			continue
		}
		if id == parentID {
			return true
		}
		if g.projection != beads.ParentAllDependencies {
			return false
		}
	}
	return false
}

// check uses the same membership-first and root-specific graph predicates as
// storeOpenDescendantIDs. Omitting dead branches preserves reachability, but
// may change which tree witness is encountered first. Retention consumes only
// the refusal class and logs root IDs; detailed per-bead callers are unchanged.
func (g *orderRetentionGuard) check(rootID string, alsoDeleting map[string]struct{}) error {
	skip := workflowDeleteSkip(alsoDeleting)
	var members []string
	for _, id := range g.members[rootID] {
		if b, ok := g.nodes[id]; ok && id != rootID && !skip(b) {
			members = append(members, id)
		}
	}
	if len(members) > 0 {
		return workflowDeleteRefusal(rootID, members)
	}
	seen := map[string]bool{rootID: true}
	queue := []string{rootID}
	for i := 0; i < len(queue); i++ {
		parentID := queue[i]
		for kind, children := range [][]string{g.parents[parentID], g.graph[parentID]} {
			for _, id := range children {
				b, ok := g.nodes[id]
				if !ok || seen[id] {
					continue
				}
				if kind == 0 && !g.hasParent(id, parentID) || kind == 1 && !orderWispGraphDependentOwnedByRoot(b, rootID) {
					continue
				}
				seen[id] = true
				if b.Status != "closed" && !skip(b) {
					return workflowDeleteRefusal(rootID, []string{id})
				}
				queue = append(queue, id)
			}
		}
	}
	return nil
}

func (g *orderRetentionGuard) delete(store beads.Store, id string) error {
	if err := g.check(id, nil); err != nil {
		return err
	}
	if err := deleteWorkflowBeadUnguarded(store, id); err != nil {
		return err
	}
	// A safe root can be an intermediate in another root's graph-owned path.
	// Do not let its now-unwound edges keep later candidates artificially live.
	delete(g.nodes, id)
	delete(g.graph, id)
	g.deleted[id] = true
	return nil
}

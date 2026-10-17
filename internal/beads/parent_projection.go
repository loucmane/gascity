package beads

// ParentProjection describes how a store projects parent-child dependencies
// into its List(ParentID) relation.
type ParentProjection int

const (
	// ParentField uses the independently stored Bead.ParentID field.
	ParentField ParentProjection = iota
	// ParentFirstDependency uses the first parent-child dependency projected
	// into ParentID; removing it can reveal the next dependency parent.
	ParentFirstDependency
	// ParentAllDependencies matches every parent-child dependency, as in a
	// SQL join filtered by its target before deduplication by bead ID.
	ParentAllDependencies
)

func parentProjectionFor(store Store) ParentProjection {
	if p, ok := store.(interface{ ParentProjection() ParentProjection }); ok {
		return p.ParentProjection()
	}
	return ParentField
}

func completeListDependenciesFor(store Store) bool {
	complete, ok := store.(listDependencyCompletenessStore)
	return ok && complete.listIncludesCompleteDependencies()
}

// ParentProjection reports the native adapter's resolved-parent filter.
func (s *NativeDoltStore) ParentProjection() ParentProjection { return ParentFirstDependency }

// ParentProjection reports the CLI adapter's resolved-parent filter.
func (s *BdStore) ParentProjection() ParentProjection { return ParentFirstDependency }

// ParentProjection follows the leaf serving proxied reads.
func (s *ProxiedStore) ParentProjection() ParentProjection {
	return parentProjectionFor(s.readLeaf())
}

// ParentProjection follows the cache's live backing reader.
func (c *CachingStore) ParentProjection() ParentProjection {
	return parentProjectionFor(c.backing)
}

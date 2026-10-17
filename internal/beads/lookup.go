package beads

import "fmt"

// LookupByID probes an identifier that may be a name rather than a bead ID.
// On a serving proxied store the exact, both-tier List is authoritative and
// avoids Get's speculative CLI fallback on a native miss. Other stores retain
// their Get behavior. The query goes through the caller's wrappers so their
// routing, cache and error handling remain in force.
func LookupByID(store Store, id string) (Bead, error) {
	view, proxied := ProxiedStoreFrom(store)
	if !proxied || view.Demoted() {
		return store.Get(id)
	}
	if guard, ok := view.BdLeaf().(relocatedClassGuard); ok {
		if err := guard.guardRelocatedClassIDs("get "+id, id); err != nil {
			return Bead{}, err
		}
	}
	rows, err := store.List(ListQuery{IDs: []string{id}, IncludeClosed: true, TierMode: TierBoth})
	if err != nil {
		return Bead{}, err
	}
	for _, row := range rows {
		if row.ID == id {
			return row, nil
		}
	}
	return Bead{}, fmt.Errorf("bead %q: %w", id, ErrNotFound)
}

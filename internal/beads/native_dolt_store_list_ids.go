package beads

import (
	"cmp"
	"context"
	"slices"
	"strings"

	beadslib "github.com/steveyegge/beads"
)

// Keep ID predicates well below MySQL's 65,535-placeholder ceiling, leaving
// room for the other filters and bounding each search/hydration working set.
const nativeListIDChunkSize = 500

func nativeSearchIssuesByIDs(ctx context.Context, storage beadslib.Storage, filter beadslib.IssueFilter) ([]*beadslib.Issue, error) {
	if len(filter.IDs) <= nativeListIDChunkSize {
		return storage.SearchIssues(ctx, "", filter)
	}
	// IDs are exact strings: the empty string is a predicate too. Dropping it
	// could turn an all-empty ID query into an unrestricted scan. Deduplicate
	// without modifying the caller's slice or repeating work across chunks.
	ids := make([]string, 0, len(filter.IDs))
	seenIDs := make(map[string]bool, len(filter.IDs))
	for _, id := range filter.IDs {
		if !seenIDs[id] {
			ids = append(ids, id)
			seenIDs[id] = true
		}
	}
	// ApplyListQuery owns the final page over the merged candidates. Per-chunk
	// limits could discard rows before residual filters or the global ordering.
	filter.Limit = 0
	var issues []*beadslib.Issue
	seenRows := make(map[string]bool)
	for chunk := range slices.Chunk(ids, nativeListIDChunkSize) {
		filter.IDs = chunk
		rows, err := storage.SearchIssues(ctx, "", filter)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			// A database collation may match the same row in different chunks
			// even though the requested ID strings differ.
			if !seenRows[row.ID] {
				issues = append(issues, row)
				seenRows[row.ID] = true
			}
		}
	}
	// SortDefault leaves input order intact in ApplyListQuery, so restore the
	// pinned library's default (priority ASC, created_at DESC, id ASC) across
	// chunks. Explicit created sorts are reapplied by ApplyListQuery afterward.
	slices.SortFunc(issues, func(a, b *beadslib.Issue) int {
		if order := cmp.Compare(a.Priority, b.Priority); order != 0 {
			return order
		}
		if order := b.CreatedAt.Compare(a.CreatedAt); order != 0 {
			return order
		}
		return strings.Compare(a.ID, b.ID)
	})
	return issues, nil
}

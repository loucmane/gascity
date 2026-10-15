package beads

import (
	"fmt"
	"strings"

	"github.com/gastownhall/gascity/internal/beads/proxyendpoint"
)

// The schema cursors of the linked beads library. The drift test resolves the
// actual source, including the pinned local fork, and checks both migration
// directories so the compatibility gate cannot drift from the linked schema.
//
// Proxied opens use the fork's OpenReadOnlyFromConfig: schema initialization,
// migration and auto-start are disabled, and writes refuse. Cursor and sentinel
// checks establish read compatibility; the post-open HEAD/sentinel observation
// remains defense in depth.
const (
	// SchemaCursorMain is schema.LatestVersion() for the pinned library.
	SchemaCursorMain = 66
	// SchemaCursorIgnored is schema.LatestIgnoredVersion() for the pinned
	// library.
	SchemaCursorIgnored = 26
)

// PinnedSchemaCursors returns the schema versions required for native proxied
// reads. Both lanes must match exactly; repairing either lane belongs to bd.
func PinnedSchemaCursors() (main, ignored int) {
	return SchemaCursorMain, SchemaCursorIgnored
}

// CursorsMatchPinned compares the observed schema against the linked library
// and names the first mismatched lane, checking the main lane first.
//
// The ignored lane is compared using reality.EffectiveIgnored: missing tables
// or columns can contradict the raw cursor. These sentinel checks reject a
// schema that cannot serve the library's reads even if its recorded version is
// current. They use the same reality floors as the library's migration source;
// the read-only constructor never attempts to repair the discrepancy.
func CursorsMatchPinned(c proxyendpoint.Cursors, reality proxyendpoint.CursorReality) (ok bool, lane, dir string) {
	main, ignored := PinnedSchemaCursors()
	effectiveIgnored := reality.EffectiveIgnored(c.Ignored)
	switch {
	case c.Main > main:
		return false, ProxiedSkewLaneMain, ProxiedSkewDirAhead
	case c.Main < main:
		return false, ProxiedSkewLaneMain, ProxiedSkewDirBehind
	case effectiveIgnored > ignored:
		return false, ProxiedSkewLaneIgnored, ProxiedSkewDirAhead
	case effectiveIgnored < ignored:
		return false, ProxiedSkewLaneIgnored, ProxiedSkewDirBehind
	default:
		return true, "", ""
	}
}

// ProxiedOpenUnmoved checks HEAD and ignored-schema sentinel reality after a
// library open. The read-only constructor prevents repairs; this comparison
// additionally catches concurrent schema changes or a constructor regression.
//
// HEAD cannot observe changes to ignored tables, so the effective ignored cursor
// must also remain at the admitted value. A completed ignored-table rewrite
// that restores all observed sentinels is invisible to this comparison; write
// prevention therefore belongs to the constructor, not this check.
//
// A moved HEAD is non-terminal because another bd client may have committed
// during the admission window. The caller logs the observation, demotes this
// open, forgets admission memoization and re-probes on the next open. The
// comparison cannot attribute a concurrent commit to a specific writer.
//
// Missing hashes mean there is no comparison to make. Production fresh opens
// require both observations; memo-only pins intentionally carry no HEAD.
func ProxiedOpenUnmoved(pin Pin, observed proxyendpoint.PostOpenReport) error {
	before, after := strings.TrimSpace(pin.Head()), strings.TrimSpace(observed.Head)
	if before == "" || after == "" {
		return nil
	}
	if before != after {
		return NewProxiedVerdictError(ProxiedVerdictHeadMoved, fmt.Sprintf(
			"opening the linked library against database %q moved HEAD from %s to %s: "+
				"PR2's native lane serves reads only, so an open that commits may be gc writing to bd's database "+
				"(a writable library open can seed dolt_ignore, heal the tracked cursor table, or add a "+
				"content_hash column, each ending in a DOLT_COMMIT); this open takes bd's front door. "+
				"If another bd client committed in the same window this is that write and not gc's, "+
				"which is why the verdict is non-terminal and the next open re-probes",
			pin.Database(), before, after), nil)
	}
	admitted := pin.Cursors().Ignored
	if now := observed.EffectiveIgnored(admitted); now != admitted {
		return NewProxiedVerdictError(ProxiedVerdictHeadMoved, fmt.Sprintf(
			"opening the linked library against database %q left HEAD at %s but the ignored plane's effective "+
				"cursor at %d where admission found %d (%s): the dolt_ignore'd plane is never committed, so HEAD "+
				"cannot see it, and a plane the next writable open would replay is not one this leaf may serve; "+
				"this open takes bd's front door and the next open re-probes",
			pin.Database(), after, now, admitted, observed.IgnoredPlaneReason()), nil)
	}
	return nil
}

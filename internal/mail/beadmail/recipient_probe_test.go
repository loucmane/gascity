package beadmail

import (
	"reflect"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/session"
)

// These routes are pinned against the unrestricted resolver before changing
// its probes. Live ownership, closed ownership, and historical ownership must
// retain their precedence, including literal-only delivery on ambiguity.
func TestRecipientRoutesWithRestrictedMailboxProbes(t *testing.T) {
	corpus := []beads.Bead{
		{ID: "typed", Type: session.BeadType, Status: "open", Metadata: map[string]string{"alias": "rig/typed", "session_name": "runtime-typed", "alias_history": "old-typed"}},
		{ID: "repair", Labels: []string{session.LabelSession}, Status: "open", Metadata: map[string]string{"alias": "rig/repair", "session_name": "runtime-repair", "alias_history": "old-repair"}},
		{ID: "dual", Type: session.BeadType, Labels: []string{session.LabelSession}, Status: "open", Metadata: map[string]string{"alias": "rig/dual", "session_name": "rig/dual"}},
		{ID: "task", Type: "task", Labels: []string{session.LabelSession}, Status: "open", Metadata: map[string]string{"alias": "rig/typed", "session_name": "runtime-repair", "alias_history": "old-typed"}},
		{ID: "empty", Status: "open", Metadata: map[string]string{"alias": "rig/typed"}},
		{ID: "closed", Type: session.BeadType, Status: "closed", Metadata: map[string]string{"alias": "rig/typed", "session_name": "retired", "alias_history": "old-typed"}},
		{ID: "closed-repair", Labels: []string{session.LabelSession}, Status: "closed", Metadata: map[string]string{"alias": "rig/closed-repair", "alias_history": "old-closed"}},
		{ID: "history", Type: session.BeadType, Status: "open", Metadata: map[string]string{"alias": "rig/history", "alias_history": "retired"}},
		{ID: "ambiguous-a", Type: session.BeadType, Status: "open", Metadata: map[string]string{"alias": "rig/ambiguous", "alias_history": "old-ambiguous"}},
		{ID: "ambiguous-b", Labels: []string{session.LabelSession}, Status: "open", Metadata: map[string]string{"session_name": "rig/ambiguous", "alias_history": "old-ambiguous"}},
		{ID: "closed-a", Type: session.BeadType, Status: "closed", Metadata: map[string]string{"alias": "rig/closed-ambiguous"}},
		{ID: "closed-b", Type: session.BeadType, Status: "closed", Metadata: map[string]string{"session_name": "rig/closed-ambiguous"}},
		{ID: "wisp", Type: session.BeadType, Status: "open", Ephemeral: true, Metadata: map[string]string{"alias": "rig/wisp"}},
	}
	cases := []struct {
		recipient string
		want      []string
	}{
		{"rig/typed", []string{"rig/typed", "typed", "runtime-typed", "old-typed"}},
		{"  rig/repair  ", []string{"rig/repair", "repair", "runtime-repair", "old-repair"}},
		{"runtime-repair", []string{"runtime-repair", "repair", "rig/repair", "old-repair"}},
		{"rig/dual", []string{"rig/dual", "dual"}},
		{"retired", []string{"retired", "closed", "rig/typed", "old-typed"}},
		{"rig/closed-repair", []string{"rig/closed-repair", "closed-repair", "old-closed"}},
		{"old-typed", []string{"old-typed", "typed", "rig/typed", "runtime-typed"}},
		{"old-repair", []string{"old-repair", "repair", "rig/repair", "runtime-repair"}},
		{"old-closed", []string{"old-closed", "closed-repair", "rig/closed-repair"}},
		{"rig/ambiguous", []string{"rig/ambiguous"}},
		{"old-ambiguous", []string{"old-ambiguous"}},
		{"rig/closed-ambiguous", []string{"rig/closed-ambiguous"}},
		{"rig/wisp", []string{"rig/wisp", "wisp"}},
		{"rig/missing", []string{"rig/missing"}},
		{"human", []string{"human"}},
		{"  ", nil},
	}
	for _, tc := range cases {
		t.Run(tc.recipient, func(t *testing.T) {
			p := New(beads.NewMemStoreFrom(len(corpus), corpus, nil))
			if got := p.recipientRoutes(tc.recipient); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("recipientRoutes(%q) = %v, want %v", tc.recipient, got, tc.want)
			}
		})
	}
}

// The idle named mailbox path must keep every metadata read session-scoped,
// including the closed pass, then skip hydration on its historical census.
func TestRecipientRoutesMissingMailboxUsesBoundedProbes(t *testing.T) {
	rec := &recipientProbeStore{Store: beads.NewMemStore()}
	if got := New(rec).recipientRoutes("rig/missing"); !reflect.DeepEqual(got, []string{"rig/missing"}) {
		t.Fatalf("missing mailbox routes = %v", got)
	}
	var live, closed, historical int
	for _, q := range rec.queries {
		if q.Type != session.BeadType && q.Label != session.LabelSession {
			t.Errorf("unrestricted recipient lookup: %+v", q)
		}
		if !q.SkipDependencies {
			t.Errorf("recipient lookup hydrates unused dependencies: %+v", q)
		}
		switch {
		case len(q.Metadata) == 0:
			historical++
		case q.Status == "closed":
			closed++
		default:
			live++
		}
	}
	if live != 4 || closed != 4 || historical != 2 {
		t.Errorf("lookup legs: live=%d closed=%d historical=%d, want 4/4/2", live, closed, historical)
	}
}

type recipientProbeStore struct {
	beads.Store
	queries []beads.ListQuery
}

func (s *recipientProbeStore) List(q beads.ListQuery) ([]beads.Bead, error) {
	s.queries = append(s.queries, q)
	return s.Store.List(q)
}

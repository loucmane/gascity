package proxyendpoint

import (
	"context"
	"testing"

	"github.com/gastownhall/gascity/internal/beads/proxyendpoint/proxyendpointtest"
)

// Count actual statements issued by both gc-owned halves of one healthy open.
// The linked library's statements are recorded separately by the offline
// measurement harness; this fixture does not simulate a library open.
func TestOpenObservationStatementCount(t *testing.T) {
	probe := &fakeProbeConnector{cursors: map[string]int64{mainCursorQuery: 66, ignoredCursorQuery: 26}}
	if _, err := readCursorsOver(context.Background(), probe); err != nil {
		t.Fatal(err)
	}
	pool := proxyendpointtest.NewPostOpenDB(proxyendpointtest.State{Head: "head"}, proxyendpointtest.State{Head: "head"})
	t.Cleanup(func() { _ = pool.DB.Close() })
	if _, err := ReadPostOpen(context.Background(), pool.DB); err != nil {
		t.Fatal(err)
	}
	for i, statement := range probe.statements() {
		t.Logf("pre %02d: %s", i+1, statement)
	}
	for i, statement := range pool.Statements() {
		t.Logf("post %02d: %s", i+1, statement)
	}
	if len(probe.statements()) != 10 || len(pool.Statements()) != 2 {
		t.Fatalf("statements: pre=%d post=%d; want 10 + 2", len(probe.statements()), len(pool.Statements()))
	}
}

package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
)

// The existing hook contract excludes control beads from session back-references.
// Exercise the complete managed delivery path for both acquisition and adoption.
func TestReviewB2ManagedControlRemainsSessionFree(t *testing.T) {
	for _, minted := range []bool{false, true} {
		name := "adoption"
		if minted {
			name = "acquisition"
		}
		t.Run(name, func(t *testing.T) {
			v, _ := newVerifiedHookFixture(t)
			b := beads.Bead{ID: "control-check", Type: "task", Status: "in_progress", Assignee: v.assignee(), Revision: 1, Metadata: map[string]string{beadmeta.KindMetadataKey: "check", beadmeta.RoutedToMetadataKey: "worker", beadmeta.ClaimedAtMetadataKey: "2026-10-13T00:00:00Z"}}
			mem := beads.NewMemStoreFrom(1, []beads.Bead{b}, nil)
			installVerifiedWorkStore(t, v, mem)
			ops := hookClaimOps{
				PublishRunMap:         noopPublishRunMap,
				ResolveSessionWorkDir: func(string) string { return "" },
				StampWorkMeta: func(_ context.Context, _ string, _ []string, id, _ string, patch map[string]string) error {
					return mem.Update(id, beads.UpdateOpts{Metadata: patch})
				},
				ReadWorkMeta: func(_ context.Context, _ string, _ []string, id, _ string) (beads.Bead, error) { return mem.Get(id) },
			}
			bindVerifiedHookOps(&ops, v)
			opts := hookClaimOptions{Verified: v, Assignee: v.assignee(), Env: []string{"GC_SESSION_ID=" + v.ID, "GC_SESSION_NAME=" + v.SessionName}, JSON: true}
			var out, stderr bytes.Buffer
			if code := writeHookClaimWorkResultForBead(hookClaimJSONResult{Action: "work", BeadID: b.ID}, b, opts, ops, "", minted, &out, &stderr); code != 0 {
				t.Fatalf("delivery failed: %d %s %s", code, &out, &stderr)
			}
			live, err := mem.Get(b.ID)
			if err != nil {
				t.Fatal(err)
			}
			if live.Metadata[beadmeta.SessionIDMetadataKey] != "" || live.Metadata[beadmeta.SessionNameMetadataKey] != "" {
				t.Fatalf("control bead acquired session back-references: session_id=%q session_name=%q", live.Metadata[beadmeta.SessionIDMetadataKey], live.Metadata[beadmeta.SessionNameMetadataKey])
			}
		})
	}
}

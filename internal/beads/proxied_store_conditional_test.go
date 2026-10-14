package beads

import (
	"testing"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/rollout/gate"
)

// TestProxiedClaimWritesUseBdRevisionCAS proves the wrapper preserves the exact
// fenced CLI path used by session claims, work fences, release and continuation.
func TestProxiedClaimWritesUseBdRevisionCAS(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts UpdateOpts
	}{
		{"current claim", UpdateOpts{Metadata: map[string]string{beadmeta.CurrentClaimBeadIDMetadataKey: "work-1"}}},
		{"work fence", UpdateOpts{Metadata: map[string]string{beadmeta.ClaimIncarnationMetadataKey: "7:token", beadmeta.ClaimAttemptMetadataKey: "attempt"}}},
		{"drain release", UpdateOpts{Status: strPtr("open"), Assignee: strPtr(""), Metadata: map[string]string{beadmeta.ClaimAttemptMetadataKey: ""}}},
		{"pending unwind", UpdateOpts{Status: strPtr("open"), Assignee: strPtr(""), Metadata: map[string]string{beadmeta.ClaimIncarnationMetadataKey: "", beadmeta.ClaimAttemptMetadataKey: ""}}},
		{"continuation pin", UpdateOpts{Assignee: strPtr("session-1")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli := &scriptedBd{id: "ga-1", revision: 41, status: "in_progress"}
			bd := NewBdStore(t.TempDir(), cli.runner)
			native := newNativeDoltStoreForTest(newNativeDoltMemStorage(), WithProxiedReadOnly())
			split, err := NewProxiedStore(native, bd, PinForTest("/scope", "", "beads"))
			if err != nil {
				t.Fatal(err)
			}
			split.stampConditionalWritesMode(gate.Require, false)
			writer, ok := RequiredConditionalWriterFor(SessionStore{Store: split})
			if !ok {
				t.Fatal("typed wrapper lost required conditional writer")
			}
			if err := writer.UpdateIfMatch("ga-1", 41, tc.opts); err != nil {
				t.Fatal(err)
			}
			if err := writer.UpdateIfMatch("ga-1", 41, tc.opts); !IsPreconditionFailed(err) {
				t.Fatalf("stale claim write = %v, want precondition failure", err)
			}
			if cli.revision != 42 || cli.writeCalls != 2 {
				t.Fatalf("revision/writes = %d/%d, want 42/2", cli.revision, cli.writeCalls)
			}
			for _, args := range cli.writeArgv {
				if rev, fenced := parseIfRevisionArg(args); !fenced || rev != 41 {
					t.Fatalf("claim escaped CLI revision fence: %v", args)
				}
			}
			for key, value := range tc.opts.Metadata {
				if cli.metadata[key] != value {
					t.Errorf("metadata %q = %q, want %q", key, cli.metadata[key], value)
				}
			}
			if got := InspectConditionalWrites(split); !got.Capable || got.Probe != ConditionalWriteProbeCapable {
				t.Fatalf("write capability inspection = %+v", got)
			}
		})
	}
}

// TestProxiedUnsupportedBdNeverFallsBackToUnconditionalClaim keeps mandatory
// claims fail-closed when the deployed CLI has no revision fence.
func TestProxiedUnsupportedBdNeverFallsBackToUnconditionalClaim(t *testing.T) {
	cli := &scriptedBd{id: "ga-1", revision: 1, probeIncapable: true}
	bd := NewBdStore(t.TempDir(), cli.runner)
	split, err := NewProxiedStore(newNativeDoltStoreForTest(newNativeDoltMemStorage(), WithProxiedReadOnly()), bd, PinForTest("/scope", "", "beads"))
	if err != nil {
		t.Fatal(err)
	}
	split.stampConditionalWritesMode(gate.Require, false)
	if _, _, err := ResolveConditionalWriter(split); !IsConditionalWritesRequired(err) {
		t.Fatalf("require mode accepted incapable bd: %v", err)
	}
	writer, ok := RequiredConditionalWriterFor(split)
	if !ok {
		t.Fatal("the capability handle must preserve the leaf's typed refusal")
	}
	if err := writer.UpdateIfMatch("ga-1", 1, UpdateOpts{Assignee: strPtr("worker")}); !IsConditionalWriteUnsupported(err) {
		t.Fatalf("mandatory claim write = %v", err)
	}
	if cli.writeCalls != 0 {
		t.Fatalf("incapable bd received %d writes", cli.writeCalls)
	}
}

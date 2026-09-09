package providerledger

import "testing"

func TestTmuxConstructorHasBoundFullProof(t *testing.T) {
	for _, entry := range Catalog() {
		if entry.ID != "runtime.builtin.tmux" {
			continue
		}
		if len(entry.Claims) != 1 {
			t.Fatalf("tmux claims = %d, want one exact constructor", len(entry.Claims))
		}
		claim := entry.Claims[0]
		if claim.Constructor != repoSymbol("internal/runtime/tmux", "NewSeamBackedWithConfig") || claim.Disposition != DispositionProved || claim.Proof == nil || claim.Waiver != nil {
			t.Fatalf("tmux exact constructor must have a full proof, not a waiver: %+v", claim)
		}
		if claim.Proof.File != "internal/runtime/tmux/adapter_test.go" || claim.Proof.Test != "TestTmuxConformance" || claim.Proof.Runner != runtimeProviderRunner || claim.Proof.Scope != "" {
			t.Fatalf("tmux proof is missing, wrong or narrowed: %+v", claim.Proof)
		}
		if err := ValidateProofRefs(repoRoot(t), []Entry{entry}); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatal("tmux production row missing")
}

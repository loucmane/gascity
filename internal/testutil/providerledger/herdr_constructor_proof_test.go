package providerledger

import "testing"

func TestHerdrConstructorHasBoundFullProof(t *testing.T) {
	for _, entry := range Catalog() {
		if entry.ID != "runtime.builtin.herdr" {
			continue
		}
		if len(entry.Claims) != 1 {
			t.Fatalf("herdr claims = %d, want one exact constructor", len(entry.Claims))
		}
		claim := entry.Claims[0]
		if claim.Constructor != repoSymbol("internal/runtime/herdr", "New") || claim.Disposition != DispositionProved || claim.Proof == nil || claim.Waiver != nil {
			t.Fatalf("herdr exact constructor must have a full proof, not a waiver: %+v", claim)
		}
		if claim.Proof.File != "internal/runtime/herdr/conformance_test.go" || claim.Proof.Test != "TestHerdrConformance" || claim.Proof.Runner != runtimeProviderRunner || claim.Proof.Scope != "" {
			t.Fatalf("herdr proof is missing, wrong or narrowed: %+v", claim.Proof)
		}
		if err := ValidateProofRefs(repoRoot(t), []Entry{entry}); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatal("herdr production row missing")
}

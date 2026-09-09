package providerledger

import "testing"

func TestSSHConstructorHasBoundFullProof(t *testing.T) {
	for _, entry := range Catalog() {
		if entry.ID != "runtime.builtin.ssh" {
			continue
		}
		if len(entry.Claims) != 1 {
			t.Fatalf("SSH claims = %d, want one exact constructor", len(entry.Claims))
		}
		claim := entry.Claims[0]
		if claim.Constructor != repoSymbol("internal/runtime/ssh", "NewSeamBacked") || claim.Disposition != DispositionProved || claim.Proof == nil || claim.Waiver != nil {
			t.Fatalf("SSH exact constructor must have a full proof, not a waiver: %+v", claim)
		}
		if claim.Proof.File != "internal/runtime/ssh/conformance_test.go" || claim.Proof.Test != "TestSSHConformance" || claim.Proof.Runner != runtimeProviderRunner || claim.Proof.Scope != "" {
			t.Fatalf("SSH proof is missing, wrong or narrowed: %+v", claim.Proof)
		}
		if err := ValidateProofRefs(repoRoot(t), []Entry{entry}); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatal("SSH production row missing")
}

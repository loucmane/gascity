package providerledger

import "testing"

func TestK8sConstructorHasBoundFullProof(t *testing.T) {
	for _, entry := range Catalog() {
		if entry.ID != "runtime.builtin.k8s" {
			continue
		}
		if len(entry.Claims) != 1 {
			t.Fatalf("k8s claims = %d, want one exact constructor", len(entry.Claims))
		}
		claim := entry.Claims[0]
		if claim.Constructor != repoSymbol("internal/runtime/k8s", "NewSeamBacked") || claim.Disposition != DispositionProved || claim.Proof == nil || claim.Waiver != nil {
			t.Fatalf("k8s exact constructor must have a full proof, not a waiver: %+v", claim)
		}
		if claim.Proof.File != "internal/runtime/k8s/conformance_test.go" || claim.Proof.Test != "TestK8sConformance" || claim.Proof.Runner != runtimeProviderRunner || claim.Proof.Scope != "" {
			t.Fatalf("k8s proof is missing, wrong or narrowed: %+v", claim.Proof)
		}
		if err := ValidateProofRefs(repoRoot(t), []Entry{entry}); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatal("k8s production row missing")
}

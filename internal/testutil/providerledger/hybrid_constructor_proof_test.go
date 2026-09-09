package providerledger

import "testing"

func TestHybridSelectedConstructorHasBoundFullProof(t *testing.T) {
	for _, entry := range Catalog() {
		if entry.ID != "runtime.builtin.hybrid" {
			continue
		}
		if len(entry.Claims) != 1 {
			t.Fatalf("hybrid claims = %d, want one selected constructor", len(entry.Claims))
		}
		claim := entry.Claims[0]
		if claim.Constructor != repoSymbol("cmd/gc", "newHybridProvider") || claim.Disposition != DispositionProved || claim.Proof == nil || claim.Waiver != nil {
			t.Fatalf("hybrid selected constructor must have a full proof, not a waiver: %+v", claim)
		}
		if claim.Proof.File != "cmd/gc/hybrid_conformance_test.go" || claim.Proof.Test != "TestHybridConformance" || claim.Proof.Runner != runtimeProviderRunner || claim.Proof.Scope != "" {
			t.Fatalf("hybrid proof is missing, wrong or narrowed: %+v", claim.Proof)
		}
		// Each route separately owes the same complete, exact-constructor
		// source proof; the catalog's mixed-route suite is not a substitute.
		for _, name := range []string{"TestHybridConformance", "TestHybridLocalConformance", "TestHybridRemoteConformance"} {
			proof := *claim.Proof
			proof.Test = name
			copyEntry := entry
			copyEntry.Claims = append([]ContractClaim(nil), entry.Claims...)
			copyEntry.Claims[0].Proof = &proof
			if err := ValidateProofRefs(repoRoot(t), []Entry{copyEntry}); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	t.Fatal("hybrid production row missing")
}

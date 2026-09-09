package providerledger

import "testing"

func TestT3ConstructorHasBothBoundFullProofs(t *testing.T) {
	constructor := repoSymbol("internal/runtime/t3bridge", "NewSeamBacked")
	count := 0
	for _, entry := range Catalog() {
		for _, claim := range entry.Claims {
			if claim.Constructor != constructor {
				continue
			}
			count++
			if claim.Disposition != DispositionProved || claim.Proof == nil || claim.Waiver != nil {
				t.Errorf("%s T3 constructor still lacks full proof: %+v", entry.ID, claim)
				continue
			}
			if claim.Proof.File != "internal/runtime/t3bridge/conformance_test.go" || claim.Proof.Test != "TestT3SeamConformance" || claim.Proof.Runner != runtimeProviderRunner || claim.Proof.Scope != "" {
				t.Errorf("%s wrong or narrowed T3 proof: %+v", entry.ID, claim.Proof)
			}
			if err := ValidateProofRefs(repoRoot(t), []Entry{entry}); err != nil {
				t.Error(err)
			}
		}
	}
	if count != 2 {
		t.Fatalf("T3 exact constructor catalog rows = %d, want 2", count)
	}
}

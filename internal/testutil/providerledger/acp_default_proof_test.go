package providerledger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestACPDefaultConstructorHasBoundFullProof(t *testing.T) {
	constructor := repoSymbol("internal/runtime/acp", "NewSeamBacked")
	for _, entry := range Catalog() {
		if entry.ID != "runtime.builtin.acp" {
			continue
		}
		for _, claim := range entry.Claims {
			if claim.Constructor != constructor {
				continue
			}
			if claim.Disposition != DispositionProved || claim.Proof == nil || claim.Waiver != nil {
				t.Fatalf("ACP default constructor must have a full proof, not a waiver: %+v", claim)
			}
			if claim.Proof.File != "internal/runtime/acp/conformance_test.go" || claim.Proof.Test != "TestACPDefaultDirConformance" || claim.Proof.Runner != runtimeProviderRunner || claim.Proof.Scope != "" {
				t.Fatalf("ACP default proof has wrong test, runner or narrowed scope: %+v", claim.Proof)
			}
			if err := ValidateProofRefs(repoRoot(t), []Entry{entry}); err != nil {
				t.Fatalf("ACP constructor proof binding: %v", err)
			}
			doc, err := os.ReadFile(filepath.Join(repoRoot(t), "TESTING.md"))
			if err != nil {
				t.Fatal(err)
			}
			if err := CheckMarkdown(string(doc), Catalog()); err != nil {
				t.Fatalf("checked provider documentation drift: %v", err)
			}
			return
		}
	}
	t.Fatal("ACP default constructor is absent from its production catalog entry")
}

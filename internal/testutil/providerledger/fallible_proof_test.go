package providerledger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The source proof must bind the returned provider and its fail-closed error
// check to the very same constructor call, not merely find those names.
func TestValidateProofRefsFallibleConstructor(t *testing.T) {
	const valid = `p, err := k8s.NewSeamBacked()
if err != nil { tb.Fatal(err) }
return p, nil, "session"`
	tests := []struct {
		name, body string
		allow      bool
	}{
		{"exact checked constructor", valid, true},
		{"renamed bindings", `provider, failure := k8s.NewSeamBacked()
if failure != nil { tb.Fatal(failure) }
return provider, nil, tb.Name()`, true},
		{"ignored error", `p, _ := k8s.NewSeamBacked(); if nil != nil { tb.Fatal(nil) }; return p, nil, "s"`, false},
		{"blank provider", `_, err := k8s.NewSeamBacked(); if err != nil { tb.Fatal(err) }; return nil, nil, "s"`, false},
		{"duplicate binding", `p, p := k8s.NewSeamBacked(); if p != nil { tb.Fatal(p) }; return p, nil, "s"`, false},
		{"unchecked error", `p, err := k8s.NewSeamBacked(); return p, err, "s"`, false},
		{"wrong condition", strings.Replace(valid, "err != nil", "err == nil", 1), false},
		{"different error checked", strings.Replace(valid, "err != nil", "other != nil", 1), false},
		{"provider checked", strings.Replace(valid, "err != nil", "p != nil", 1), false},
		{"error only logged", strings.Replace(valid, "tb.Fatal(err)", "tb.Error(err)", 1), false},
		{"error skipped", strings.Replace(valid, "tb.Fatal(err)", "tb.Skip(err)", 1), false},
		{"wrong test receiver", strings.Replace(valid, "tb.Fatal(err)", "outer.Fatal(err)", 1), false},
		{"wrong error reported", strings.Replace(valid, "tb.Fatal(err)", "tb.Fatal(other)", 1), false},
		{"conditional return", strings.Replace(valid, "tb.Fatal(err)", "return p, nil, \"s\"", 1), false},
		{"extra branch", strings.Replace(valid, "{ tb.Fatal(err) }", "{ tb.Fatal(err) } else { return nil, nil, \"s\" }", 1), false},
		{"condition initializer", strings.Replace(valid, "if err != nil", "if other := err; other != nil", 1), false},
		{"substituted provider", strings.Replace(valid, "return p,", "return other,", 1), false},
		{"provider overwritten", strings.Replace(valid, "return p,", "p = other; return p,", 1), false},
		{"extra error action", strings.Replace(valid, "tb.Fatal(err)", "tb.Log(err); tb.Fatal(err)", 1), false},
		{"constructor replaced", strings.Replace(valid, "k8s.NewSeamBacked()", "k8s.NewProvider()", 1), false},
		{"second constructor", strings.Replace(valid, "return p, nil", "return p, k8s.NewSeamBacked()", 1), false},
		{"helper invocation", strings.Replace(valid, "return p, nil", "return p, helper()", 1), false},
		{"deferred mutation", "defer helper(); " + valid, false},
		{"nil shadow", strings.ReplaceAll(valid, "err", "nil"), false},
		{"test receiver shadow", strings.ReplaceAll(valid, "p", "tb"), false},
		{"closure return", strings.Replace(valid, "return p,", "return func() any { return p }(),", 1), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			source := `package fixture
import (
  k8s "github.com/gastownhall/gascity/internal/runtime/k8s"
  contract "github.com/gastownhall/gascity/internal/runtime/runtimetest"
  "testing"
)
func TestProof(outer *testing.T) {
  contract.RunProviderTests(outer, func(tb *testing.T) (any, any, string) {
` + tt.body + `
  })
}`
			if err := os.WriteFile(filepath.Join(root, "provider_test.go"), []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			entry := proofFixtureEntry("provider_test.go", "TestProof")
			constructor := repoSymbol("internal/runtime/k8s", "NewSeamBacked")
			entry.Constructors = []SymbolRef{constructor}
			entry.Claims[0].Constructor = constructor
			err := ValidateProofRefs(root, []Entry{entry})
			if tt.allow && err != nil {
				t.Fatalf("checked exact constructor refused: %v", err)
			}
			if !tt.allow && err == nil {
				t.Fatal("unsafe fallible constructor proof accepted")
			}
		})
	}
}

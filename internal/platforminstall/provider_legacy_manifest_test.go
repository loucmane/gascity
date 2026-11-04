package platforminstall

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gastownhall/gascity/internal/providerallowlist"
)

func TestLegacyCityProviderGateIgnoresUnrelatedManifest(t *testing.T) {
	for _, state := range []string{"malformed", "directory", "ordinary", "malformed unrelated key"} {
		t.Run(state, func(t *testing.T) {
			city := t.TempDir()
			path := DefaultManifestPath(city)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if state == "directory" {
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			} else {
				data := []byte("not-json")
				if state == "ordinary" {
					data = []byte(`{"unrelated":true}`)
				}
				if state == "malformed unrelated key" {
					data = []byte(`{"integrity":null,"\q":{"provider_allowlist":{}}}`)
				}
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			delegated, err := CityDelegatesProviderApproval(city)
			if delegated || err != nil {
				t.Fatalf("legacy city acquired a new manifest dependency: delegated=%v, %v", delegated, err)
			}
			if err := VerifyCityProviderAllowlist(city, ""); err != nil {
				t.Fatalf("legacy gate: %v", err)
			}
		})
	}
}

func TestApprovalInputsRequireValidUndelegatedManifest(t *testing.T) {
	for _, state := range []string{"empty", "empty integrity", "other city", "valid legacy"} {
		t.Run(state, func(t *testing.T) {
			city := t.TempDir()
			manifest := integrityManifest(t, city)
			if state == "other city" {
				manifest = integrityManifest(t, t.TempDir())
			}
			data, err := MarshalManifest(finalizeManifest(t, manifest))
			if err != nil {
				t.Fatal(err)
			}
			switch state {
			case "empty":
				data = []byte(`{}`)
			case "empty integrity":
				data = []byte(`{"integrity":{}}`)
			}
			writeProviderFixture(t, DefaultManifestPath(city), data)
			writeProviderFixture(t, providerallowlist.Path(city), []byte(`{"schema":"gc.provider-allowlist.v1","entries":[]}`))
			command, err := ResolveCityProviderLaunch(context.Background(), city, "", "legacy-command")
			if state == "valid legacy" {
				if err != nil || command != "legacy-command" {
					t.Fatalf("valid undelegated city changed: %q, %v", command, err)
				}
			} else if err == nil {
				t.Fatal("invalid manifest erased surviving approval before launch")
			}
		})
	}
}

func TestCityProviderGateRetainsMalformedOptIn(t *testing.T) {
	for _, item := range []struct {
		name, manifest, receipt string
	}{
		{"duplicate bad integrity", `{"integrity":{"provider_allowlist":null},"integrity":17}`, ""},
		{"bad integrity before opt in", `{"integrity":17,"integrity":{"provider_allowlist":null}}`, ""},
		{"array integrity before opt in", `{"integrity":[1,{}],"integrity":{"provider_allowlist":null}}`, ""},
		{"trailing manifest data", `{"integrity":{"provider_allowlist":null}} trailing`, ""},
		{"truncated manifest value", `{"integrity":{"provider_allowlist":`, ""},
		{"trailing receipt data", "", `{"provider_allowlist":null} trailing`},
		{"truncated receipt value", "", `{"provider_allowlist":`},
	} {
		t.Run(item.name, func(t *testing.T) {
			city := t.TempDir()
			for path, data := range map[string]string{
				DefaultManifestPath(city):           item.manifest,
				providerallowlist.ReceiptPath(city): item.receipt,
			} {
				if data == "" {
					continue
				}
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := CityDelegatesProviderApproval(city); err == nil {
				t.Fatal("surviving provider approval declaration silently became legacy")
			}
		})
	}
}

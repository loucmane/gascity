package platforminstall

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/providerallowlist"
)

func TestManifestAcceptsProviderAllowlistReference(t *testing.T) {
	manifest := integrityManifest(t, t.TempDir())
	encoded, err := MarshalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(encoded, &doc); err != nil {
		t.Fatal(err)
	}
	doc["manifest_sha256"] = ""
	doc["integrity"].(map[string]any)["provider_allowlist"] = map[string]any{
		"path":     filepath.Join(manifest.CityPath, "managed", "providers.json"),
		"bindings": []map[string]string{{"provider": "sample", "path": "/opt/bin/sample"}},
	}
	encoded, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := FinalizeManifest(encoded); err != nil {
		t.Fatalf("allowlist reference refused: %v", err)
	}
}

func TestProviderAllowlistStartAndIntegrity(t *testing.T) {
	for _, fault := range []string{"valid", "missing binary", "changed binary", "unapproved list", "missing receipt", "stale revision", "wrong link", "duplicate active"} {
		t.Run(fault, func(t *testing.T) {
			manifest := integrityManifest(t, t.TempDir())
			pin := manifest.Integrity.Providers[0]
			if err := os.Chmod(pin.ResolvedPath, 0o555); err != nil {
				t.Fatal(err)
			}
			list := providerallowlist.List{Schema: providerallowlist.Schema, Entries: []providerallowlist.Entry{{
				Provider: "sample", Version: "1.0", Path: pin.ResolvedPath, SHA256: pin.SHA256, Active: true,
			}}}
			manifest.Integrity.Providers = nil
			manifest.Integrity.ProviderAllowlist = &providerallowlist.Reference{
				Path: providerallowlist.Path(manifest.CityPath), Bindings: []providerallowlist.Binding{{Provider: "sample", Path: pin.Path}},
			}
			listData, err := json.Marshal(list)
			if err != nil {
				t.Fatal(err)
			}
			writeProviderFixture(t, manifest.Integrity.ProviderAllowlist.Path, listData)
			receipt := map[string]any{
				"schema": "gc.provisioning-receipt.v2", "permission_revision": strings.Repeat("b", 64),
				"provider_allowlist": map[string]string{"path": manifest.Integrity.ProviderAllowlist.Path, "sha256": testSHA256(listData)},
			}
			unsigned, err := json.Marshal(receipt)
			if err != nil {
				t.Fatal(err)
			}
			receipt["receipt_sha256"] = testSHA256(unsigned)
			encoded, err := json.Marshal(receipt)
			if err != nil {
				t.Fatal(err)
			}
			writeProviderFixture(t, providerallowlist.ReceiptPath(manifest.CityPath), encoded)
			manifest = finalizeManifest(t, manifest)
			if _, err := Install(manifest); err != nil {
				t.Fatal(err)
			}
			if err := VerifyAllowlistedProvider(context.Background(), manifest, pin); err != nil {
				t.Fatal(err)
			}
			revision := strings.Repeat("b", 64)
			switch fault {
			case "missing binary":
				err = os.Remove(pin.ResolvedPath)
			case "changed binary":
				err = os.Chmod(pin.ResolvedPath, 0o755)
				if err == nil {
					err = os.WriteFile(pin.ResolvedPath, []byte("changed"), 0o555)
				}
				if err == nil {
					err = os.Chmod(pin.ResolvedPath, 0o555)
				}
			case "unapproved list":
				writeProviderFixture(t, manifest.Integrity.ProviderAllowlist.Path, append(listData, '\n'))
			case "missing receipt":
				err = os.Remove(providerallowlist.ReceiptPath(manifest.CityPath))
			case "stale revision":
				revision = strings.Repeat("c", 64)
			case "wrong link":
				err = os.Remove(pin.Path)
				if err == nil {
					err = os.Symlink("/missing", pin.Path)
				}
			case "duplicate active":
				list.Entries = append(list.Entries, list.Entries[0])
				listData, err = json.Marshal(list)
				if err == nil {
					writeProviderFixture(t, manifest.Integrity.ProviderAllowlist.Path, listData)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			err = VerifyCityProviderAllowlist(manifest.CityPath, revision)
			if fault == "valid" && err != nil {
				t.Fatal(err)
			}
			if fault != "valid" && (err == nil || !strings.Contains(err.Error(), "provider allowlist")) {
				t.Fatalf("start accepted %s: %v", fault, err)
			}
			report, err := InspectIntegrity(context.Background(), manifest)
			if err != nil {
				t.Fatal(err)
			}
			wantDrift := fault != "valid" && fault != "stale revision"
			if wantDrift != (len(report.Drifts) > 0) {
				t.Fatalf("integrity for %s: %+v", fault, report.Drifts)
			}
		})
	}
}

func writeProviderFixture(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCompanionManifestRejectsNullReference(t *testing.T) {
	manifest := integrityManifest(t, t.TempDir())
	data, err := MarshalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	doc["manifest_sha256"] = ""
	doc["integrity"].(map[string]any)["provider_allowlist"] = nil
	data, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := FinalizeManifest(data); err == nil {
		t.Fatal("accepted null provider_allowlist reference")
	}
}

func TestLegacyProviderStartIgnoresUnrelatedManifestDrift(t *testing.T) {
	city := t.TempDir()
	manifest := integrityManifest(t, city)
	manifest.ReleaseID = "stale-self-digest"
	data, err := MarshalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeProviderFixture(t, DefaultManifestPath(city), data)
	if err := VerifyCityProviderAllowlist(city, ""); err != nil {
		t.Fatalf("legacy direct start changed when no allowlist delegates approval: %v", err)
	}
}

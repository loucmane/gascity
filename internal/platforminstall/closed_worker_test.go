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

func TestClosedWorkerAllowlistVersion(t *testing.T) {
	for _, family := range []string{"signing", "candidate", "template-candidate", "project-candidate"} {
		t.Run(family, func(t *testing.T) {
			root := t.TempDir()
			city := filepath.Join(root, "city")
			native := filepath.Join(root, "toolchains", "claude")
			writeProviderFixture(t, native, []byte("approved native CLI"))
			if err := os.Chmod(native, 0o555); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(root, "bin", "claude")
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(native, link); err != nil {
				t.Fatal(err)
			}
			sha := testSHA256([]byte("approved native CLI"))
			name := "gct-claude-" + family + "-worker"
			prefix := name + " 2 sources_sha256=" + strings.Repeat("c", 64) + " cli_sha256="
			version := prefix + sha
			wrapper := filepath.Join(root, name)
			data := []byte("#!/bin/sh\nprintf '%s\\n' '" + version + "'\n")
			writeProviderFixture(t, wrapper, data)
			if err := os.Chmod(wrapper, 0o755); err != nil {
				t.Fatal(err)
			}
			pin := ProviderPin{
				Name: "claude", Path: wrapper, ResolvedPath: wrapper,
				SHA256: testSHA256(data), VersionArgs: []string{"--version"}, Version: prefix + strings.Repeat("a", 64),
			}
			list := providerallowlist.List{Schema: providerallowlist.Schema, Entries: []providerallowlist.Entry{
				{Provider: "claude", Version: "2.1.318", Path: native, SHA256: sha, Active: true},
			}}
			listData, err := json.Marshal(list)
			if err != nil {
				t.Fatal(err)
			}
			writeProviderFixture(t, providerallowlist.Path(city), listData)
			receipt := map[string]any{
				"schema": "gc.provisioning-receipt.v2", "permission_revision": strings.Repeat("b", 64),
				"provider_allowlist": map[string]string{"path": providerallowlist.Path(city), "sha256": testSHA256(listData)},
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
			writeProviderFixture(t, providerallowlist.ReceiptPath(city), encoded)
			manifest := Manifest{CityPath: city, Integrity: &IntegritySpec{
				Providers: []ProviderPin{pin},
				ProviderAllowlist: &providerallowlist.Reference{
					Path:     providerallowlist.Path(city),
					Bindings: []providerallowlist.Binding{{Provider: "claude", Path: link}},
				},
			}}
			if report := inspectPinnedIntegrity(context.Background(), manifest); len(report.Drifts) != 0 {
				t.Fatalf("approved CLI-only bump refused: %+v", report.Drifts)
			}
			current := pin
			current.Version = version
			if err := VerifyAllowlistedProvider(context.Background(), manifest, current); err != nil {
				t.Fatalf("refreshed wrapper receipt refused: %v", err)
			}
			for _, fault := range []string{"sources", "sha", "args", "stale", "unapproved", "unbound"} {
				t.Run(fault, func(t *testing.T) {
					bad := current
					m := manifest
					i := *manifest.Integrity
					m.Integrity = &i
					switch fault {
					case "sources":
						bad.Version = strings.Replace(version, strings.Repeat("c", 64), strings.Repeat("d", 64), 1)
					case "sha":
						bad.SHA256 = strings.Repeat("d", 64)
					case "args":
						bad.VersionArgs = []string{"--other"}
					case "stale":
						bad.Version = pin.Version
					case "unapproved":
						writeProviderFixture(t, providerallowlist.Path(city), append(listData, '\n'))
						defer writeProviderFixture(t, providerallowlist.Path(city), listData)
					case "unbound":
						i.Providers = nil
					}
					if err := VerifyAllowlistedProvider(context.Background(), m, bad); err == nil {
						t.Fatal("accepted " + fault)
					}
				})
			}
			manifest.Integrity.ProviderAllowlist = nil
			if report := inspectPinnedIntegrity(context.Background(), manifest); len(report.Drifts) == 0 {
				t.Fatal("legacy version checks disabled")
			}
		})
	}
}

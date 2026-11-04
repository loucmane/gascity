package platforminstall

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/providerallowlist"
	"github.com/gastownhall/gascity/internal/shellquote"
)

func TestAttentionLaunchRequiresSignedChain(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		for _, fault := range []string{"valid", "unsigned", "missing helper", "missing parser", "bytes", "mode", "symlink", "parent symlink", "noncanonical binding"} {
			t.Run(provider+"/"+fault, func(t *testing.T) {
				root := t.TempDir()
				manifest, wrapper := attentionManifestFixture(t, root, provider)
				helper := filepath.Join(filepath.Dir(filepath.Dir(wrapper)), "lib", "gct_attention_launch.py")
				switch fault {
				case "unsigned":
					manifest.Integrity.Files = manifest.Integrity.Files[1:]
				case "missing helper":
					manifest.Integrity.Files = append(manifest.Integrity.Files[:1], manifest.Integrity.Files[2:]...)
				case "missing parser":
					manifest.Integrity.Files = manifest.Integrity.Files[:2]
				case "bytes":
					if err := os.Chmod(helper, 0o644); err != nil {
						t.Fatal(err)
					}
					writeProviderFixture(t, helper, []byte("changed"))
					if err := os.Chmod(helper, 0o444); err != nil {
						t.Fatal(err)
					}
				case "mode":
					if err := os.Chmod(helper, 0o600); err != nil {
						t.Fatal(err)
					}
				case "symlink":
					target := filepath.Join(root, "helper-copy")
					data, err := os.ReadFile(helper)
					if err != nil {
						t.Fatal(err)
					}
					writeProviderFixture(t, target, data)
					if err := os.Remove(helper); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(target, helper); err != nil {
						t.Fatal(err)
					}
				case "parent symlink":
					parent := filepath.Dir(helper)
					if err := os.Rename(parent, parent+"-real"); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(parent+"-real", parent); err != nil {
						t.Fatal(err)
					}
				case "noncanonical binding":
					binding := &manifest.Integrity.ProviderAllowlist.Bindings[0]
					alternate := binding.Path + "-other"
					if err := os.Rename(binding.Path, alternate); err != nil {
						t.Fatal(err)
					}
					binding.Path = alternate
				}
				manifest = finalizeManifest(t, manifest)
				data, err := MarshalManifest(manifest)
				if err != nil {
					t.Fatal(err)
				}
				writeProviderFixture(t, DefaultManifestPath(manifest.CityPath), data)
				command := shellquote.Join([]string{wrapper, "--resume", "conversation"})
				got, err := ResolveCityProviderLaunch(context.Background(), manifest.CityPath, strings.Repeat("b", 64), command)
				if fault == "valid" {
					want := shellquote.Join([]string{"/usr/bin/python3", "-I", helper, provider, manifest.CityPath, "--resume", "conversation"})
					if err != nil || got != want {
						t.Fatalf("signed attention chain = %q, %v", got, err)
					}
					if again, err := ResolveCityProviderLaunch(context.Background(), manifest.CityPath, "", got); err != nil || again != got {
						t.Fatalf("canonical stored attention resume refused: %q, %v", again, err)
					}
					wrongCity := shellquote.Join([]string{"/usr/bin/python3", "-I", helper, provider, "/another-city"})
					if _, err := ResolveCityProviderLaunch(context.Background(), manifest.CityPath, "", wrongCity); err == nil {
						t.Fatal("accepted caller-selected authority city")
					}
				} else if err == nil {
					t.Fatalf("accepted %s chain: %q", fault, got)
				}
			})
		}
	}
}

func attentionManifestFixture(t *testing.T, root, provider string) (Manifest, string) {
	t.Helper()
	manifest := testManifest(t, root, []byte("candidate"), []byte("installed"))
	manifest.CityPath = filepath.Join(root, "city")
	target := filepath.Join(root, "toolchains", provider)
	cli := []byte("#!/bin/sh\nexit 0\n")
	writeProviderFixture(t, target, cli)
	if err := os.Chmod(target, 0o555); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(root, "bin", provider)
	if err := os.Symlink(target, stable); err != nil {
		t.Fatal(err)
	}
	list, err := json.Marshal(providerallowlist.List{Schema: providerallowlist.Schema, Entries: []providerallowlist.Entry{{Provider: provider, Version: "1", Path: target, SHA256: testSHA256(cli), Active: true}}})
	if err != nil {
		t.Fatal(err)
	}
	writeProviderFixture(t, providerallowlist.Path(manifest.CityPath), list)
	receipt := map[string]any{"schema": "gc.provisioning-receipt.v2", "permission_revision": strings.Repeat("b", 64), "provider_allowlist": map[string]string{"path": providerallowlist.Path(manifest.CityPath), "sha256": testSHA256(list)}}
	unsigned, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	receipt["receipt_sha256"] = testSHA256(unsigned)
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	writeProviderFixture(t, providerallowlist.ReceiptPath(manifest.CityPath), data)
	wrapper := filepath.Join(root, "native", "bin", provider+"-attention")
	manifest.Integrity = &IntegritySpec{ProviderAllowlist: &providerallowlist.Reference{Path: providerallowlist.Path(manifest.CityPath), Bindings: []providerallowlist.Binding{{Provider: provider, Path: stable}}}}
	for _, relative := range []string{"bin/" + provider + "-attention", "lib/gct_attention_launch.py", "lib/gct_provider_allowlist.py"} {
		path := filepath.Join(root, "native", relative)
		contents := []byte("signed source " + relative)
		mode := os.FileMode(0o444)
		if path == wrapper {
			mode = 0o555
		}
		writeProviderFixture(t, path, contents)
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		manifest.Integrity.Files = append(manifest.Integrity.Files, FilePin{Name: relative, Path: path, SHA256: testSHA256(contents), Mode: uint32(mode)})
	}
	return manifest, wrapper
}

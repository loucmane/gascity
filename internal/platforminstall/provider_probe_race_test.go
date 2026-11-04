package platforminstall

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gastownhall/gascity/internal/providerallowlist"
)

func TestProviderInspectionRefusesBeforeExecutionOnDrift(t *testing.T) {
	for _, fault := range []string{"digest", "resolved", "mode"} {
		t.Run(fault, func(t *testing.T) {
			root := t.TempDir()
			marker := filepath.Join(root, "ran")
			path := filepath.Join(root, "cli")
			data := []byte("#!/bin/sh\necho replacement > " + marker + "\necho version\n")
			if err := os.WriteFile(path, data, 0o755); err != nil {
				t.Fatal(err)
			}
			pin := ProviderPin{Name: "sample", Path: path, ResolvedPath: path, SHA256: testSHA256(data), VersionArgs: []string{"--version"}, Version: "version"}
			switch fault {
			case "digest":
				pin.SHA256 = testSHA256([]byte("approved"))
			case "resolved":
				pin.ResolvedPath += ".approved"
			case "mode":
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := VerifyProviderPin(context.Background(), pin); err == nil {
				t.Fatal("accepted drift")
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("executed mismatched bytes: %v", err)
			}
		})
	}
}

func TestProviderVersionProbeRetainsVerifiedInode(t *testing.T) {
	root := t.TempDir()
	path, marker := filepath.Join(root, "cli"), filepath.Join(root, "ran")
	approved := []byte("#!/bin/sh\necho approved > " + marker + "\necho version\n")
	if err := os.WriteFile(path, approved, 0o755); err != nil {
		t.Fatal(err)
	}
	pin := ProviderPin{Name: "sample", Path: path, ResolvedPath: path, SHA256: testSHA256(approved), VersionArgs: []string{"--version"}, Version: "version"}
	old := providerInspectionCommandContext
	t.Cleanup(func() { providerInspectionCommandContext = old })
	providerInspectionCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		replacement := path + ".new"
		if err := os.WriteFile(replacement, []byte("#!/bin/sh\necho replacement > "+marker+"\necho version\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(replacement, path); err != nil {
			t.Fatal(err)
		}
		return exec.CommandContext(ctx, name, args...)
	}
	// The descriptor variant is used for delegated native pins and toolchains.
	if err := VerifyProviderPinDescriptor(context.Background(), pin); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "approved\n" {
		t.Fatalf("ran %q: %v", data, err)
	}
}

func TestAllowlistedProviderProbeRetainsVerifiedInode(t *testing.T) {
	manifest := integrityManifest(t, t.TempDir())
	pin := manifest.Integrity.Providers[0]
	if err := os.Chmod(pin.ResolvedPath, 0o555); err != nil {
		t.Fatal(err)
	}
	list := providerallowlist.List{Schema: providerallowlist.Schema, Entries: []providerallowlist.Entry{{Provider: "sample", Version: "1.0", Path: pin.ResolvedPath, SHA256: pin.SHA256, Active: true}}}
	data, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	ref := &providerallowlist.Reference{Path: providerallowlist.Path(manifest.CityPath), Bindings: []providerallowlist.Binding{{Provider: "sample", Path: pin.Path}}}
	manifest.Integrity.ProviderAllowlist = ref
	writeProviderFixture(t, ref.Path, data)
	receipt := map[string]any{"schema": "gc.provisioning-receipt.v2", "permission_revision": testSHA256([]byte("revision")), "provider_allowlist": map[string]string{"path": ref.Path, "sha256": testSHA256(data)}}
	data, err = json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	receipt["receipt_sha256"] = testSHA256(data)
	data, err = json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	writeProviderFixture(t, providerallowlist.ReceiptPath(manifest.CityPath), data)
	marker := filepath.Join(t.TempDir(), "ran")
	old := providerInspectionCommandContext
	t.Cleanup(func() { providerInspectionCommandContext = old })
	providerInspectionCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		path := pin.ResolvedPath + ".new"
		if err := os.WriteFile(path, []byte("#!/bin/sh\necho replacement > "+marker+"\necho replacement\n"), 0o555); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path, pin.ResolvedPath); err != nil {
			t.Fatal(err)
		}
		return exec.CommandContext(ctx, name, args...)
	}
	if err := VerifyAllowlistedProvider(context.Background(), manifest, pin); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("replacement ran: %v", err)
	}
	if err := VerifyAllowlistedProvider(context.Background(), manifest, pin); err == nil {
		t.Fatal("accepted pre-open replacement")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("mismatch executed: %v", err)
	}
}

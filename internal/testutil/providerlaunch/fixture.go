// Package providerlaunch installs the signed companion launcher in isolated tests.
package providerlaunch

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/platforminstall"
	"github.com/gastownhall/gascity/internal/providerallowlist"
)

// Companion copies are byte-compared with Template and Native by artifacts6.py.
//
//go:embed testdata/*.py
var sources embed.FS

// Fixture contains a synthetic native executable, its authority and signed launcher.
type Fixture struct {
	City, Stable, Target, Helper string
}

// New installs an approved native CLI and the actual companion helper sources.
func New(t *testing.T, provider string) Fixture {
	t.Helper()
	root := t.TempDir()
	f := Fixture{
		City: filepath.Join(root, "city"), Stable: filepath.Join(root, "bin", provider),
		Target: filepath.Join(root, "versions", provider), Helper: filepath.Join(root, "native", "lib", "gct_attention_launch.py"),
	}
	write := func(path string, data []byte, mode os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, mode); err != nil {
			t.Fatal(err)
		}
	}
	digest := func(data []byte) string { hash := sha256.Sum256(data); return hex.EncodeToString(hash[:]) }
	encode := func(v any) []byte {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	cli := []byte("#!/bin/sh\nprintf 'APPROVED\\n'\nprintf '%s\\n' \"$@\"\n")
	write(f.Target, cli, 0o555)
	if err := os.MkdirAll(filepath.Dir(f.Stable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(f.Target, f.Stable); err != nil {
		t.Fatal(err)
	}
	list := encode(providerallowlist.List{Schema: providerallowlist.Schema, Entries: []providerallowlist.Entry{
		{Provider: provider, Version: "1", Path: f.Target, SHA256: digest(cli), Active: true},
	}})
	write(providerallowlist.Path(f.City), list, 0o600)
	receipt := map[string]any{
		"schema": "gc.provisioning-receipt.v2", "permission_revision": digest([]byte("revision")),
		"provider_allowlist": map[string]string{"path": providerallowlist.Path(f.City), "sha256": digest(list)},
	}
	receipt["receipt_sha256"] = digest(encode(receipt))
	write(providerallowlist.ReceiptPath(f.City), encode(receipt), 0o600)
	manifest := platforminstall.Manifest{
		Schema: platforminstall.ManifestSchemaV1, ReleaseID: "native-race", CityPath: f.City,
		Core:           platforminstall.Artifact{Name: "gc", Source: filepath.Join(root, "candidate"), Destination: filepath.Join(root, "bin/gc"), SHA256: digest([]byte("core")), Mode: 0o755},
		PreviousSHA256: digest([]byte("old")), BackupPath: filepath.Join(root, "backup"), ReceiptPath: filepath.Join(root, "receipt"),
		Integrity: &platforminstall.IntegritySpec{ProviderAllowlist: &providerallowlist.Reference{Path: providerallowlist.Path(f.City), Bindings: []providerallowlist.Binding{{Provider: provider, Path: f.Stable}}}},
	}
	for _, name := range []string{"gct_attention_launch.py", "gct_provider_allowlist.py"} {
		data, err := sources.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "native/lib", name)
		write(path, data, 0o444)
		manifest.Integrity.Files = append(manifest.Integrity.Files, platforminstall.FilePin{Name: name, Path: path, SHA256: digest(data), Mode: 0o444})
	}
	wrapper := filepath.Join(root, "native/bin", provider+"-attention")
	write(wrapper, []byte("#!/bin/sh\nexit 1\n"), 0o555)
	manifest.Integrity.Files = append(manifest.Integrity.Files, platforminstall.FilePin{Name: "attention", Path: wrapper, SHA256: digest([]byte("#!/bin/sh\nexit 1\n")), Mode: 0o555})
	_, data, err := platforminstall.FinalizeManifest(encode(manifest))
	if err != nil {
		t.Fatal(err)
	}
	write(platforminstall.DefaultManifestPath(f.City), data, 0o600)
	return f
}

// Run replaces only the executable's directory entry immediately before runtime exec.
// A trusted launcher must refuse it; the old direct command prints UNAPPROVED.
func (f Fixture) Run(t *testing.T, command string, replace bool) (string, error) {
	t.Helper()
	if replace {
		path := f.Target + ".replacement"
		if err := os.WriteFile(path, []byte("#!/bin/sh\necho UNAPPROVED\n"), 0o555); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path, f.Target); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + filepath.Dir(f.City), "GC_HOME=" + filepath.Join(f.City, "gc-home")}
	output, err := cmd.CombinedOutput()
	return string(output), err
}

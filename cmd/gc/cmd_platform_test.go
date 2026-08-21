package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/platforminstall"
)

func TestPlatformInstallRequiresExactlyOneMode(t *testing.T) {
	manifestPath, _ := platformCommandFixture(t)
	for _, args := range [][]string{
		{"install", "--manifest", manifestPath},
		{"install", "--manifest", manifestPath, "--dry-run", "--apply"},
	} {
		cmd := newPlatformCmd(&bytes.Buffer{}, &bytes.Buffer{})
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "exactly one of --dry-run or --apply") {
			t.Fatalf("Execute(%v) error = %v, want explicit-mode refusal", args, err)
		}
	}
}

func TestPlatformInstallDryRunPrintsOrderedPlanWithoutMutation(t *testing.T) {
	manifestPath, manifest := platformCommandFixture(t)
	before, err := os.Stat(manifest.Core.Destination)
	if err != nil {
		t.Fatal(err)
	}
	beforeBytes, err := os.ReadFile(manifest.Core.Destination)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	cmd := newPlatformCmd(&stdout, &stderr)
	cmd.SetArgs([]string{"install", "--manifest", manifestPath, "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v; stderr=%s", err, stderr.String())
	}
	for _, want := range []string{"01 CHECK verify-manifest", "04 MUTATE write-backup", "08 CHECK verify-integrity"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
	after, err := os.Stat(manifest.Core.Destination)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("dry-run changed destination identity or mtime")
	}
	afterBytes, err := os.ReadFile(manifest.Core.Destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterBytes, beforeBytes) {
		t.Fatal("dry-run changed destination bytes")
	}
	for _, path := range []string{manifest.BackupPath, manifest.ReceiptPath, platforminstall.DefaultManifestPath(manifest.CityPath)} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("dry-run created %s: %v", path, err)
		}
	}
}

func TestPlatformInstallApplyUsesManifestTransaction(t *testing.T) {
	manifestPath, manifest := platformCommandFixture(t)
	var stdout, stderr bytes.Buffer
	cmd := newPlatformCmd(&stdout, &stderr)
	cmd.SetArgs([]string{"install", "--manifest", manifestPath, "--apply"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v; stderr=%s", err, stderr.String())
	}
	if got, err := os.ReadFile(manifest.Core.Destination); err != nil || string(got) != "candidate" {
		t.Fatalf("installed destination = %q, err=%v", got, err)
	}
	if !strings.Contains(stdout.String(), "result=installed") || !strings.Contains(stdout.String(), manifest.ManifestSHA256) {
		t.Fatalf("stdout = %q, want installed result and manifest digest", stdout.String())
	}
}

func TestRootRegistersPlatformCommand(t *testing.T) {
	root := newRootCmdWithOptions(&bytes.Buffer{}, &bytes.Buffer{}, rootCommandOptions{})
	command, _, err := root.Find([]string{"platform", "install"})
	if err != nil || command == nil || command.Name() != "install" {
		t.Fatalf("root.Find(platform install) = command=%v err=%v", command, err)
	}
}

func platformCommandFixture(t *testing.T) (string, platforminstall.Manifest) {
	t.Helper()
	dir := t.TempDir()
	candidate := []byte("candidate")
	previous := []byte("previous")
	candidatePath := filepath.Join(dir, "candidate", "gc")
	destinationPath := filepath.Join(dir, "bin", "gc")
	for _, parent := range []string{filepath.Dir(candidatePath), filepath.Dir(destinationPath)} {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(candidatePath, candidate, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destinationPath, previous, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := platforminstall.Manifest{
		Schema:         platforminstall.ManifestSchemaV1,
		ReleaseID:      "test-platform-release",
		CityPath:       dir,
		Core:           platforminstall.Artifact{Name: "gc", Source: candidatePath, Destination: destinationPath, SHA256: platformCommandSHA(candidate), Mode: 0o755},
		PreviousSHA256: platformCommandSHA(previous),
		BackupPath:     filepath.Join(dir, ".gc", "platform", "gc.previous"),
		ReceiptPath:    filepath.Join(dir, ".gc", "platform", "install-receipt.json"),
	}
	digest, err := platforminstall.ManifestDigest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifest.ManifestSHA256 = digest
	data, err := platforminstall.MarshalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(dir, "input-manifest.json")
	if err := os.WriteFile(inputPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return inputPath, manifest
}

func platformCommandSHA(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

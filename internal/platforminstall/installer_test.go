package platforminstall

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestLoadManifestRejectsSelfDigestMismatch(t *testing.T) {
	dir := t.TempDir()
	manifest := testManifest(t, dir, []byte("candidate"), []byte("installed"))
	data := marshalManifest(t, manifest)
	data = bytes.Replace(data, []byte(manifest.Core.SHA256), []byte(testSHA256([]byte("different"))), 1)

	_, err := LoadManifest(data)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("manifest_sha256")) {
		t.Fatalf("LoadManifest() error = %v, want manifest_sha256 mismatch", err)
	}
}

func TestInstallPreflightsCandidateBeforeMutation(t *testing.T) {
	dir := t.TempDir()
	manifest := testManifest(t, dir, []byte("candidate"), []byte("installed"))
	manifest.Core.SHA256 = testSHA256([]byte("not-the-candidate"))
	manifest = finalizeManifest(t, manifest)
	before := mustReadFile(t, manifest.Core.Destination)

	_, err := Install(manifest)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("candidate sha256")) {
		t.Fatalf("Install() error = %v, want candidate sha256 mismatch", err)
	}
	if got := mustReadFile(t, manifest.Core.Destination); !bytes.Equal(got, before) {
		t.Fatalf("destination changed after preflight failure: got %q want %q", got, before)
	}
	assertPathAbsent(t, manifest.BackupPath)
	assertPathAbsent(t, manifest.ReceiptPath)
}

func TestInstallPublishesCandidateWithExactBackupAndReceipt(t *testing.T) {
	dir := t.TempDir()
	manifest := testManifest(t, dir, []byte("candidate"), []byte("installed"))

	receipt, err := Install(manifest)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if receipt.Result != ResultInstalled {
		t.Fatalf("receipt result = %q, want %q", receipt.Result, ResultInstalled)
	}
	if got := mustReadFile(t, manifest.Core.Destination); string(got) != "candidate" {
		t.Fatalf("destination = %q, want candidate", got)
	}
	if got := mustReadFile(t, manifest.BackupPath); string(got) != "installed" {
		t.Fatalf("backup = %q, want installed", got)
	}
	if got := mustReadFile(t, manifest.ReceiptPath); !bytes.Contains(got, []byte(`"result":"installed"`)) {
		t.Fatalf("receipt file does not record installed result: %s", got)
	}
	if info, err := os.Stat(manifest.Core.Destination); err != nil {
		t.Fatal(err)
	} else if got, want := info.Mode().Perm(), os.FileMode(0o755); got != want {
		t.Fatalf("destination mode = %o, want %o", got, want)
	}
}

func TestInstallIdenticalReplayIsNoOp(t *testing.T) {
	dir := t.TempDir()
	manifest := testManifest(t, dir, []byte("candidate"), []byte("installed"))
	if _, err := Install(manifest); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	destinationBefore := mustStat(t, manifest.Core.Destination)
	backupBefore := mustStat(t, manifest.BackupPath)
	receiptBefore := mustStat(t, manifest.ReceiptPath)

	time.Sleep(10 * time.Millisecond)
	receipt, err := Install(manifest)
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if receipt.Result != ResultNoop {
		t.Fatalf("second receipt result = %q, want %q", receipt.Result, ResultNoop)
	}
	assertSameFileIdentityAndTime(t, manifest.Core.Destination, destinationBefore)
	assertSameFileIdentityAndTime(t, manifest.BackupPath, backupBefore)
	assertSameFileIdentityAndTime(t, manifest.ReceiptPath, receiptBefore)
}

func TestInstallerPublishFailureLeavesInstalledBinaryUntouched(t *testing.T) {
	dir := t.TempDir()
	manifest := testManifest(t, dir, []byte("candidate"), []byte("installed"))
	want := mustReadFile(t, manifest.Core.Destination)
	inst := newInstaller()
	inst.rename = func(_, _ string) error { return errors.New("injected rename failure") }

	_, err := inst.install(manifest)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("injected rename failure")) {
		t.Fatalf("install() error = %v, want injected rename failure", err)
	}
	if got := mustReadFile(t, manifest.Core.Destination); !bytes.Equal(got, want) {
		t.Fatalf("destination changed after publish failure: got %q want %q", got, want)
	}
	assertPathAbsent(t, manifest.ReceiptPath)
}

func TestInstallerReceiptFailureRollsBackExactPriorBinary(t *testing.T) {
	dir := t.TempDir()
	manifest := testManifest(t, dir, []byte("candidate"), []byte("installed"))
	want := mustReadFile(t, manifest.Core.Destination)
	inst := newInstaller()
	inst.writeReceipt = func(string, Receipt) error { return errors.New("injected receipt failure") }

	_, err := inst.install(manifest)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("injected receipt failure")) {
		t.Fatalf("install() error = %v, want injected receipt failure", err)
	}
	if got := mustReadFile(t, manifest.Core.Destination); !bytes.Equal(got, want) {
		t.Fatalf("destination after rollback = %q, want %q", got, want)
	}
	assertPathAbsent(t, manifest.ReceiptPath)
}

func testManifest(t *testing.T, dir string, candidate, installed []byte) Manifest {
	t.Helper()
	source := filepath.Join(dir, "candidate-gc")
	destination := filepath.Join(dir, "bin", "gc")
	backup := filepath.Join(dir, "backups", "gc.previous")
	receipt := filepath.Join(dir, "state", "install-receipt.json")
	for _, parent := range []string{filepath.Dir(source), filepath.Dir(destination)} {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(source, candidate, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, installed, 0o755); err != nil {
		t.Fatal(err)
	}
	return finalizeManifest(t, Manifest{
		Schema:      ManifestSchemaV1,
		ReleaseID:   "v1.4.1-loucmane.1-d0a197d31",
		Core:        Artifact{Name: "gc", Source: source, Destination: destination, SHA256: testSHA256(candidate), Mode: 0o755},
		BackupPath:  backup,
		ReceiptPath: receipt,
	})
}

func finalizeManifest(t *testing.T, manifest Manifest) Manifest {
	t.Helper()
	digest, err := ManifestDigest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifest.ManifestSHA256 = digest
	return manifest
}

func marshalManifest(t *testing.T, manifest Manifest) []byte {
	t.Helper()
	data, err := MarshalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func assertSameFileIdentityAndTime(t *testing.T, path string, before os.FileInfo) {
	t.Helper()
	after := mustStat(t, path)
	if !os.SameFile(before, after) {
		t.Fatalf("%s inode changed on no-op replay", path)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("%s mtime changed on no-op replay: before=%s after=%s", path, before.ModTime(), after.ModTime())
	}
}

func assertPathAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s exists or stat failed unexpectedly: %v", path, err)
	}
}

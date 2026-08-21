package platforminstall

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallPreflightsEveryManagedFileBeforeCoreMutation(t *testing.T) {
	dir := t.TempDir()
	manifest := testManifestWithManagedFiles(t, dir)
	manifest.ManagedFiles[1].SHA256 = testSHA256([]byte("not-the-source"))
	manifest = finalizeManifest(t, manifest)
	coreBefore := mustReadFile(t, manifest.Core.Destination)
	firstBefore := mustReadFile(t, manifest.ManagedFiles[0].Destination)

	_, err := Install(manifest)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("managed file")) {
		t.Fatalf("Install() error = %v, want managed-file preflight failure", err)
	}
	if got := mustReadFile(t, manifest.Core.Destination); !bytes.Equal(got, coreBefore) {
		t.Fatalf("core changed after managed-file preflight failure: got %q want %q", got, coreBefore)
	}
	if got := mustReadFile(t, manifest.ManagedFiles[0].Destination); !bytes.Equal(got, firstBefore) {
		t.Fatalf("first managed file changed after later preflight failure: got %q want %q", got, firstBefore)
	}
}

func TestInstallPublishesManagedFilesWithExactBackupsAndReceipt(t *testing.T) {
	dir := t.TempDir()
	manifest := testManifestWithManagedFiles(t, dir)

	receipt, err := Install(manifest)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if got := string(mustReadFile(t, manifest.ManagedFiles[0].Destination)); got != "rules-v2" {
		t.Fatalf("managed rules = %q, want rules-v2", got)
	}
	if got := string(mustReadFile(t, manifest.ManagedFiles[0].BackupPath)); got != "rules-v1" {
		t.Fatalf("managed rules backup = %q, want rules-v1", got)
	}
	if got := string(mustReadFile(t, manifest.ManagedFiles[1].Destination)); got != "validator-v1" {
		t.Fatalf("new managed validator = %q, want validator-v1", got)
	}
	if len(receipt.ManagedFiles) != 2 {
		t.Fatalf("receipt managed_files = %+v, want two entries", receipt.ManagedFiles)
	}
	if receipt.ManagedFiles[0].Name != "control-rules" || receipt.ManagedFiles[0].PreviousSHA256 != testSHA256([]byte("rules-v1")) {
		t.Fatalf("receipt first managed file = %+v", receipt.ManagedFiles[0])
	}
	if receipt.ManagedFiles[1].Name != "validator" || receipt.ManagedFiles[1].PreviousSHA256 != "" {
		t.Fatalf("receipt second managed file = %+v", receipt.ManagedFiles[1])
	}
}

func TestManagedFilePublishFailureRollsBackCoreAndEarlierFiles(t *testing.T) {
	dir := t.TempDir()
	manifest := testManifestWithManagedFiles(t, dir)
	coreBefore := mustReadFile(t, manifest.Core.Destination)
	firstBefore := mustReadFile(t, manifest.ManagedFiles[0].Destination)
	inst := newInstaller()
	realRename := inst.rename
	inst.rename = func(source, destination string) error {
		if destination == manifest.ManagedFiles[1].Destination {
			return errors.New("injected second managed-file publish failure")
		}
		return realRename(source, destination)
	}

	_, err := inst.install(manifest)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("injected second managed-file publish failure")) {
		t.Fatalf("install() error = %v, want injected managed-file failure", err)
	}
	if got := mustReadFile(t, manifest.Core.Destination); !bytes.Equal(got, coreBefore) {
		t.Fatalf("core after rollback = %q, want %q", got, coreBefore)
	}
	if got := mustReadFile(t, manifest.ManagedFiles[0].Destination); !bytes.Equal(got, firstBefore) {
		t.Fatalf("first managed file after rollback = %q, want %q", got, firstBefore)
	}
	assertPathAbsent(t, manifest.ManagedFiles[1].Destination)
	assertPathAbsent(t, manifest.ReceiptPath)
}

func TestManagedFilesIdenticalReplayIsNoOp(t *testing.T) {
	dir := t.TempDir()
	manifest := testManifestWithManagedFiles(t, dir)
	if _, err := Install(manifest); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	firstDestination := mustStat(t, manifest.ManagedFiles[0].Destination)
	firstBackup := mustStat(t, manifest.ManagedFiles[0].BackupPath)
	secondDestination := mustStat(t, manifest.ManagedFiles[1].Destination)

	receipt, err := Install(manifest)
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if receipt.Result != ResultNoop {
		t.Fatalf("second receipt result = %q, want %q", receipt.Result, ResultNoop)
	}
	assertSameFileIdentityAndTime(t, manifest.ManagedFiles[0].Destination, firstDestination)
	assertSameFileIdentityAndTime(t, manifest.ManagedFiles[0].BackupPath, firstBackup)
	assertSameFileIdentityAndTime(t, manifest.ManagedFiles[1].Destination, secondDestination)
}

func testManifestWithManagedFiles(t *testing.T, dir string) Manifest {
	t.Helper()
	manifest := testManifest(t, dir, []byte("candidate"), []byte("installed"))
	assetDir := filepath.Join(dir, "asset-source")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rulesSource := filepath.Join(assetDir, "native.rules")
	validatorSource := filepath.Join(assetDir, "build-artifact-valid.sh")
	rulesDestination := filepath.Join(dir, "launcher", ".codex", "rules", "native.rules")
	validatorDestination := filepath.Join(dir, "launcher", ".gc", "scripts", "checks", "build-artifact-valid.sh")
	if err := os.MkdirAll(filepath.Dir(rulesDestination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulesSource, []byte("rules-v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(validatorSource, []byte("validator-v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulesDestination, []byte("rules-v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest.ManagedFiles = []ManagedFile{
		{
			Name:           "control-rules",
			Source:         rulesSource,
			Destination:    rulesDestination,
			SHA256:         testSHA256([]byte("rules-v2")),
			Mode:           0o644,
			PreviousSHA256: testSHA256([]byte("rules-v1")),
			BackupPath:     filepath.Join(dir, "backups", "native.rules.previous"),
		},
		{
			Name:        "validator",
			Source:      validatorSource,
			Destination: validatorDestination,
			SHA256:      testSHA256([]byte("validator-v1")),
			Mode:        0o755,
		},
	}
	return finalizeManifest(t, manifest)
}

package platforminstall

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifestRejectsUnsortedManagedFiles(t *testing.T) {
	dir := t.TempDir()
	manifest := testManifestWithManagedFiles(t, dir)
	manifest.ManagedFiles[0], manifest.ManagedFiles[1] = manifest.ManagedFiles[1], manifest.ManagedFiles[0]
	manifest = finalizeManifest(t, manifest)

	_, err := LoadManifest(marshalManifest(t, manifest))
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("strictly sorted")) {
		t.Fatalf("LoadManifest() error = %v, want sorted managed-files refusal", err)
	}
}

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

func TestInstallResumesAfterCoreAndOneManagedFileWerePublished(t *testing.T) {
	dir := t.TempDir()
	manifest := testManifestWithManagedFiles(t, dir)
	if err := os.MkdirAll(filepath.Dir(manifest.BackupPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest.BackupPath, []byte("installed"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest.Core.Destination, []byte("candidate"), 0o755); err != nil {
		t.Fatal(err)
	}
	first := manifest.ManagedFiles[0]
	if err := os.WriteFile(first.BackupPath, []byte("rules-v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first.Destination, []byte("rules-v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	coreBefore := mustStat(t, manifest.Core.Destination)
	firstBefore := mustStat(t, first.Destination)

	receipt, err := Install(manifest)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if receipt.Result != ResultInstalled {
		t.Fatalf("receipt result = %q, want installed", receipt.Result)
	}
	assertSameFileIdentityAndTime(t, manifest.Core.Destination, coreBefore)
	assertSameFileIdentityAndTime(t, first.Destination, firstBefore)
	if got := string(mustReadFile(t, manifest.ManagedFiles[1].Destination)); got != "validator-v1" {
		t.Fatalf("remaining managed file = %q, want validator-v1", got)
	}
}

func TestManagedFilePlanNamesEveryBackupAndPublicationWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	manifest := testManifestWithManagedFiles(t, dir)
	rulesBefore := mustStat(t, manifest.ManagedFiles[0].Destination)

	steps, err := Plan(manifest)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	want := map[string]string{
		"write-managed-backup:control-rules": manifest.ManagedFiles[0].BackupPath,
		"publish-managed-file:control-rules": manifest.ManagedFiles[0].Destination,
		"publish-managed-file:validator":     manifest.ManagedFiles[1].Destination,
	}
	for _, step := range steps {
		if path, exists := want[step.Action]; exists {
			if step.Path != path || !step.Mutates {
				t.Errorf("Plan() step = %+v, want path=%q mutates=true", step, path)
			}
			delete(want, step.Action)
		}
	}
	if len(want) != 0 {
		t.Fatalf("Plan() missing managed actions: %v", want)
	}
	assertSameFileIdentityAndTime(t, manifest.ManagedFiles[0].Destination, rulesBefore)
	assertPathAbsent(t, manifest.ManagedFiles[0].BackupPath)
	assertPathAbsent(t, manifest.ManagedFiles[1].Destination)
}

func TestInspectIntegrityReportsManagedFileAndBackupDrift(t *testing.T) {
	dir := t.TempDir()
	manifest := testManifestWithManagedFiles(t, dir)
	if _, err := Install(manifest); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if err := os.WriteFile(manifest.ManagedFiles[0].Destination, []byte("rules drift"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest.ManagedFiles[0].BackupPath, []byte("backup drift"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := InspectIntegrity(context.Background(), manifest)
	if err != nil {
		t.Fatalf("InspectIntegrity() error = %v", err)
	}
	fields := driftFields(report)
	for _, want := range []string{"managed_files[control-rules].sha256", "managed_files[control-rules].backup.sha256"} {
		if !containsString(fields, want) {
			t.Errorf("drift fields = %v, want %q", fields, want)
		}
	}
}

func TestNoopRejectsReceiptWithDifferentManagedFileSet(t *testing.T) {
	dir := t.TempDir()
	manifest := testManifestWithManagedFiles(t, dir)
	if _, err := Install(manifest); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	receipt, err := loadReceipt(manifest.ReceiptPath)
	if err != nil {
		t.Fatal(err)
	}
	receipt.ManagedFiles = receipt.ManagedFiles[:1]
	if err := finalizeReceipt(&receipt); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest.ReceiptPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = Install(manifest)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("different release")) {
		t.Fatalf("Install() error = %v, want receipt managed-file mismatch", err)
	}
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

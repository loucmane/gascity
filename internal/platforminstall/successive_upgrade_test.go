package platforminstall

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestApplySuccessiveReleaseAndRevertRestoresPreviousManagedPlatform(t *testing.T) {
	dir := t.TempDir()
	first := activationManifest(t, dir)
	if _, err := Apply(context.Background(), first, exactFakeLifecycle(first)); err != nil {
		t.Fatalf("Apply(first) error = %v", err)
	}
	firstManifest := mustReadFile(t, DefaultManifestPath(dir))
	firstReceipt := mustReadFile(t, first.ReceiptPath)

	second := successiveActivationManifest(t, first, firstManifest, firstReceipt)
	secondLifecycle := exactFakeLifecycle(second)
	if _, err := Apply(context.Background(), second, secondLifecycle); err != nil {
		t.Fatalf("Apply(second) error = %v", err)
	}
	if secondLifecycle.restarts != 1 || secondLifecycle.verifies != 1 {
		t.Fatalf("second lifecycle calls restart=%d verify=%d, want 1/1", secondLifecycle.restarts, secondLifecycle.verifies)
	}
	if got := string(mustReadFile(t, second.Core.Destination)); got != "candidate-v2" {
		t.Fatalf("second core = %q, want candidate-v2", got)
	}
	if got := string(mustReadFile(t, second.ManagedFiles[0].Destination)); got != "rules-v3" {
		t.Fatalf("second rules = %q, want rules-v3", got)
	}
	if got := string(mustReadFile(t, second.ManagedFiles[1].Destination)); got != "validator-v2" {
		t.Fatalf("second validator = %q, want validator-v2", got)
	}

	replayPaths := []string{
		second.Core.Destination,
		second.ManagedFiles[0].Destination,
		second.ManagedFiles[1].Destination,
		DefaultManifestPath(dir),
		second.ReceiptPath,
		second.PreviousMetadata.ManifestBackupPath,
		second.PreviousMetadata.ReceiptBackupPath,
	}
	replayStats := make(map[string]os.FileInfo, len(replayPaths))
	for _, path := range replayPaths {
		replayStats[path] = mustStat(t, path)
	}
	replayLifecycle := exactFakeLifecycle(second)
	receipt, err := Apply(context.Background(), second, replayLifecycle)
	if err != nil {
		t.Fatalf("replay Apply(second) error = %v", err)
	}
	if receipt.Result != ResultNoop || replayLifecycle.restarts != 0 || replayLifecycle.verifies != 1 {
		t.Fatalf("replay result=%q restart=%d verify=%d, want noop/0/1", receipt.Result, replayLifecycle.restarts, replayLifecycle.verifies)
	}
	for _, path := range replayPaths {
		assertSameFileIdentityAndTime(t, path, replayStats[path])
	}

	previousLifecycle := previousFakeLifecycle(second)
	if _, err := Revert(context.Background(), second, previousLifecycle); err != nil {
		t.Fatalf("Revert(second) error = %v", err)
	}
	if previousLifecycle.restarts != 1 || previousLifecycle.verifies != 1 {
		t.Fatalf("revert lifecycle calls restart=%d verify=%d, want 1/1", previousLifecycle.restarts, previousLifecycle.verifies)
	}
	if got := string(mustReadFile(t, first.Core.Destination)); got != "candidate" {
		t.Fatalf("restored first core = %q, want candidate", got)
	}
	if got := string(mustReadFile(t, first.ManagedFiles[0].Destination)); got != "rules-v2" {
		t.Fatalf("restored first rules = %q, want rules-v2", got)
	}
	if got := string(mustReadFile(t, first.ManagedFiles[1].Destination)); got != "validator-v1" {
		t.Fatalf("restored first validator = %q, want validator-v1", got)
	}
	if got := mustReadFile(t, DefaultManifestPath(dir)); !bytes.Equal(got, firstManifest) {
		t.Fatalf("restored first manifest differs: got %s want %s", got, firstManifest)
	}
	if got := mustReadFile(t, first.ReceiptPath); !bytes.Equal(got, firstReceipt) {
		t.Fatalf("restored first receipt differs: got %s want %s", got, firstReceipt)
	}
	if report, err := InspectIntegrity(context.Background(), first); err != nil {
		t.Fatalf("InspectIntegrity(first) error = %v", err)
	} else if len(report.Drifts) != 0 {
		t.Fatalf("restored first platform drift = %+v", report.Drifts)
	}
}

func successiveActivationManifest(t *testing.T, first Manifest, firstManifest, firstReceipt []byte) Manifest {
	t.Helper()
	dir := first.CityPath
	coreSource := filepath.Join(dir, "candidate-gc-v2")
	rulesSource := filepath.Join(dir, "asset-source", "native-v3.rules")
	validatorSource := filepath.Join(dir, "asset-source", "build-artifact-valid-v2.sh")
	for path, data := range map[string][]byte{
		coreSource:      []byte("candidate-v2"),
		rulesSource:     []byte("rules-v3"),
		validatorSource: []byte("validator-v2"),
	} {
		if err := os.WriteFile(path, data, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	second := Manifest{
		Schema:    ManifestSchemaV1,
		ReleaseID: "v1.4.1-loucmane.2-successive",
		CityPath:  dir,
		Core: Artifact{
			Name:        "gc",
			Source:      coreSource,
			Destination: first.Core.Destination,
			SHA256:      testSHA256([]byte("candidate-v2")),
			Mode:        0o755,
		},
		ManagedFiles: []ManagedFile{
			{
				Name:           "control-rules",
				Source:         rulesSource,
				Destination:    first.ManagedFiles[0].Destination,
				SHA256:         testSHA256([]byte("rules-v3")),
				Mode:           0o644,
				PreviousSHA256: first.ManagedFiles[0].SHA256,
				BackupPath:     filepath.Join(dir, "backups", "native.rules.release-1"),
			},
			{
				Name:           "validator",
				Source:         validatorSource,
				Destination:    first.ManagedFiles[1].Destination,
				SHA256:         testSHA256([]byte("validator-v2")),
				Mode:           0o755,
				PreviousSHA256: first.ManagedFiles[1].SHA256,
				BackupPath:     filepath.Join(dir, "backups", "validator.release-1"),
			},
		},
		PreviousMetadata: &PreviousMetadataSpec{
			ManifestSHA256:     testSHA256(firstManifest),
			ManifestBackupPath: filepath.Join(dir, "backups", "install-manifest.release-1.json"),
			ReceiptSHA256:      testSHA256(firstReceipt),
			ReceiptBackupPath:  filepath.Join(dir, "backups", "install-receipt.release-1.json"),
		},
		PreviousSHA256: first.Core.SHA256,
		BackupPath:     filepath.Join(dir, "backups", "gc.release-1"),
		ReceiptPath:    first.ReceiptPath,
		Activation: &ActivationSpec{
			ExpectedCommit:  "1111111111111111111111111111111111111111",
			ExpectedVersion: "gc version 1.4.1-successive",
			PreviousCommit:  first.Activation.ExpectedCommit,
			PreviousVersion: first.Activation.ExpectedVersion,
		},
	}
	return finalizeManifest(t, second)
}

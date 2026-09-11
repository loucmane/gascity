package platforminstall

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/builtinpacks"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/packman"
)

func TestLegacyMetadataOnlyAdoptionNeverEntersRepairCapableAssurance(t *testing.T) {
	manifest := metadataOnlyManifest(t)
	installCandidateFilesystem(t, manifest)
	installer := newInstaller()
	installer.ensurePackCache = func(Manifest, *preflight) error {
		return errors.New("repair-capable cache assurance reached")
	}
	_, err := installer.adoptWithMode(context.Background(), manifest, exactFakeLifecycle(manifest), true)
	if err != nil {
		t.Fatalf("metadata-only adoption error = %v", err)
	}
}

func TestLegacyMetadataOnlyAdoptionPlanAndReplayVerifyRuntimeWithoutArtifactWrites(t *testing.T) {
	manifest := metadataOnlyManifest(t)
	installCandidateFilesystem(t, manifest)
	lifecycle := exactFakeLifecycle(manifest)
	steps, err := legacyMetadataOnlyPlan(context.Background(), manifest, lifecycle)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range steps {
		if step.Mutates && step.Action != "publish-manifest" && step.Action != "write-activation-receipt" {
			t.Fatalf("unexpected metadata-only mutation: %+v", step)
		}
	}
	assertPathAbsent(t, manifest.ReceiptPath)
	before := mustStat(t, manifest.Core.Destination)
	if _, err := legacyMetadataOnlyAdoptForTest(context.Background(), manifest, lifecycle); err != nil {
		t.Fatal(err)
	}
	receiptBefore := mustStat(t, manifest.ReceiptPath)
	receipt, err := legacyMetadataOnlyAdoptForTest(context.Background(), manifest, lifecycle)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Result != ResultNoop || lifecycle.restarts != 0 || lifecycle.verifies != 3 {
		t.Fatalf("result=%s restarts=%d verifies=%d; want noop/0/3", receipt.Result, lifecycle.restarts, lifecycle.verifies)
	}
	assertSameFileIdentityAndTime(t, manifest.Core.Destination, before)
	assertSameFileIdentityAndTime(t, manifest.ReceiptPath, receiptBefore)
}

func TestLegacyMetadataOnlyAdoptionRefusesArtifactAndRuntimeDrift(t *testing.T) {
	for _, scenario := range []string{"managed-delta", "core-mode", "managed-mode", "missing-backup", "runtime-mismatch", "runtime-error", "manifest-digest"} {
		t.Run(scenario, func(t *testing.T) {
			manifest := metadataOnlyManifest(t)
			if scenario == "managed-delta" {
				installBrokerActivatedCore(t, manifest)
			} else {
				installCandidateFilesystem(t, manifest)
			}
			lifecycle := exactFakeLifecycle(manifest)
			switch scenario {
			case "core-mode":
				if err := os.Chmod(manifest.Core.Destination, 0o644); err != nil {
					t.Fatal(err)
				}
			case "managed-mode":
				if err := os.Chmod(manifest.ManagedFiles[0].Destination, 0o600); err != nil {
					t.Fatal(err)
				}
			case "missing-backup":
				if err := os.Remove(manifest.ManagedFiles[0].BackupPath); err != nil {
					t.Fatal(err)
				}
			case "runtime-mismatch":
				lifecycle.proof.Commit = strings.Repeat("f", 40)
			case "runtime-error":
				lifecycle.verifyErr = errors.New("genuine runtime unavailable")
			case "manifest-digest":
				manifest.ManifestSHA256 = strings.Repeat("0", 64)
			}
			before := mustStat(t, manifest.Core.Destination)
			if _, err := legacyMetadataOnlyAdoptForTest(context.Background(), manifest, lifecycle); err == nil {
				t.Fatal("metadata-only adoption accepted drift")
			}
			assertSameFileIdentityAndTime(t, manifest.Core.Destination, before)
			assertPathAbsent(t, manifest.ReceiptPath)
			assertPathAbsent(t, DefaultManifestPath(manifest.CityPath))
			if lifecycle.restarts != 0 {
				t.Fatal("metadata-only refusal restarted runtime")
			}
		})
	}
}

func TestLegacyMetadataOnlyAdoptionReceiptFailureRollsBackMetadataOnly(t *testing.T) {
	manifest := metadataOnlyManifest(t)
	installCandidateFilesystem(t, manifest)
	before := map[string]os.FileInfo{}
	for _, filename := range []string{manifest.Core.Destination, manifest.ManagedFiles[0].Destination, manifest.ManagedFiles[1].Destination, manifest.BackupPath} {
		before[filename] = mustStat(t, filename)
	}
	installer := newInstaller()
	installer.writeReceipt = func(string, Receipt) error { return errors.New("injected metadata receipt failure") }
	_, err := installer.adoptWithMode(context.Background(), manifest, exactFakeLifecycle(manifest), true)
	if err == nil || !strings.Contains(err.Error(), "injected metadata receipt failure") {
		t.Fatalf("receipt failure = %v", err)
	}
	for filename, info := range before {
		assertSameFileIdentityAndTime(t, filename, info)
	}
	assertPathAbsent(t, manifest.ReceiptPath)
	assertPathAbsent(t, DefaultManifestPath(manifest.CityPath))
}

func TestLegacyMetadataOnlyAdoptionValidatesCacheOnInitialApplyAndReplay(t *testing.T) {
	for _, scenario := range []string{"valid", "missing-lock", "missing-cache", "dirty-cache", "dirty-replay"} {
		t.Run(scenario, func(t *testing.T) {
			manifest, cachePath, packPath := metadataOnlyPackFixture(t)
			if scenario == "dirty-replay" {
				if _, err := legacyMetadataOnlyAdoptForTest(context.Background(), manifest, exactFakeLifecycle(manifest)); err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "missing-lock":
				if err := os.Remove(filepath.Join(filepath.Dir(cachePath), ".packman-cache.lock")); err != nil {
					t.Fatal(err)
				}
			case "missing-cache":
				if err := os.RemoveAll(cachePath); err != nil {
					t.Fatal(err)
				}
			case "dirty-cache", "dirty-replay":
				if err := os.WriteFile(packPath, []byte("preserved drift"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			before := mustStat(t, manifest.Core.Destination)
			_, err := legacyMetadataOnlyAdoptForTest(context.Background(), manifest, exactFakeLifecycle(manifest))
			if scenario == "valid" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("invalid cache was accepted")
			}
			if scenario != "valid" && scenario != "dirty-replay" {
				assertPathAbsent(t, manifest.ReceiptPath)
				assertPathAbsent(t, DefaultManifestPath(manifest.CityPath))
			}
			if scenario == "dirty-cache" || scenario == "dirty-replay" {
				if !bytes.Equal(mustReadFile(t, packPath), []byte("preserved drift")) {
					t.Fatal("cache drift was repaired")
				}
			}
			if scenario == "missing-lock" {
				assertPathAbsent(t, filepath.Join(filepath.Dir(cachePath), ".packman-cache.lock"))
			}
			if scenario == "missing-cache" {
				assertPathAbsent(t, cachePath)
			}
			assertSameFileIdentityAndTime(t, manifest.Core.Destination, before)
		})
	}
}

func TestLegacyMetadataOnlyAdoptionChecksImportsWithoutNamedPackArtifacts(t *testing.T) {
	manifest, _, packPath := metadataOnlyPackFixture(t)
	for index := range manifest.ManagedFiles {
		manifest.ManagedFiles[index].Name = "unnamed-" + manifest.ManagedFiles[index].Name
	}
	manifest = finalizeManifest(t, manifest)
	if err := os.WriteFile(packPath, []byte("preserved unbound drift"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := legacyMetadataOnlyAdoptForTest(context.Background(), manifest, exactFakeLifecycle(manifest)); err == nil {
		t.Fatal("unbound installed imports bypassed the read-only cache check")
	}
	assertPathAbsent(t, manifest.ReceiptPath)
	assertPathAbsent(t, DefaultManifestPath(manifest.CityPath))
}

func TestLegacyMetadataOnlyAdoptionRefusesMetadataInsideCache(t *testing.T) {
	for _, scenario := range []string{"direct", "symlink-parent"} {
		t.Run(scenario, func(t *testing.T) {
			manifest, cachePath, _ := metadataOnlyPackFixture(t)
			outputDirectory := cachePath
			if scenario == "symlink-parent" {
				outputDirectory = filepath.Join(t.TempDir(), "cache-alias")
				if err := os.Symlink(cachePath, outputDirectory); err != nil {
					t.Fatal(err)
				}
			}
			manifest.ReceiptPath = filepath.Join(outputDirectory, "new-metadata", "receipt.json")
			manifest = finalizeManifest(t, manifest)
			if _, err := legacyMetadataOnlyPlan(context.Background(), manifest, exactFakeLifecycle(manifest)); err == nil {
				t.Fatal("plan accepted a cache-overlapping metadata destination")
			}
			if _, err := legacyMetadataOnlyAdoptForTest(context.Background(), manifest, exactFakeLifecycle(manifest)); err == nil {
				t.Fatal("apply accepted a cache-overlapping metadata destination")
			}
			assertPathAbsent(t, filepath.Join(cachePath, "new-metadata"))
			assertPathAbsent(t, DefaultManifestPath(manifest.CityPath))
		})
	}
}

func metadataOnlyManifest(t *testing.T) Manifest {
	t.Helper()
	t.Setenv("GC_HOME", t.TempDir())
	root, err := packman.RepoCacheRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".packman-cache.lock"), nil, 0o400); err != nil {
		t.Fatal(err)
	}
	return activationManifest(t, t.TempDir())
}

func metadataOnlyPackFixture(t *testing.T) (Manifest, string, string) {
	t.Helper()
	t.Setenv("GC_HOME", t.TempDir())
	source := builtinpacks.MustSource("core")
	version := config.BundledSourcePinnedVersion(source)
	commit := strings.TrimPrefix(version, "sha:")
	lock := []byte(fmt.Sprintf("schema = 1\n[packs.%q]\nversion = %q\ncommit = %q\n", source, version, commit))
	manifest := testPackReleaseManifest(t, t.TempDir(), lock)
	manifest.Activation = &ActivationSpec{ExpectedCommit: strings.Repeat("1", 40), ExpectedVersion: "gc version test", PreviousCommit: strings.Repeat("2", 40), PreviousVersion: "gc version prior"}
	pack := []byte(fmt.Sprintf("[pack]\nname = \"city\"\nschema = 2\n[imports.core]\nsource = %q\nversion = %q\n", source, version))
	for index := range manifest.ManagedFiles {
		file := &manifest.ManagedFiles[index]
		if file.Name == managedPackManifestName {
			if err := os.WriteFile(file.Source, pack, 0o644); err != nil {
				t.Fatal(err)
			}
			file.SHA256 = testSHA256(pack)
		}
	}
	manifest = finalizeManifest(t, manifest)
	installCandidateFilesystem(t, manifest)
	cachePath, err := packman.RepoCachePath(source, commit)
	if err != nil {
		t.Fatal(err)
	}
	if err := builtinpacks.MaterializeSyntheticRepo(cachePath, commit); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(cachePath), ".packman-cache.lock"), nil, 0o400); err != nil {
		t.Fatal(err)
	}
	bundled, ok := builtinpacks.ByName("core")
	if !ok {
		t.Fatal("core bundled fixture is unavailable")
	}
	return manifest, cachePath, filepath.Join(cachePath, bundled.Subpath, "pack.toml")
}

func legacyMetadataOnlyAdoptForTest(ctx context.Context, manifest Manifest, lifecycle Lifecycle) (Receipt, error) {
	return newInstaller().adoptWithMode(ctx, manifest, lifecycle, true)
}

func TestMetadataOnlyPublicEntrypointsRefuseLegacyLifecycleFallback(t *testing.T) {
	manifest := metadataOnlyManifest(t)
	lifecycle := exactFakeLifecycle(manifest)
	if _, err := AdoptMetadataOnlyPlan(context.Background(), manifest, lifecycle); err == nil {
		t.Fatal("legacy plan fallback accepted")
	}
	if _, err := AdoptMetadataOnly(context.Background(), manifest, lifecycle); err == nil {
		t.Fatal("legacy writer fallback accepted")
	}
	if lifecycle.verifies != 0 || lifecycle.restarts != 0 {
		t.Fatal("legacy lifecycle invoked")
	}
}

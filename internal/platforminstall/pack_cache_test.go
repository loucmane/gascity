package platforminstall

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/packman"
)

func TestPlanPackReleaseEnsuresCandidateCacheBeforeLiveMutation(t *testing.T) {
	dir := t.TempDir()
	manifest := testPackReleaseManifest(t, dir, []byte(`schema = 1

[packs."https://example.invalid/gascity-packs.git"]
version = "sha:0123456789abcdef"
commit = "0123456789abcdef"
fetched = "2026-08-22T00:00:00Z"
`))

	steps, err := Plan(manifest)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	cacheStep := -1
	firstMutation := -1
	for index, step := range steps {
		if step.Action == "ensure-pack-cache" {
			cacheStep = index
		}
		if firstMutation == -1 && step.Mutates {
			firstMutation = index
		}
	}
	if cacheStep == -1 {
		t.Fatalf("Plan() is missing ensure-pack-cache: %+v", steps)
	}
	if firstMutation == -1 || cacheStep > firstMutation {
		t.Fatalf("ensure-pack-cache index=%d, first mutation index=%d; want cache preflight first", cacheStep, firstMutation)
	}
}

func TestInstallRejectsInvalidCandidatePackLockBeforeLiveMutation(t *testing.T) {
	dir := t.TempDir()
	manifest := testPackReleaseManifest(t, dir, []byte(`schema = 1

[packs."https://example.invalid/gascity-packs.git"]
version = "sha:0123456789abcdef"
commit = ""
fetched = "2026-08-22T00:00:00Z"
`))
	tracked := []string{
		manifest.Core.Destination,
		manifest.ManagedFiles[0].Destination,
		manifest.ManagedFiles[1].Destination,
	}
	beforeInfo := make(map[string]os.FileInfo, len(tracked))
	beforeBytes := make(map[string][]byte, len(tracked))
	for _, path := range tracked {
		beforeInfo[path] = mustStat(t, path)
		beforeBytes[path] = mustReadFile(t, path)
	}

	_, err := Install(manifest)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("pack cache")) {
		t.Fatalf("Install() error = %v, want candidate pack cache refusal", err)
	}
	for _, path := range tracked {
		assertSameFileIdentityAndTime(t, path, beforeInfo[path])
		if got := mustReadFile(t, path); !bytes.Equal(got, beforeBytes[path]) {
			t.Fatalf("%s changed after pack-cache preflight failure: got %q want %q", path, got, beforeBytes[path])
		}
	}
	assertPathAbsent(t, manifest.BackupPath)
	assertPathAbsent(t, manifest.ManagedFiles[0].BackupPath)
	assertPathAbsent(t, manifest.ManagedFiles[1].BackupPath)
	assertPathAbsent(t, manifest.ReceiptPath)
	assertPathAbsent(t, DefaultManifestPath(manifest.CityPath))
}

func TestInstallPackCacheFailurePrecedesEveryLiveMutation(t *testing.T) {
	dir := t.TempDir()
	manifest := testPackReleaseManifest(t, dir, []byte(`schema = 1

[packs."https://example.invalid/gascity-packs.git"]
version = "sha:0123456789abcdef"
commit = "0123456789abcdef"
fetched = "2026-08-22T00:00:00Z"
`))
	tracked := []string{
		manifest.Core.Destination,
		manifest.ManagedFiles[0].Destination,
		manifest.ManagedFiles[1].Destination,
	}
	beforeInfo := make(map[string]os.FileInfo, len(tracked))
	beforeBytes := make(map[string][]byte, len(tracked))
	for _, path := range tracked {
		beforeInfo[path] = mustStat(t, path)
		beforeBytes[path] = mustReadFile(t, path)
	}
	installer := newInstaller()
	installer.ensurePackCache = func(Manifest, *preflight) error {
		return errors.New("injected bad candidate pin")
	}

	_, err := installer.install(manifest)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("injected bad candidate pin")) {
		t.Fatalf("install() error = %v, want injected pack-cache failure", err)
	}
	for _, path := range tracked {
		assertSameFileIdentityAndTime(t, path, beforeInfo[path])
		if got := mustReadFile(t, path); !bytes.Equal(got, beforeBytes[path]) {
			t.Fatalf("%s changed after pack-cache failure: got %q want %q", path, got, beforeBytes[path])
		}
	}
	assertPathAbsent(t, manifest.BackupPath)
	assertPathAbsent(t, manifest.ManagedFiles[0].BackupPath)
	assertPathAbsent(t, manifest.ManagedFiles[1].BackupPath)
	assertPathAbsent(t, manifest.ReceiptPath)
	assertPathAbsent(t, DefaultManifestPath(manifest.CityPath))
}

func TestCandidatePackCacheIncludesCityScopedImports(t *testing.T) {
	dir := t.TempDir()
	manifest := testPackReleaseManifest(t, dir, []byte("schema = 1\n"))
	cityConfig := []byte(`[defaults.rig.imports.gc]
source = "https://example.invalid/gascity-packs.git/roles"
version = "sha:fedcba9876543210"

[[rigs]]
name = "example"
[rigs.imports.gc]
source = "https://example.invalid/gascity-packs.git/roles"
version = "sha:fedcba9876543210"
`)
	if err := os.WriteFile(filepath.Join(dir, "city.toml"), cityConfig, 0o644); err != nil {
		t.Fatal(err)
	}
	state, err := preflightManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	previous := installLockedCandidateWithImports
	t.Cleanup(func() { installLockedCandidateWithImports = previous })
	var got map[string]config.Import
	installLockedCandidateWithImports = func(_ string, credentialCityRoot string, imports map[string]config.Import) (*packman.Lockfile, error) {
		if credentialCityRoot != dir {
			t.Fatalf("credential city root = %q, want %q", credentialCityRoot, dir)
		}
		got = imports
		return &packman.Lockfile{}, nil
	}

	if err := ensureCandidatePackCache(manifest, state); err != nil {
		t.Fatalf("ensureCandidatePackCache() error = %v", err)
	}
	for _, key := range []string{"pack:gascity", "default-rig:gc", "rig:example:gc"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("candidate imports missing %q: %#v", key, got)
		}
	}
}

func testPackReleaseManifest(t *testing.T, dir string, candidateLock []byte) Manifest {
	t.Helper()
	manifest := testManifest(t, dir, []byte("candidate"), []byte("installed"))
	assetDir := filepath.Join(dir, "candidate-platform")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	candidatePack := []byte(`[pack]
name = "city"
schema = 2

[imports.gascity]
source = "https://example.invalid/gascity-packs.git"
version = "sha:0123456789abcdef"
`)
	previousPack := []byte("[pack]\nname = \"previous-city\"\nschema = 2\n")
	previousLock := []byte("schema = 1\n")
	packSource := filepath.Join(assetDir, "pack.toml")
	lockSource := filepath.Join(assetDir, "packs.lock")
	packDestination := filepath.Join(dir, "pack.toml")
	lockDestination := filepath.Join(dir, "packs.lock")
	for path, data := range map[string][]byte{
		packSource:      candidatePack,
		lockSource:      candidateLock,
		packDestination: previousPack,
		lockDestination: previousLock,
	} {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifest.ManagedFiles = []ManagedFile{
		{
			Name:           "pack-lock",
			Source:         lockSource,
			Destination:    lockDestination,
			SHA256:         testSHA256(candidateLock),
			Mode:           0o644,
			PreviousSHA256: testSHA256(previousLock),
			BackupPath:     filepath.Join(dir, "backups", "packs.lock.previous"),
		},
		{
			Name:           "pack-manifest",
			Source:         packSource,
			Destination:    packDestination,
			SHA256:         testSHA256(candidatePack),
			Mode:           0o644,
			PreviousSHA256: testSHA256(previousPack),
			BackupPath:     filepath.Join(dir, "backups", "pack.toml.previous"),
		},
	}
	return finalizeManifest(t, manifest)
}

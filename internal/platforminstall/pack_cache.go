package platforminstall

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gastownhall/gascity/internal/packman"
)

const (
	managedPackLockName     = "pack-lock"
	managedPackManifestName = "pack-manifest"
)

type candidatePackPair struct {
	manifest []byte
	lock     []byte
	lockFile ManagedFile
}

func ensureCandidatePackCache(manifest Manifest, state *preflight) error {
	pair, err := candidatePackFiles(manifest, state)
	if err != nil || pair == nil {
		return err
	}
	stage, err := os.MkdirTemp("", ".gc-platform-pack-candidate-*")
	if err != nil {
		return fmt.Errorf("create candidate pack staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()
	if err := os.WriteFile(filepath.Join(stage, "pack.toml"), pair.manifest, 0o644); err != nil {
		return fmt.Errorf("stage candidate pack.toml: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stage, packman.LockfileName), pair.lock, 0o644); err != nil {
		return fmt.Errorf("stage candidate %s: %w", packman.LockfileName, err)
	}
	if _, err := packman.InstallLockedCandidate(stage, manifest.CityPath); err != nil {
		return fmt.Errorf("install candidate pack cache: %w", err)
	}
	return nil
}

func candidatePackFiles(manifest Manifest, state *preflight) (*candidatePackPair, error) {
	var packManifest *managedFilePreflight
	var packLock *managedFilePreflight
	for index := range state.managedFiles {
		file := &state.managedFiles[index]
		switch file.file.Name {
		case managedPackManifestName:
			packManifest = file
		case managedPackLockName:
			packLock = file
		}
	}
	if packManifest == nil && packLock == nil {
		return nil, nil
	}
	if packManifest == nil || packLock == nil {
		return nil, fmt.Errorf("managed pack release requires both %q and %q files", managedPackManifestName, managedPackLockName)
	}
	wantManifest := filepath.Join(manifest.CityPath, "pack.toml")
	if packManifest.file.Destination != wantManifest {
		return nil, fmt.Errorf("managed %q destination is %q, want %q", managedPackManifestName, packManifest.file.Destination, wantManifest)
	}
	wantLock := filepath.Join(manifest.CityPath, packman.LockfileName)
	if packLock.file.Destination != wantLock {
		return nil, fmt.Errorf("managed %q destination is %q, want %q", managedPackLockName, packLock.file.Destination, wantLock)
	}
	return &candidatePackPair{
		manifest: packManifest.candidate,
		lock:     packLock.candidate,
		lockFile: packLock.file,
	}, nil
}

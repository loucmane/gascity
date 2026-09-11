package platforminstall

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/gastownhall/gascity/internal/importsvc"
	"github.com/gastownhall/gascity/internal/packman"
)

// AdoptMetadataOnlyPlan validates installed artifacts, cache state and the
// running supervisor without publishing metadata or repairing cache state.
func AdoptMetadataOnlyPlan(ctx context.Context, manifest Manifest, _ Lifecycle) ([]PlanStep, error) {
	_, steps, err := RunMetadataTransaction(ctx, manifest, true)
	return steps, err
}

func legacyMetadataOnlyPlan(ctx context.Context, manifest Manifest, lifecycle Lifecycle) ([]PlanStep, error) {
	if lifecycle == nil || manifest.Activation == nil {
		return nil, fmt.Errorf("metadata-only adoption requires an activation lifecycle and manifest")
	}
	var steps []PlanStep
	err := withMetadataOnlyCacheLock(func() error {
		var err error
		steps, err = adoptPlan(ctx, manifest, true)
		if err != nil {
			return err
		}
		proof, err := lifecycle.Verify(ctx, manifest)
		if err != nil {
			return fmt.Errorf("verify broker-activated supervisor: %w", err)
		}
		return validateRuntimeProof(manifest, proof)
	})
	if err != nil {
		return nil, err
	}
	return steps, nil
}

// AdoptMetadataOnly publishes only platform metadata for an exact installed
// candidate. Cache or managed-artifact drift is refused, never repaired.
func AdoptMetadataOnly(ctx context.Context, manifest Manifest, _ Lifecycle) (Receipt, error) {
	receipt, _, err := RunMetadataTransaction(ctx, manifest, false)
	return receipt, err
}

func withMetadataOnlyCacheLock(check func() error) error {
	root, err := packman.RepoCacheRoot()
	if err != nil {
		return err
	}
	return config.WithExistingRepoCacheReadLock(root, check)
}

func preflightMetadataOnlyArtifacts(manifest Manifest, state *preflight) error {
	if err := preflightMetadataOnlyPaths(manifest); err != nil {
		return err
	}
	if !allManagedFilesInstalled(state.managedFiles) {
		return fmt.Errorf("metadata-only adoption requires all managed artifacts already installed")
	}
	report := IntegrityReport{}
	inspectRegularFile(&report, "core", manifest.Core.Destination, manifest.Core.SHA256, manifest.Core.Mode)
	inspectRegularFile(&report, "backup", manifest.BackupPath, manifest.PreviousSHA256, manifest.Core.Mode)
	for _, file := range state.managedFiles {
		inspectRegularFile(&report, "managed_files["+file.file.Name+"]", file.file.Destination, file.file.SHA256, file.file.Mode)
		if file.file.PreviousSHA256 != "" {
			if !file.reuseBackup {
				return fmt.Errorf("metadata-only adoption requires existing managed backup %q", file.file.Name)
			}
			inspectRegularFile(&report, "managed_backup["+file.file.Name+"]", file.file.BackupPath, file.file.PreviousSHA256, file.file.Mode)
		}
	}
	if len(report.Drifts) != 0 {
		return fmt.Errorf("metadata-only artifact drift: %+v", report.Drifts)
	}
	return nil
}

func checkCandidatePackCacheReadOnly(manifest Manifest, state *preflight) error {
	return checkCandidatePackCacheReadOnlyContext(context.Background(), manifest, state)
}

func checkCandidatePackCacheReadOnlyContext(ctx context.Context, manifest Manifest, state *preflight) error {
	if _, err := candidatePackFiles(manifest, state); err != nil {
		return err
	}
	var inputFS fsys.FS = fsys.OSFS{}
	if manifest.Metadata != nil {
		inputFS = metadataImportFS(manifest.Metadata)
	}
	imports, err := importsvc.CollectAllImports(inputFS, manifest.CityPath)
	if err != nil {
		return fmt.Errorf("read installed city imports: %w", err)
	}
	var report *packman.CheckReport
	if manifest.Metadata != nil {
		ctx = context.WithValue(ctx, metadataInspectionKey{}, true)
		report, err = packman.CheckInstalledReadOnlyStrict(manifest.CityPath, imports, func(directory string, args ...string) (string, error) {
			return runInspectionCommand(ctx, "git", append([]string{"-C", directory}, args...)...)
		})
	} else {
		report, err = packman.CheckInstalledReadOnly(manifest.CityPath, imports)
	}
	if err != nil {
		return err
	}
	if report.HasIssues() {
		return fmt.Errorf("installed pack cache requires repair; metadata-only adoption refuses: %+v", report.Issues)
	}
	return nil
}

func preflightMetadataOnlyPaths(manifest Manifest) error {
	cacheRoot, err := packman.RepoCacheRoot()
	if err != nil {
		return err
	}
	cacheRoot, err = filepath.EvalSymlinks(cacheRoot)
	if err != nil {
		return fmt.Errorf("resolve metadata-only cache root: %w", err)
	}
	outputs := []string{DefaultManifestPath(manifest.CityPath), manifest.ReceiptPath}
	if manifest.PreviousMetadata != nil {
		outputs = append(outputs, manifest.PreviousMetadata.ManifestBackupPath, manifest.PreviousMetadata.ReceiptBackupPath)
	}
	for _, output := range outputs {
		resolved, err := resolveMetadataOutput(output)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(cacheRoot, resolved)
		if err != nil {
			return fmt.Errorf("compare metadata output with cache: %w", err)
		}
		if relative == "." || (!filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))) {
			return fmt.Errorf("metadata-only output %q overlaps the repo cache", output)
		}
	}
	return nil
}

func resolveMetadataOutput(output string) (string, error) {
	ancestor := filepath.Clean(output)
	var suffix []string
	for {
		_, err := os.Lstat(ancestor)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(ancestor)
			if err != nil {
				return "", fmt.Errorf("resolve metadata output %q: %w", output, err)
			}
			for index := len(suffix) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, suffix[index])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) || filepath.Dir(ancestor) == ancestor {
			return "", fmt.Errorf("inspect metadata output %q: %w", output, err)
		}
		suffix = append(suffix, filepath.Base(ancestor))
		ancestor = filepath.Dir(ancestor)
	}
}

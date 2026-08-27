package platforminstall

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// AdoptPlan validates that the complete candidate filesystem is already in
// place and returns the metadata-only adoption plan without mutation.
func AdoptPlan(manifest Manifest) ([]PlanStep, error) {
	state, err := preflightManifest(manifest)
	if err != nil {
		return nil, err
	}
	if !state.coreAlreadyInstalled || !allManagedFilesInstalled(state.managedFiles) {
		return nil, fmt.Errorf("platform adoption requires the complete candidate filesystem state")
	}
	if report := inspectPinnedIntegrity(context.Background(), manifest); len(report.Drifts) != 0 {
		return nil, fmt.Errorf("preflight platform integrity drift: %+v", report.Drifts)
	}
	steps := []PlanStep{
		{Action: "verify-manifest", Path: DefaultManifestPath(manifest.CityPath), SHA256: manifest.ManifestSHA256},
		{Action: "verify-installed-candidate", Path: manifest.Core.Destination, SHA256: manifest.Core.SHA256},
		{Action: "verify-broker-activated-runtime", Path: manifest.Core.Destination, SHA256: manifest.Core.SHA256},
	}
	if state.previousMetadata != nil {
		steps = append(steps,
			PlanStep{Action: "write-previous-manifest-backup", Path: manifest.PreviousMetadata.ManifestBackupPath, SHA256: manifest.PreviousMetadata.ManifestSHA256, Mutates: !state.previousMetadata.reuseManifestBackup},
			PlanStep{Action: "write-previous-receipt-backup", Path: manifest.PreviousMetadata.ReceiptBackupPath, SHA256: manifest.PreviousMetadata.ReceiptSHA256, Mutates: !state.previousMetadata.reuseReceiptBackup},
		)
	}
	steps = append(steps,
		PlanStep{Action: "publish-manifest", Path: DefaultManifestPath(manifest.CityPath), SHA256: manifest.ManifestSHA256, Mutates: !state.reuseManifest},
		PlanStep{Action: "write-activation-receipt", Path: manifest.ReceiptPath, Mutates: state.noopReceipt == nil || state.noopReceipt.Activation == nil},
		PlanStep{Action: "verify-integrity", Path: manifest.CityPath},
	)
	for index := range steps {
		steps[index].Order = index + 1
	}
	return steps, nil
}

// Adopt records an exact platform that was already installed and activated by
// the fixed-operation privileged broker. It never writes the core artifact or
// managed files and never restarts the supervisor.
func Adopt(ctx context.Context, manifest Manifest, lifecycle Lifecycle) (Receipt, error) {
	return newInstaller().adopt(ctx, manifest, lifecycle)
}

func (i *installer) adopt(ctx context.Context, manifest Manifest, lifecycle Lifecycle) (Receipt, error) {
	if lifecycle == nil {
		return Receipt{}, fmt.Errorf("platform activation lifecycle is required")
	}
	if manifest.Activation == nil {
		return Receipt{}, fmt.Errorf("manifest activation is required")
	}
	state, err := preflightManifest(manifest)
	if err != nil {
		return Receipt{}, err
	}
	if !state.coreAlreadyInstalled || !allManagedFilesInstalled(state.managedFiles) {
		return Receipt{}, fmt.Errorf("platform adoption requires the complete candidate filesystem state")
	}
	if report := inspectPinnedIntegrity(ctx, manifest); len(report.Drifts) != 0 {
		return Receipt{}, fmt.Errorf("preflight platform integrity drift: %+v", report.Drifts)
	}
	proof, err := lifecycle.Verify(ctx, manifest)
	if err != nil {
		return Receipt{}, fmt.Errorf("verify broker-activated supervisor: %w", err)
	}
	if err := validateRuntimeProof(manifest, proof); err != nil {
		return Receipt{}, err
	}
	if state.noopReceipt != nil && state.noopReceipt.Activation != nil {
		result := *state.noopReceipt
		result.Result = ResultNoop
		if err := finalizeReceipt(&result); err != nil {
			return Receipt{}, err
		}
		return result, nil
	}

	directories := []string{
		filepath.Dir(manifest.ReceiptPath),
		filepath.Dir(DefaultManifestPath(manifest.CityPath)),
	}
	if manifest.PreviousMetadata != nil {
		directories = append(directories,
			filepath.Dir(manifest.PreviousMetadata.ManifestBackupPath),
			filepath.Dir(manifest.PreviousMetadata.ReceiptBackupPath),
		)
	}
	for _, directory := range directories {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return Receipt{}, fmt.Errorf("create adoption metadata directory %q: %w", directory, err)
		}
	}
	if err := i.preservePreviousMetadata(manifest, state); err != nil {
		return Receipt{}, err
	}

	receipt := Receipt{
		Schema:         receiptSchemaV1,
		ReleaseID:      manifest.ReleaseID,
		ManifestSHA256: manifest.ManifestSHA256,
		ArtifactSHA256: manifest.Core.SHA256,
		PreviousSHA256: manifest.PreviousSHA256,
		ManagedFiles:   receiptManagedFiles(manifest.ManagedFiles),
		Activation:     &proof,
		Result:         ResultInstalled,
	}
	if err := finalizeReceipt(&receipt); err != nil {
		return Receipt{}, err
	}

	metadataMutated := false
	restore := func(cause error) error {
		if !metadataMutated {
			return cause
		}
		if restoreErr := i.restorePlatformMetadata(manifest, state); restoreErr != nil {
			return fmt.Errorf("%w; metadata rollback also failed: %w", cause, restoreErr)
		}
		return cause
	}
	if !state.reuseManifest {
		metadataMutated = true
		if err := i.writeAtomic(DefaultManifestPath(manifest.CityPath), state.manifest, 0o644); err != nil {
			return Receipt{}, restore(fmt.Errorf("write adopted platform manifest: %w", err))
		}
	}
	metadataMutated = true
	if err := i.writeReceipt(manifest.ReceiptPath, receipt); err != nil {
		return Receipt{}, restore(fmt.Errorf("write adopted platform receipt: %w", err))
	}
	report, err := InspectIntegrity(ctx, manifest)
	if err != nil {
		return Receipt{}, restore(fmt.Errorf("inspect adopted platform: %w", err))
	}
	if len(report.Drifts) != 0 {
		return Receipt{}, restore(fmt.Errorf("adopted platform integrity drift: %+v", report.Drifts))
	}
	return receipt, nil
}

func (i *installer) preservePreviousMetadata(manifest Manifest, state *preflight) error {
	if state.previousMetadata == nil {
		return nil
	}
	if !state.previousMetadata.reuseManifestBackup {
		if err := i.writeAtomic(manifest.PreviousMetadata.ManifestBackupPath, state.previousMetadata.manifest, 0o644); err != nil {
			return fmt.Errorf("write exact prior-manifest backup: %w", err)
		}
	}
	if !state.previousMetadata.reuseReceiptBackup {
		if err := i.writeAtomic(manifest.PreviousMetadata.ReceiptBackupPath, state.previousMetadata.receipt, 0o644); err != nil {
			return fmt.Errorf("write exact prior-receipt backup: %w", err)
		}
	}
	if err := verifyRegularFileDigest(manifest.PreviousMetadata.ManifestBackupPath, manifest.PreviousMetadata.ManifestSHA256, "prior-manifest backup"); err != nil {
		return err
	}
	return verifyRegularFileDigest(manifest.PreviousMetadata.ReceiptBackupPath, manifest.PreviousMetadata.ReceiptSHA256, "prior-receipt backup")
}

package platforminstall

import (
	"context"
	"fmt"
)

// Lifecycle is the supervisor transition boundary used by Apply. Production
// supplies the Gas City supervisor implementation; tests use a deterministic
// fake and never touch a live process.
type Lifecycle interface {
	Restart(context.Context, Manifest) error
	Verify(context.Context, Manifest) (RuntimeProof, error)
}

// Apply installs the manifest and completes its explicitly authorized runtime
// activation. A fresh install restarts once. A replay first verifies the
// current runtime, allowing a crash after restart but before receipt finalizing
// to complete without an unnecessary second restart.
func Apply(ctx context.Context, manifest Manifest, lifecycle Lifecycle) (Receipt, error) {
	if lifecycle == nil {
		return Receipt{}, fmt.Errorf("platform activation lifecycle is required")
	}
	if manifest.Activation == nil {
		return Receipt{}, fmt.Errorf("manifest activation is required")
	}
	receipt, err := Install(manifest)
	if err != nil {
		return Receipt{}, err
	}

	if receipt.Result == ResultNoop {
		proof, verifyErr := lifecycle.Verify(ctx, manifest)
		if verifyErr == nil {
			if proofErr := validateRuntimeProof(manifest, proof); proofErr == nil {
				if receipt.Activation == nil {
					return finalizeActivationReceipt(ctx, manifest, receipt, proof)
				}
				return receipt, nil
			}
		}
	}

	if err := lifecycle.Restart(ctx, manifest); err != nil {
		return Receipt{}, rollbackActivationFailure(manifest, fmt.Errorf("restart supervisor: %w", err))
	}
	proof, err := lifecycle.Verify(ctx, manifest)
	if err != nil {
		return Receipt{}, rollbackActivationFailure(manifest, fmt.Errorf("verify restarted supervisor: %w", err))
	}
	if err := validateRuntimeProof(manifest, proof); err != nil {
		return Receipt{}, rollbackActivationFailure(manifest, err)
	}
	return finalizeActivationReceipt(ctx, manifest, receipt, proof)
}

// Rollback restores the exact pre-install filesystem state from manifest-bound
// backups and removes the candidate's manifest and receipt. Backups are
// retained as evidence and as independently verifiable recovery inputs.
func Rollback(manifest Manifest) error {
	state, err := preflightManifest(manifest)
	if err != nil {
		return fmt.Errorf("preflight rollback: %w", err)
	}
	if !state.coreAlreadyInstalled || !allManagedFilesInstalled(state.managedFiles) {
		return fmt.Errorf("rollback requires the complete candidate filesystem state")
	}
	return newInstaller().rollbackTransactionAndRemoveMetadata(manifest, state, true)
}

// Revert restores the prior filesystem and then activates and verifies the
// manifest-pinned previous runtime. Disk restoration deliberately precedes
// the single restart attempt: a failed restart leaves the known prior bytes
// in place for explicit operator recovery instead of retrying a transition.
func Revert(ctx context.Context, manifest Manifest, lifecycle Lifecycle) (RuntimeProof, error) {
	if lifecycle == nil {
		return RuntimeProof{}, fmt.Errorf("platform rollback lifecycle is required")
	}
	if manifest.Activation == nil {
		return RuntimeProof{}, fmt.Errorf("manifest activation is required")
	}
	if err := Rollback(manifest); err != nil {
		return RuntimeProof{}, err
	}

	previousManifest := manifest
	previousManifest.Core.SHA256 = manifest.PreviousSHA256
	previousManifest.Activation = &ActivationSpec{
		ExpectedCommit:  manifest.Activation.PreviousCommit,
		ExpectedVersion: manifest.Activation.PreviousVersion,
		PreviousCommit:  manifest.Activation.ExpectedCommit,
		PreviousVersion: manifest.Activation.ExpectedVersion,
	}
	if err := lifecycle.Restart(ctx, previousManifest); err != nil {
		return RuntimeProof{}, fmt.Errorf("restart previous supervisor: %w", err)
	}
	proof, err := lifecycle.Verify(ctx, previousManifest)
	if err != nil {
		return RuntimeProof{}, fmt.Errorf("verify previous supervisor: %w", err)
	}
	if err := validateRuntimeProof(previousManifest, proof); err != nil {
		return RuntimeProof{}, fmt.Errorf("verify previous runtime: %w", err)
	}
	return proof, nil
}

// RollbackPlan returns the ordered rollback without mutating the filesystem.
func RollbackPlan(manifest Manifest) ([]PlanStep, error) {
	state, err := preflightManifest(manifest)
	if err != nil {
		return nil, fmt.Errorf("preflight rollback plan: %w", err)
	}
	if !state.coreAlreadyInstalled || !allManagedFilesInstalled(state.managedFiles) {
		return nil, fmt.Errorf("rollback plan requires the complete candidate filesystem state")
	}
	if manifest.Activation == nil {
		return nil, fmt.Errorf("manifest activation is required")
	}

	steps := make([]PlanStep, 0, len(state.managedFiles)+5)
	for index := len(state.managedFiles) - 1; index >= 0; index-- {
		file := state.managedFiles[index]
		if file.previousPresent {
			steps = append(steps, PlanStep{
				Action:  "restore-managed-file:" + file.file.Name,
				Path:    file.file.Destination,
				SHA256:  file.file.PreviousSHA256,
				Mutates: true,
			})
			continue
		}
		steps = append(steps, PlanStep{
			Action:  "remove-managed-file:" + file.file.Name,
			Path:    file.file.Destination,
			Mutates: true,
		})
	}
	steps = append(steps,
		PlanStep{Action: "restore-core", Path: manifest.Core.Destination, SHA256: manifest.PreviousSHA256, Mutates: true},
		PlanStep{Action: "remove-receipt", Path: manifest.ReceiptPath, Mutates: true},
		PlanStep{Action: "remove-manifest", Path: DefaultManifestPath(manifest.CityPath), Mutates: true},
		PlanStep{Action: "restart-supervisor", Path: manifest.Core.Destination, SHA256: manifest.PreviousSHA256, Mutates: true},
		PlanStep{Action: "verify-previous-runtime", Path: manifest.Core.Destination, SHA256: manifest.PreviousSHA256},
	)
	for index := range steps {
		steps[index].Order = index + 1
	}
	return steps, nil
}

func finalizeActivationReceipt(ctx context.Context, manifest Manifest, receipt Receipt, proof RuntimeProof) (Receipt, error) {
	persisted := receipt
	persisted.Result = ResultInstalled
	persisted.Activation = &proof
	if err := finalizeReceipt(&persisted); err != nil {
		return Receipt{}, rollbackActivationFailure(manifest, fmt.Errorf("finalize activation receipt: %w", err))
	}
	if err := newInstaller().writeReceipt(manifest.ReceiptPath, persisted); err != nil {
		return Receipt{}, rollbackActivationFailure(manifest, fmt.Errorf("write activation receipt: %w", err))
	}
	report, err := InspectIntegrity(ctx, manifest)
	if err != nil {
		return Receipt{}, rollbackActivationFailure(manifest, fmt.Errorf("inspect activated platform: %w", err))
	}
	if len(report.Drifts) != 0 {
		return Receipt{}, rollbackActivationFailure(manifest, fmt.Errorf("activated platform integrity drift: %+v", report.Drifts))
	}
	if receipt.Result == ResultNoop {
		persisted.Result = ResultNoop
		if err := finalizeReceipt(&persisted); err != nil {
			return Receipt{}, err
		}
	}
	return persisted, nil
}

func validateRuntimeProof(manifest Manifest, proof RuntimeProof) error {
	if proof.ExecutableSHA256 != manifest.Core.SHA256 {
		return fmt.Errorf("runtime executable sha256 mismatch: got %s want %s", proof.ExecutableSHA256, manifest.Core.SHA256)
	}
	if proof.Commit != manifest.Activation.ExpectedCommit {
		return fmt.Errorf("runtime commit mismatch: got %s want %s", proof.Commit, manifest.Activation.ExpectedCommit)
	}
	if proof.Version != manifest.Activation.ExpectedVersion {
		return fmt.Errorf("runtime version mismatch: got %q want %q", proof.Version, manifest.Activation.ExpectedVersion)
	}
	return nil
}

func rollbackActivationFailure(manifest Manifest, cause error) error {
	if rollbackErr := Rollback(manifest); rollbackErr != nil {
		return fmt.Errorf("%w; rollback also failed: %w", cause, rollbackErr)
	}
	return cause
}

package platforminstall

import (
	"context"
	"fmt"
)

// PlanStep is one deterministic, ordered preflight or mutation in an install.
type PlanStep struct {
	Order   int
	Action  string
	Path    string
	SHA256  string
	Mutates bool
}

// Plan validates a manifest and returns the exact ordered install plan without mutation.
func Plan(manifest Manifest) ([]PlanStep, error) {
	state, err := preflightManifest(manifest)
	if err != nil {
		return nil, err
	}
	if report := inspectPinnedIntegrity(context.Background(), manifest); len(report.Drifts) != 0 {
		return nil, fmt.Errorf("preflight platform integrity drift: %+v", report.Drifts)
	}
	noop := state.noopReceipt != nil
	steps := []PlanStep{
		{Action: "verify-manifest", Path: DefaultManifestPath(manifest.CityPath), SHA256: manifest.ManifestSHA256},
		{Action: "verify-candidate", Path: manifest.Core.Source, SHA256: manifest.Core.SHA256},
		{Action: "verify-live-baseline", Path: manifest.Core.Destination, SHA256: state.previousSHA256},
		{Action: "write-backup", Path: manifest.BackupPath, SHA256: manifest.PreviousSHA256, Mutates: !noop && !state.reuseBackup},
	}
	for _, file := range state.managedFiles {
		if !file.previousPresent {
			continue
		}
		steps = append(steps, PlanStep{Action: "write-managed-backup:" + file.file.Name, Path: file.file.BackupPath, SHA256: file.file.PreviousSHA256, Mutates: !noop && !file.reuseBackup})
	}
	steps = append(steps, PlanStep{Action: "publish-core", Path: manifest.Core.Destination, SHA256: manifest.Core.SHA256, Mutates: !noop && !state.coreAlreadyInstalled})
	for _, file := range state.managedFiles {
		steps = append(steps, PlanStep{Action: "publish-managed-file:" + file.file.Name, Path: file.file.Destination, SHA256: file.file.SHA256, Mutates: !noop && !file.alreadyInstalled})
	}
	steps = append(steps,
		PlanStep{Action: "publish-manifest", Path: DefaultManifestPath(manifest.CityPath), SHA256: manifest.ManifestSHA256, Mutates: !noop && !state.reuseManifest},
		PlanStep{Action: "write-receipt", Path: manifest.ReceiptPath, Mutates: !noop},
	)
	if manifest.Activation != nil {
		steps = append(steps,
			PlanStep{Action: "verify-runtime-or-restart", Path: manifest.Core.Destination, SHA256: manifest.Core.SHA256},
			PlanStep{Action: "restart-supervisor-if-needed", Path: manifest.Core.Destination, SHA256: manifest.Core.SHA256, Mutates: true},
			PlanStep{Action: "write-activation-receipt", Path: manifest.ReceiptPath, Mutates: true},
		)
	}
	steps = append(steps, PlanStep{Action: "verify-integrity", Path: manifest.CityPath})
	for index := range steps {
		steps[index].Order = index + 1
	}
	return steps, nil
}

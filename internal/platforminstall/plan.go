package platforminstall

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
	noop := state.noopReceipt != nil
	return []PlanStep{
		{Order: 1, Action: "verify-manifest", Path: DefaultManifestPath(manifest.CityPath), SHA256: manifest.ManifestSHA256},
		{Order: 2, Action: "verify-candidate", Path: manifest.Core.Source, SHA256: manifest.Core.SHA256},
		{Order: 3, Action: "verify-live-baseline", Path: manifest.Core.Destination, SHA256: state.previousSHA256},
		{Order: 4, Action: "write-backup", Path: manifest.BackupPath, SHA256: manifest.PreviousSHA256, Mutates: !noop && !state.reuseBackup},
		{Order: 5, Action: "publish-core", Path: manifest.Core.Destination, SHA256: manifest.Core.SHA256, Mutates: !noop && !state.recoverReceipt},
		{Order: 6, Action: "publish-manifest", Path: DefaultManifestPath(manifest.CityPath), SHA256: manifest.ManifestSHA256, Mutates: !noop && !state.reuseManifest},
		{Order: 7, Action: "write-receipt", Path: manifest.ReceiptPath, Mutates: !noop},
		{Order: 8, Action: "verify-integrity", Path: manifest.CityPath},
	}, nil
}

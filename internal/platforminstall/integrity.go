package platforminstall

import (
	"context"
	"errors"
)

// FilePin identifies a managed file whose exact bytes and mode are authority.
type FilePin struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Mode   uint32 `json:"mode"`
}

// GitPin identifies a repository authority by exact checkout commit.
type GitPin struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Commit     string `json:"commit"`
	AllowDirty bool   `json:"allow_dirty,omitempty"`
}

// ProviderPin identifies the stable provider entrypoint and executable it must
// resolve to, including its bytes and reported version.
type ProviderPin struct {
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	ResolvedPath string   `json:"resolved_path"`
	SHA256       string   `json:"sha256"`
	VersionArgs  []string `json:"version_args"`
	Version      string   `json:"version"`
}

// IntegritySpec is the complete managed-platform fingerprint inspected by the
// installer doctor.
type IntegritySpec struct {
	Files        []FilePin     `json:"files,omitempty"`
	Repositories []GitPin      `json:"repositories,omitempty"`
	Providers    []ProviderPin `json:"providers,omitempty"`
}

// Drift describes one exact field that differs from its manifest authority.
type Drift struct {
	Field    string
	Expected string
	Actual   string
}

// IntegrityReport aggregates every drift found in one inspection.
type IntegrityReport struct {
	Drifts []Drift
}

// ErrIntegrityInspectionDisabled keeps the RED contract fail-closed until the
// complete inspector lands.
var ErrIntegrityInspectionDisabled = errors.New("platform integrity inspection is disabled")

// InspectIntegrity compares the live filesystem and tools with the manifest.
func InspectIntegrity(_ context.Context, _ Manifest) (IntegrityReport, error) {
	return IntegrityReport{}, ErrIntegrityInspectionDisabled
}

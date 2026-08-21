package doctor

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gastownhall/gascity/internal/platforminstall"
)

// PlatformInstallIntegrityCheck verifies an opted-in managed platform
// installation against its canonical manifest.
type PlatformInstallIntegrityCheck struct{}

// NewPlatformInstallIntegrityCheck creates the managed platform integrity check.
func NewPlatformInstallIntegrityCheck() *PlatformInstallIntegrityCheck {
	return &PlatformInstallIntegrityCheck{}
}

// Name returns the stable doctor check identifier.
func (c *PlatformInstallIntegrityCheck) Name() string { return "platform-install-integrity" }

// Run loads an opted-in manifest and reports every exact-field drift.
func (c *PlatformInstallIntegrityCheck) Run(ctx *CheckContext) *CheckResult {
	result := &CheckResult{Name: c.Name()}
	manifestPath := platforminstall.DefaultManifestPath(ctx.CityPath)
	info, err := os.Lstat(manifestPath)
	if os.IsNotExist(err) {
		result.Status = StatusOK
		result.Message = "city is not managed by a platform install manifest"
		return result
	}
	if err != nil {
		result.Status = StatusError
		result.Message = fmt.Sprintf("inspect platform manifest: %v", err)
		result.FixHint = platformIntegrityFixHint
		return result
	}
	if !info.Mode().IsRegular() {
		result.Status = StatusError
		result.Message = fmt.Sprintf("platform manifest must be a regular file, got mode %s", info.Mode())
		result.FixHint = platformIntegrityFixHint
		return result
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		result.Status = StatusError
		result.Message = fmt.Sprintf("read platform manifest: %v", err)
		result.FixHint = platformIntegrityFixHint
		return result
	}
	manifest, err := platforminstall.LoadManifest(data)
	if err != nil {
		result.Status = StatusError
		result.Message = fmt.Sprintf("invalid platform manifest: %v", err)
		result.FixHint = platformIntegrityFixHint
		return result
	}
	report, err := platforminstall.InspectIntegrity(context.Background(), manifest)
	if err != nil {
		result.Status = StatusError
		result.Message = fmt.Sprintf("inspect managed platform: %v", err)
		result.FixHint = platformIntegrityFixHint
		return result
	}
	if len(report.Drifts) == 0 {
		result.Status = StatusOK
		result.Message = fmt.Sprintf("managed platform release %q matches its manifest", manifest.ReleaseID)
		return result
	}
	result.Status = StatusError
	result.Message = fmt.Sprintf("managed platform has %d integrity drift(s)", len(report.Drifts))
	result.Details = make([]string, 0, len(report.Drifts))
	for _, drift := range report.Drifts {
		result.Details = append(result.Details, fmt.Sprintf("%s: expected %s, actual %s", drift.Field, quoteDoctorValue(drift.Expected), quoteDoctorValue(drift.Actual)))
	}
	result.FixHint = platformIntegrityFixHint
	return result
}

const platformIntegrityFixHint = "reconcile drift against the reviewed manifest, then rerun the versioned platform installer or restore the recorded rollback; never edit the receipt by hand"

func quoteDoctorValue(value string) string {
	if strings.ContainsAny(value, " \t\r\n") {
		return fmt.Sprintf("%q", value)
	}
	return value
}

// CanFix returns false; drift must be reconciled through a reviewed manifest.
func (c *PlatformInstallIntegrityCheck) CanFix() bool { return false }

// Fix is a no-op.
func (c *PlatformInstallIntegrityCheck) Fix(_ *CheckContext) error { return nil }

// WarmupEligible returns false; the full fingerprint may execute provider and
// Git probes and is intended for explicit doctor/preflight runs.
func (c *PlatformInstallIntegrityCheck) WarmupEligible() bool { return false }

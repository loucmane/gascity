package doctor

// PlatformInstallIntegrityCheck verifies an opted-in managed platform
// installation against its canonical manifest.
type PlatformInstallIntegrityCheck struct{}

// NewPlatformInstallIntegrityCheck creates the managed platform integrity check.
func NewPlatformInstallIntegrityCheck() *PlatformInstallIntegrityCheck {
	return &PlatformInstallIntegrityCheck{}
}

// Name returns the stable doctor check identifier.
func (c *PlatformInstallIntegrityCheck) Name() string { return "platform-install-integrity" }

// Run remains fail-closed in the RED contract until manifest-backed inspection lands.
func (c *PlatformInstallIntegrityCheck) Run(_ *CheckContext) *CheckResult {
	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: "platform install integrity inspection is disabled",
	}
}

// CanFix returns false; drift must be reconciled through a reviewed manifest.
func (c *PlatformInstallIntegrityCheck) CanFix() bool { return false }

// Fix is a no-op.
func (c *PlatformInstallIntegrityCheck) Fix(_ *CheckContext) error { return nil }

// WarmupEligible returns false; the full fingerprint may execute provider and
// Git probes and is intended for explicit doctor/preflight runs.
func (c *PlatformInstallIntegrityCheck) WarmupEligible() bool { return false }

package managedworker

import (
	"context"

	"github.com/gastownhall/gascity/internal/platforminstall"
)

// PreflightRequest supplies the receipt and live launch facts to verify.
type PreflightRequest struct {
	CheckPath          string
	ObservedProfile    WorkerProfile
	PermissionRevision string
	ProfileName        string
	Receipt            []byte
}

// Probes supplies the side-effecting boundaries used by Preflight.
type Probes struct {
	ReadFile        func(string) ([]byte, error)
	InspectProvider func(context.Context, platforminstall.ProviderPin) error
	ProbeReadiness  func(context.Context, string) error
	ProbeSigner     func(context.Context, string) error
}

// PreflightReport records whether every required managed-worker check passed.
type PreflightReport struct {
	OK bool
}

// Preflight is intentionally skeletal in the RED commit. The focused tests
// require every boundary to be verified before this may remain successful.
func Preflight(context.Context, PreflightRequest, Probes) (PreflightReport, error) {
	return PreflightReport{OK: true}, nil
}

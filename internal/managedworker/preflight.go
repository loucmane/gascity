package managedworker

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gastownhall/gascity/internal/beadmeta"
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
	ReadFile         func(string) ([]byte, error)
	InspectProvider  func(context.Context, platforminstall.ProviderPin) error
	InspectToolchain func(context.Context, ToolchainPin, map[string]string) error
	ProbeReadiness   func(context.Context, string) error
	ProbeSigner      func(context.Context, string) error
}

// PreflightReport records whether every required managed-worker check passed.
type PreflightReport struct {
	Checks []string
	OK     bool
}

// Failure identifies a deterministic managed-worker launch refusal. Callers
// use the typed error to emit an immediate attention event without mistaking it
// for a provider crash.
type Failure struct {
	Profile string
	Err     error
}

func (failure *Failure) Error() string {
	if failure == nil {
		return "managed-worker preflight failed"
	}
	return fmt.Sprintf("managed-worker profile %q: %v", failure.Profile, failure.Err)
}

func (failure *Failure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Err
}

// Preflight verifies every controller-owned launch boundary in deterministic
// order. It returns at the first failure so a caller cannot start the provider
// from a partially verified profile.
func Preflight(ctx context.Context, request PreflightRequest, probes Probes) (PreflightReport, error) {
	receipt, err := LoadProvisioningReceipt(request.Receipt)
	if err != nil {
		return PreflightReport{}, fmt.Errorf("provisioning receipt: %w", err)
	}
	report := PreflightReport{Checks: []string{"receipt"}}

	profile, ok := receipt.Profile(request.ProfileName)
	if !ok {
		return report, fmt.Errorf("managed worker profile %q is not declared in the provisioning receipt", request.ProfileName)
	}
	if err := validateProbes(probes, len(profile.Toolchains) > 0); err != nil {
		return report, err
	}
	report.Checks = append(report.Checks, "profile")
	if request.PermissionRevision != receipt.PermissionRevision {
		return report, fmt.Errorf("permission_revision mismatch: got %q want %q", request.PermissionRevision, receipt.PermissionRevision)
	}
	report.Checks = append(report.Checks, "permission_revision")

	checkPath := filepath.Clean(strings.TrimSpace(request.CheckPath))
	if !filepath.IsAbs(request.CheckPath) || checkPath != request.CheckPath {
		return report, fmt.Errorf("%s must be a clean absolute path: %q", beadmeta.CheckPathMetadataKey, request.CheckPath)
	}
	if checkPath != profile.CheckPath.Path {
		return report, fmt.Errorf("%s mismatch: got %q want %q", beadmeta.CheckPathMetadataKey, checkPath, profile.CheckPath.Path)
	}
	report.Checks = append(report.Checks, "check_path_stamp")

	observedDigest, err := WorkerProfileDigest(request.ObservedProfile)
	if err != nil {
		return report, fmt.Errorf("observed worker profile: %w", err)
	}
	if !equalDigest(observedDigest, profile.WorkerProfileSHA256) {
		return report, fmt.Errorf("worker_profile_sha256 mismatch: got %q want %q", observedDigest, profile.WorkerProfileSHA256)
	}
	report.Checks = append(report.Checks, "worker_profile")

	if err := inspectFilePin(probes.ReadFile, "rules", receipt.Rules); err != nil {
		return report, err
	}
	report.Checks = append(report.Checks, "rules")
	if err := inspectFilePin(probes.ReadFile, beadmeta.CheckPathMetadataKey, profile.CheckPath); err != nil {
		return report, err
	}
	report.Checks = append(report.Checks, "check_path")

	if err := probes.InspectProvider(ctx, profile.Provider); err != nil {
		return report, fmt.Errorf("provider identity %q: %w", profile.Provider.Name, err)
	}
	report.Checks = append(report.Checks, "provider_identity")
	for _, toolchain := range profile.Toolchains {
		if err := probes.InspectToolchain(ctx, toolchain, profile.Environment); err != nil {
			return report, fmt.Errorf("toolchain identity %q: %w", toolchain.Name, err)
		}
	}
	if len(profile.Toolchains) > 0 {
		report.Checks = append(report.Checks, "toolchains")
	}
	if err := probes.ProbeReadiness(ctx, profile.Provider.Name); err != nil {
		return report, fmt.Errorf("provider readiness %q: %w", profile.Provider.Name, err)
	}
	report.Checks = append(report.Checks, "provider_readiness")
	if err := probes.ProbeSigner(ctx, profile.SignerIdentity); err != nil {
		return report, fmt.Errorf("signer readiness %q: %w", profile.SignerIdentity, err)
	}
	report.Checks = append(report.Checks, "signer")
	report.OK = true
	return report, nil
}

func validateProbes(probes Probes, requireToolchains bool) error {
	required := map[string]bool{
		"read file":          probes.ReadFile != nil,
		"inspect provider":   probes.InspectProvider != nil,
		"provider readiness": probes.ProbeReadiness != nil,
		"signer readiness":   probes.ProbeSigner != nil,
	}
	if requireToolchains {
		required["inspect toolchain"] = probes.InspectToolchain != nil
	}
	for name, present := range required {
		if !present {
			return fmt.Errorf("managed-worker preflight probe %q is required", name)
		}
	}
	return nil
}

func inspectFilePin(readFile func(string) ([]byte, error), name string, pin FilePin) error {
	data, err := readFile(pin.Path)
	if err != nil {
		return fmt.Errorf("%s %q: %w", name, pin.Path, err)
	}
	got := sha256.Sum256(data)
	gotDigest := hex.EncodeToString(got[:])
	if !equalDigest(gotDigest, pin.SHA256) {
		return fmt.Errorf("%s sha256 mismatch: got %q want %q", name, gotDigest, pin.SHA256)
	}
	return nil
}

func equalDigest(left, right string) bool {
	leftBytes, leftErr := hex.DecodeString(left)
	rightBytes, rightErr := hex.DecodeString(right)
	if leftErr != nil || rightErr != nil || len(leftBytes) != sha256.Size || len(rightBytes) != sha256.Size {
		return false
	}
	return subtle.ConstantTimeCompare(leftBytes, rightBytes) == 1
}

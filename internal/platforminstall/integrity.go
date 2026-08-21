package platforminstall

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
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

// InspectIntegrity compares the live filesystem and tools with the manifest.
func InspectIntegrity(ctx context.Context, manifest Manifest) (IntegrityReport, error) {
	if err := validateManifest(manifest); err != nil {
		return IntegrityReport{}, err
	}
	wantManifestDigest, err := ManifestDigest(manifest)
	if err != nil {
		return IntegrityReport{}, err
	}
	if manifest.ManifestSHA256 != wantManifestDigest {
		return IntegrityReport{}, fmt.Errorf("manifest_sha256 mismatch: got %q want %q", manifest.ManifestSHA256, wantManifestDigest)
	}

	report := IntegrityReport{}
	inspectRegularFile(&report, "core", manifest.Core.Destination, manifest.Core.SHA256, manifest.Core.Mode)
	inspectRegularFile(&report, "backup", manifest.BackupPath, manifest.PreviousSHA256, manifest.Core.Mode)
	for _, file := range manifest.ManagedFiles {
		field := "managed_files[" + file.Name + "]"
		inspectRegularFile(&report, field, file.Destination, file.SHA256, file.Mode)
		if file.PreviousSHA256 != "" {
			inspectRegularFile(&report, field+".backup", file.BackupPath, file.PreviousSHA256, file.Mode)
		}
	}
	inspectReceipt(&report, manifest)
	pinned := inspectPinnedIntegrity(ctx, manifest)
	report.Drifts = append(report.Drifts, pinned.Drifts...)
	sort.Slice(report.Drifts, func(i, j int) bool {
		return report.Drifts[i].Field < report.Drifts[j].Field
	})
	return report, nil
}

func inspectPinnedIntegrity(ctx context.Context, manifest Manifest) IntegrityReport {
	report := IntegrityReport{}
	if manifest.Integrity == nil {
		return report
	}
	for _, pin := range manifest.Integrity.Files {
		inspectRegularFile(&report, "files["+pin.Name+"]", pin.Path, pin.SHA256, pin.Mode)
	}
	for _, pin := range manifest.Integrity.Repositories {
		inspectRepository(ctx, &report, pin)
	}
	for _, pin := range manifest.Integrity.Providers {
		inspectProvider(ctx, &report, pin)
	}
	sort.Slice(report.Drifts, func(i, j int) bool {
		return report.Drifts[i].Field < report.Drifts[j].Field
	})
	return report
}

func validateIntegritySpec(spec *IntegritySpec) error {
	if spec == nil {
		return nil
	}
	fileNames := make(map[string]struct{}, len(spec.Files))
	for _, pin := range spec.Files {
		field := "integrity.files[" + pin.Name + "]"
		if err := validatePinName("integrity file", pin.Name, fileNames); err != nil {
			return err
		}
		if !filepath.IsAbs(pin.Path) {
			return fmt.Errorf("%s.path must be an absolute path: %q", field, pin.Path)
		}
		if err := validateSHA256(field+".sha256", pin.SHA256); err != nil {
			return err
		}
		if err := validatePinnedMode(field+".mode", pin.Mode); err != nil {
			return err
		}
	}

	repositoryNames := make(map[string]struct{}, len(spec.Repositories))
	for _, pin := range spec.Repositories {
		field := "integrity.repositories[" + pin.Name + "]"
		if err := validatePinName("integrity repository", pin.Name, repositoryNames); err != nil {
			return err
		}
		if !filepath.IsAbs(pin.Path) {
			return fmt.Errorf("%s.path must be an absolute path: %q", field, pin.Path)
		}
		if err := validateGitCommit(field+".commit", pin.Commit); err != nil {
			return err
		}
	}

	providerNames := make(map[string]struct{}, len(spec.Providers))
	for _, pin := range spec.Providers {
		field := "integrity.providers[" + pin.Name + "]"
		if err := validatePinName("integrity provider", pin.Name, providerNames); err != nil {
			return err
		}
		if !filepath.IsAbs(pin.Path) {
			return fmt.Errorf("%s.path must be an absolute path: %q", field, pin.Path)
		}
		if !filepath.IsAbs(pin.ResolvedPath) {
			return fmt.Errorf("%s.resolved_path must be an absolute path: %q", field, pin.ResolvedPath)
		}
		if err := validateSHA256(field+".sha256", pin.SHA256); err != nil {
			return err
		}
		if strings.TrimSpace(pin.Version) == "" {
			return fmt.Errorf("%s.version is required", field)
		}
		for index, arg := range pin.VersionArgs {
			if strings.TrimSpace(arg) == "" {
				return fmt.Errorf("%s.version_args[%d] must not be empty", field, index)
			}
		}
	}
	return nil
}

func validatePinName(kind, name string, seen map[string]struct{}) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%s name is required", kind)
	}
	if _, exists := seen[name]; exists {
		return fmt.Errorf("duplicate %s name %q", kind, name)
	}
	seen[name] = struct{}{}
	return nil
}

func validatePinnedMode(field string, mode uint32) error {
	if mode == 0 || mode&^0o777 != 0 {
		return fmt.Errorf("%s must contain permission bits only and must not be zero", field)
	}
	return nil
}

func validateGitCommit(field, value string) error {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != 20 || value != strings.ToLower(value) {
		return fmt.Errorf("manifest %s must be a full lowercase Git commit", field)
	}
	return nil
}

func inspectRegularFile(report *IntegrityReport, field, path, wantSHA string, wantMode uint32) {
	info, err := os.Lstat(path)
	if err != nil {
		report.add(field+".sha256", wantSHA, describePathError(err))
		return
	}
	if !info.Mode().IsRegular() {
		report.add(field+".sha256", wantSHA, "unsafe mode "+info.Mode().String())
		return
	}
	gotSHA, err := digestRegularFile(path)
	if err != nil {
		report.add(field+".sha256", wantSHA, "error: "+err.Error())
	} else if gotSHA != wantSHA {
		report.add(field+".sha256", wantSHA, gotSHA)
	}
	gotMode := uint32(info.Mode().Perm())
	if gotMode != wantMode {
		report.add(field+".mode", fmt.Sprintf("%#o", wantMode), fmt.Sprintf("%#o", gotMode))
	}
}

func inspectReceipt(report *IntegrityReport, manifest Manifest) {
	receipt, err := loadReceipt(manifest.ReceiptPath)
	if err != nil {
		report.add("receipt.self_digest", "valid", "error: "+err.Error())
		return
	}
	for _, comparison := range []struct {
		field    string
		expected string
		actual   string
	}{
		{"receipt.release_id", manifest.ReleaseID, receipt.ReleaseID},
		{"receipt.manifest_sha256", manifest.ManifestSHA256, receipt.ManifestSHA256},
		{"receipt.artifact_sha256", manifest.Core.SHA256, receipt.ArtifactSHA256},
		{"receipt.previous_sha256", manifest.PreviousSHA256, receipt.PreviousSHA256},
	} {
		if comparison.actual != comparison.expected {
			report.add(comparison.field, comparison.expected, comparison.actual)
		}
	}
	wantManaged := receiptManagedFiles(manifest.ManagedFiles)
	if len(receipt.ManagedFiles) != len(wantManaged) {
		report.add("receipt.managed_files", fmt.Sprintf("%d entries", len(wantManaged)), fmt.Sprintf("%d entries", len(receipt.ManagedFiles)))
		return
	}
	for index := range wantManaged {
		if receipt.ManagedFiles[index] != wantManaged[index] {
			report.add("receipt.managed_files", fmt.Sprintf("%+v", wantManaged), fmt.Sprintf("%+v", receipt.ManagedFiles))
			return
		}
	}
	if manifest.Activation == nil {
		if receipt.Activation != nil {
			report.add("receipt.activation", "absent", "present")
		}
		return
	}
	if receipt.Activation == nil {
		report.add("receipt.activation", "verified", "missing")
		return
	}
	for _, comparison := range []struct {
		field    string
		expected string
		actual   string
	}{
		{"receipt.activation.executable_sha256", manifest.Core.SHA256, receipt.Activation.ExecutableSHA256},
		{"receipt.activation.commit", manifest.Activation.ExpectedCommit, receipt.Activation.Commit},
		{"receipt.activation.version", manifest.Activation.ExpectedVersion, receipt.Activation.Version},
	} {
		if comparison.actual != comparison.expected {
			report.add(comparison.field, comparison.expected, comparison.actual)
		}
	}
}

func inspectRepository(ctx context.Context, report *IntegrityReport, pin GitPin) {
	field := "repositories[" + pin.Name + "]"
	head, err := runInspectionCommand(ctx, "git", "-C", pin.Path, "rev-parse", "HEAD")
	if err != nil {
		report.add(field+".commit", pin.Commit, "error: "+err.Error())
		return
	}
	if head != pin.Commit {
		report.add(field+".commit", pin.Commit, head)
	}
	if pin.AllowDirty {
		return
	}
	status, err := runInspectionCommand(ctx, "git", "-C", pin.Path, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		report.add(field+".clean", "true", "error: "+err.Error())
	} else if status != "" {
		report.add(field+".clean", "true", "false")
	}
}

func inspectProvider(ctx context.Context, report *IntegrityReport, pin ProviderPin) {
	field := "providers[" + pin.Name + "]"
	resolved, err := filepath.EvalSymlinks(pin.Path)
	if err != nil {
		report.add(field+".resolved_path", filepath.Clean(pin.ResolvedPath), describePathError(err))
		return
	}
	resolved = filepath.Clean(resolved)
	wantResolved := filepath.Clean(pin.ResolvedPath)
	if resolved != wantResolved {
		report.add(field+".resolved_path", wantResolved, resolved)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		report.add(field+".sha256", pin.SHA256, describePathError(err))
		return
	}
	if !info.Mode().IsRegular() {
		report.add(field+".sha256", pin.SHA256, "unsafe mode "+info.Mode().String())
		return
	}
	if info.Mode().Perm()&0o111 == 0 {
		report.add(field+".executable", "true", "false")
	}
	gotSHA, err := digestRegularFile(resolved)
	if err != nil {
		report.add(field+".sha256", pin.SHA256, "error: "+err.Error())
	} else if gotSHA != pin.SHA256 {
		report.add(field+".sha256", pin.SHA256, gotSHA)
	}
	version, err := runInspectionCommand(ctx, pin.Path, pin.VersionArgs...)
	if err != nil {
		report.add(field+".version", pin.Version, "error: "+err.Error())
	} else if version != pin.Version {
		report.add(field+".version", pin.Version, version)
	}
}

func runInspectionCommand(ctx context.Context, name string, args ...string) (string, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, name, args...).CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if commandCtx.Err() != nil {
			return "", commandCtx.Err()
		}
		if text != "" {
			return "", fmt.Errorf("%w: %s", err, text)
		}
		return "", err
	}
	return text, nil
}

func describePathError(err error) string {
	if os.IsNotExist(err) {
		return "missing"
	}
	return "error: " + err.Error()
}

func (report *IntegrityReport) add(field, expected, actual string) {
	report.Drifts = append(report.Drifts, Drift{Field: field, Expected: expected, Actual: actual})
}

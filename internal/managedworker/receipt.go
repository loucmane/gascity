// Package managedworker verifies the controller-owned inputs required before
// a managed worker may start.
package managedworker

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gastownhall/gascity/internal/platforminstall"
)

const (
	// ProvisioningReceiptSchemaV1 is the first managed-worker provisioning receipt schema.
	ProvisioningReceiptSchemaV1 = "gc.provisioning-receipt.v1"
	// ProvisioningReceiptSchemaV2 binds the managed launch environment and its
	// executable toolchains in addition to the v1 controls.
	ProvisioningReceiptSchemaV2 = "gc.provisioning-receipt.v2"
	// ProvisioningReceiptRelativePath is the controller-owned authority consumed
	// immediately before a managed provider starts.
	ProvisioningReceiptRelativePath = ".gc/runtime/provisioning/receipt.json"
)

// MemberHead binds one provisioning-convoy member to its reviewed commit.
type MemberHead struct {
	Name   string `json:"name"`
	Commit string `json:"commit"`
}

// PackPin binds the imported worker pack to its exact source, commit, and bytes.
type PackPin struct {
	Commit string `json:"commit"`
	SHA256 string `json:"sha256"`
	Source string `json:"source"`
}

// FilePin binds one required regular file to its absolute path and bytes.
type FilePin struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// ExecutablePin binds both the requested and resolved executable paths, exact
// bytes, and the version output observed during provisioning.
type ExecutablePin struct {
	Path         string   `json:"path"`
	ResolvedPath string   `json:"resolved_path"`
	SHA256       string   `json:"sha256"`
	VersionArgs  []string `json:"version_args"`
	Version      string   `json:"version"`
}

// ToolchainPin binds one named build toolchain used by the managed worker.
type ToolchainPin struct {
	Executable ExecutablePin `json:"executable"`
	Name       string        `json:"name"`
}

// WorkerProfile is the complete launch-control fingerprint for one managed worker.
type WorkerProfile struct {
	ApprovalPolicy      string                      `json:"approval_policy"`
	Argv                []string                    `json:"argv"`
	CheckPath           FilePin                     `json:"check_path"`
	Environment         map[string]string           `json:"environment,omitempty"`
	Name                string                      `json:"name"`
	NetworkPolicy       string                      `json:"network_policy"`
	Provider            platforminstall.ProviderPin `json:"provider"`
	SandboxMode         string                      `json:"sandbox_mode"`
	SignerIdentity      string                      `json:"signer_identity"`
	Toolchains          []ToolchainPin              `json:"toolchains,omitempty"`
	WorkerProfileSHA256 string                      `json:"worker_profile_sha256,omitempty"`
	WritableRoots       []string                    `json:"writable_roots"`
}

// ProvisioningReceipt binds reviewed provisioning inputs and worker profiles.
type ProvisioningReceipt struct {
	CanaryRunner       FilePin         `json:"canary_runner"`
	MemberHeads        []MemberHead    `json:"member_heads"`
	Pack               PackPin         `json:"pack"`
	PermissionRevision string          `json:"permission_revision"`
	Profiles           []WorkerProfile `json:"profiles"`
	ReceiptSHA256      string          `json:"receipt_sha256,omitempty"`
	Rules              FilePin         `json:"rules"`
	Schema             string          `json:"schema"`
	TemplateCommit     string          `json:"template_commit"`
}

// ProvisioningReceiptPath returns the canonical receipt location for a city.
func ProvisioningReceiptPath(cityPath string) string {
	return filepath.Join(cityPath, filepath.FromSlash(ProvisioningReceiptRelativePath))
}

// Profile returns the exact named managed-worker profile from a verified receipt.
func (receipt ProvisioningReceipt) Profile(name string) (WorkerProfile, bool) {
	name = strings.TrimSpace(name)
	for _, profile := range receipt.Profiles {
		if profile.Name == name {
			return profile, true
		}
	}
	return WorkerProfile{}, false
}

// FinalizeProvisioningReceipt validates, canonicalizes, and self-digests a receipt.
func FinalizeProvisioningReceipt(receipt ProvisioningReceipt) (ProvisioningReceipt, []byte, error) {
	if receipt.ReceiptSHA256 != "" {
		return ProvisioningReceipt{}, nil, errors.New("receipt_sha256 must be empty before finalization")
	}
	receipt = canonicalizeReceiptSlices(receipt)
	for index := range receipt.Profiles {
		if receipt.Profiles[index].WorkerProfileSHA256 != "" {
			return ProvisioningReceipt{}, nil, fmt.Errorf("profiles[%d].worker_profile_sha256 must be empty before finalization", index)
		}
		digest, err := WorkerProfileDigest(receipt.Profiles[index])
		if err != nil {
			return ProvisioningReceipt{}, nil, fmt.Errorf("profiles[%d]: %w", index, err)
		}
		receipt.Profiles[index].WorkerProfileSHA256 = digest
	}
	if err := validateReceiptContent(receipt); err != nil {
		return ProvisioningReceipt{}, nil, err
	}
	digest, err := provisioningReceiptDigest(receipt)
	if err != nil {
		return ProvisioningReceipt{}, nil, err
	}
	receipt.ReceiptSHA256 = digest
	encoded, err := canonicalJSON(receipt)
	if err != nil {
		return ProvisioningReceipt{}, nil, fmt.Errorf("marshal finalized provisioning receipt: %w", err)
	}
	return receipt, encoded, nil
}

// LoadProvisioningReceipt strictly decodes and verifies a finalized receipt.
func LoadProvisioningReceipt(data []byte) (ProvisioningReceipt, error) {
	var receipt ProvisioningReceipt
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return ProvisioningReceipt{}, fmt.Errorf("decode provisioning receipt: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return ProvisioningReceipt{}, err
	}
	if err := validateReceiptContent(receipt); err != nil {
		return ProvisioningReceipt{}, err
	}
	if err := validateSHA256("receipt_sha256", receipt.ReceiptSHA256); err != nil {
		return ProvisioningReceipt{}, err
	}
	want, err := provisioningReceiptDigest(receipt)
	if err != nil {
		return ProvisioningReceipt{}, err
	}
	if subtle.ConstantTimeCompare([]byte(receipt.ReceiptSHA256), []byte(want)) != 1 {
		return ProvisioningReceipt{}, fmt.Errorf("receipt_sha256 mismatch: got %q want %q", receipt.ReceiptSHA256, want)
	}
	return receipt, nil
}

// WorkerProfileDigest hashes every launch-control field in one profile.
func WorkerProfileDigest(profile WorkerProfile) (string, error) {
	profile.WorkerProfileSHA256 = ""
	if err := validateWorkerProfile(profile, false, len(profile.Environment) > 0 || len(profile.Toolchains) > 0); err != nil {
		return "", err
	}
	encoded, err := canonicalJSON(profile)
	if err != nil {
		return "", fmt.Errorf("marshal worker profile digest domain: %w", err)
	}
	return sha256Hex(encoded), nil
}

func provisioningReceiptDigest(receipt ProvisioningReceipt) (string, error) {
	receipt.ReceiptSHA256 = ""
	encoded, err := canonicalJSON(receipt)
	if err != nil {
		return "", fmt.Errorf("marshal provisioning receipt digest domain: %w", err)
	}
	return sha256Hex(encoded), nil
}

func canonicalizeReceiptSlices(receipt ProvisioningReceipt) ProvisioningReceipt {
	receipt.MemberHeads = append([]MemberHead(nil), receipt.MemberHeads...)
	sort.Slice(receipt.MemberHeads, func(i, j int) bool { return receipt.MemberHeads[i].Name < receipt.MemberHeads[j].Name })
	receipt.Profiles = append([]WorkerProfile(nil), receipt.Profiles...)
	for index := range receipt.Profiles {
		receipt.Profiles[index].Argv = append([]string(nil), receipt.Profiles[index].Argv...)
		receipt.Profiles[index].Environment = cloneStringMap(receipt.Profiles[index].Environment)
		receipt.Profiles[index].Toolchains = append([]ToolchainPin(nil), receipt.Profiles[index].Toolchains...)
		for toolchainIndex := range receipt.Profiles[index].Toolchains {
			receipt.Profiles[index].Toolchains[toolchainIndex].Executable.VersionArgs = append(
				[]string(nil), receipt.Profiles[index].Toolchains[toolchainIndex].Executable.VersionArgs...,
			)
		}
		sort.Slice(receipt.Profiles[index].Toolchains, func(left, right int) bool {
			return receipt.Profiles[index].Toolchains[left].Name < receipt.Profiles[index].Toolchains[right].Name
		})
		receipt.Profiles[index].WritableRoots = append([]string(nil), receipt.Profiles[index].WritableRoots...)
		sort.Strings(receipt.Profiles[index].WritableRoots)
	}
	sort.Slice(receipt.Profiles, func(i, j int) bool { return receipt.Profiles[i].Name < receipt.Profiles[j].Name })
	return receipt
}

func validateReceiptContent(receipt ProvisioningReceipt) error {
	if receipt.Schema != ProvisioningReceiptSchemaV1 && receipt.Schema != ProvisioningReceiptSchemaV2 {
		return fmt.Errorf("provisioning receipt schema %q is unsupported", receipt.Schema)
	}
	if len(receipt.MemberHeads) == 0 {
		return errors.New("member_heads must not be empty")
	}
	if err := validateFilePin("canary_runner", receipt.CanaryRunner); err != nil {
		return err
	}
	last := ""
	for index, head := range receipt.MemberHeads {
		if strings.TrimSpace(head.Name) == "" {
			return fmt.Errorf("member_heads[%d].name is required", index)
		}
		if head.Name <= last {
			return errors.New("member_heads must be sorted by unique name")
		}
		last = head.Name
		if err := validateGitCommit(fmt.Sprintf("member_heads[%d].commit", index), head.Commit); err != nil {
			return err
		}
	}
	if err := validateGitCommit("template_commit", receipt.TemplateCommit); err != nil {
		return err
	}
	if strings.TrimSpace(receipt.Pack.Source) == "" {
		return errors.New("pack.source is required")
	}
	if err := validateGitCommit("pack.commit", receipt.Pack.Commit); err != nil {
		return err
	}
	if err := validateSHA256("pack.sha256", receipt.Pack.SHA256); err != nil {
		return err
	}
	if err := validateFilePin("rules", receipt.Rules); err != nil {
		return err
	}
	if err := validateSHA256("permission_revision", receipt.PermissionRevision); err != nil {
		return err
	}
	if len(receipt.Profiles) == 0 {
		return errors.New("profiles must not be empty")
	}
	last = ""
	for index, profile := range receipt.Profiles {
		if profile.Name <= last {
			return errors.New("profiles must be sorted by unique name")
		}
		last = profile.Name
		requireRuntime := receipt.Schema == ProvisioningReceiptSchemaV2
		if err := validateWorkerProfile(profile, true, requireRuntime); err != nil {
			return fmt.Errorf("profiles[%d]: %w", index, err)
		}
		want, err := WorkerProfileDigest(profile)
		if err != nil {
			return fmt.Errorf("profiles[%d]: %w", index, err)
		}
		if subtle.ConstantTimeCompare([]byte(profile.WorkerProfileSHA256), []byte(want)) != 1 {
			return fmt.Errorf("profiles[%d].worker_profile_sha256 mismatch: got %q want %q", index, profile.WorkerProfileSHA256, want)
		}
	}
	return nil
}

func validateWorkerProfile(profile WorkerProfile, requireDigest, requireRuntime bool) error {
	for name, value := range map[string]string{
		"name": profile.Name, "approval_policy": profile.ApprovalPolicy,
		"sandbox_mode": profile.SandboxMode, "network_policy": profile.NetworkPolicy,
		"signer_identity": profile.SignerIdentity,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if len(profile.Argv) == 0 {
		return errors.New("argv must not be empty")
	}
	for index, arg := range profile.Argv {
		if strings.TrimSpace(arg) == "" {
			return fmt.Errorf("argv[%d] must not be empty", index)
		}
	}
	lastRoot := ""
	for index, root := range profile.WritableRoots {
		if !filepath.IsAbs(root) || filepath.Clean(root) != root {
			return fmt.Errorf("writable_roots[%d] must be a clean absolute path: %q", index, root)
		}
		if root <= lastRoot {
			return errors.New("writable_roots must be sorted and unique")
		}
		lastRoot = root
	}
	if err := validateFilePin("check_path", profile.CheckPath); err != nil {
		return err
	}
	if err := validateProviderPin(profile.Provider); err != nil {
		return err
	}
	if requireRuntime && len(profile.Environment) == 0 {
		return errors.New("environment must not be empty in a v2 profile")
	}
	if requireRuntime && len(profile.Toolchains) == 0 {
		return errors.New("toolchains must not be empty in a v2 profile")
	}
	if !requireRuntime && (len(profile.Environment) > 0 || len(profile.Toolchains) > 0) {
		return errors.New("environment and toolchains require provisioning receipt v2")
	}
	for key, value := range profile.Environment {
		if !validEnvironmentName(key) {
			return fmt.Errorf("environment key %q is invalid", key)
		}
		if value == "" || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("environment[%q] must be non-empty and contain no NUL", key)
		}
	}
	lastToolchain := ""
	for index, toolchain := range profile.Toolchains {
		if strings.TrimSpace(toolchain.Name) == "" {
			return fmt.Errorf("toolchains[%d].name is required", index)
		}
		if toolchain.Name <= lastToolchain {
			return errors.New("toolchains must be sorted by unique name")
		}
		lastToolchain = toolchain.Name
		if err := validateExecutablePin(fmt.Sprintf("toolchains[%d].executable", index), toolchain.Executable); err != nil {
			return err
		}
	}
	if requireDigest {
		return validateSHA256("worker_profile_sha256", profile.WorkerProfileSHA256)
	}
	return nil
}

func validateExecutablePin(name string, pin ExecutablePin) error {
	for field, path := range map[string]string{"path": pin.Path, "resolved_path": pin.ResolvedPath} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fmt.Errorf("%s.%s must be a clean absolute path: %q", name, field, path)
		}
	}
	if err := validateSHA256(name+".sha256", pin.SHA256); err != nil {
		return err
	}
	if strings.TrimSpace(pin.Version) == "" {
		return fmt.Errorf("%s.version is required", name)
	}
	for index, argument := range pin.VersionArgs {
		if strings.TrimSpace(argument) == "" {
			return fmt.Errorf("%s.version_args[%d] must not be empty", name, index)
		}
	}
	return nil
}

func validEnvironmentName(name string) bool {
	if name == "" || !validEnvironmentNameCharacter(name[0], false) {
		return false
	}
	for index := 1; index < len(name); index++ {
		if !validEnvironmentNameCharacter(name[index], true) {
			return false
		}
	}
	return true
}

func validEnvironmentNameCharacter(character byte, allowDigit bool) bool {
	switch {
	case character == '_':
		return true
	case character >= 'A' && character <= 'Z':
		return true
	case character >= 'a' && character <= 'z':
		return true
	case allowDigit && character >= '0' && character <= '9':
		return true
	default:
		return false
	}
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func validateProviderPin(pin platforminstall.ProviderPin) error {
	if strings.TrimSpace(pin.Name) == "" {
		return errors.New("provider.name is required")
	}
	for name, path := range map[string]string{"provider.path": pin.Path, "provider.resolved_path": pin.ResolvedPath} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fmt.Errorf("%s must be a clean absolute path: %q", name, path)
		}
	}
	if err := validateSHA256("provider.sha256", pin.SHA256); err != nil {
		return err
	}
	if strings.TrimSpace(pin.Version) == "" {
		return errors.New("provider.version is required")
	}
	for index, arg := range pin.VersionArgs {
		if strings.TrimSpace(arg) == "" {
			return fmt.Errorf("provider.version_args[%d] must not be empty", index)
		}
	}
	return nil
}

func validateFilePin(name string, pin FilePin) error {
	if !filepath.IsAbs(pin.Path) || filepath.Clean(pin.Path) != pin.Path {
		return fmt.Errorf("%s.path must be a clean absolute path: %q", name, pin.Path)
	}
	return validateSHA256(name+".sha256", pin.SHA256)
}

func validateGitCommit(name, value string) error {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != 20 || value != strings.ToLower(value) {
		return fmt.Errorf("%s must be a full lowercase Git commit", name)
	}
	return nil
}

func validateSHA256(name, value string) error {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size || value != strings.ToLower(value) {
		return fmt.Errorf("%s must be a lowercase SHA-256", name)
	}
	return nil
}

func canonicalJSON(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var document any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	return json.Marshal(document)
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode provisioning receipt: trailing JSON value")
		}
		return fmt.Errorf("decode provisioning receipt trailer: %w", err)
	}
	return nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

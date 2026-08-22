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

// ProvisioningReceiptSchemaV1 is the first managed-worker provisioning receipt schema.
const ProvisioningReceiptSchemaV1 = "gc.provisioning-receipt.v1"

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

// WorkerProfile is the complete launch-control fingerprint for one managed worker.
type WorkerProfile struct {
	ApprovalPolicy      string                      `json:"approval_policy"`
	Argv                []string                    `json:"argv"`
	CheckPath           FilePin                     `json:"check_path"`
	Name                string                      `json:"name"`
	NetworkPolicy       string                      `json:"network_policy"`
	Provider            platforminstall.ProviderPin `json:"provider"`
	SandboxMode         string                      `json:"sandbox_mode"`
	SignerIdentity      string                      `json:"signer_identity"`
	WorkerProfileSHA256 string                      `json:"worker_profile_sha256,omitempty"`
	WritableRoots       []string                    `json:"writable_roots"`
}

// ProvisioningReceipt binds reviewed provisioning inputs and worker profiles.
type ProvisioningReceipt struct {
	MemberHeads        []MemberHead    `json:"member_heads"`
	Pack               PackPin         `json:"pack"`
	PermissionRevision string          `json:"permission_revision"`
	Profiles           []WorkerProfile `json:"profiles"`
	ReceiptSHA256      string          `json:"receipt_sha256,omitempty"`
	Rules              FilePin         `json:"rules"`
	Schema             string          `json:"schema"`
	TemplateCommit     string          `json:"template_commit"`
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
	if err := validateWorkerProfile(profile, false); err != nil {
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
		receipt.Profiles[index].WritableRoots = append([]string(nil), receipt.Profiles[index].WritableRoots...)
		sort.Strings(receipt.Profiles[index].WritableRoots)
	}
	sort.Slice(receipt.Profiles, func(i, j int) bool { return receipt.Profiles[i].Name < receipt.Profiles[j].Name })
	return receipt
}

func validateReceiptContent(receipt ProvisioningReceipt) error {
	if receipt.Schema != ProvisioningReceiptSchemaV1 {
		return fmt.Errorf("provisioning receipt schema %q is unsupported", receipt.Schema)
	}
	if len(receipt.MemberHeads) == 0 {
		return errors.New("member_heads must not be empty")
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
		if err := validateWorkerProfile(profile, true); err != nil {
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

func validateWorkerProfile(profile WorkerProfile, requireDigest bool) error {
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
	if requireDigest {
		return validateSHA256("worker_profile_sha256", profile.WorkerProfileSHA256)
	}
	return nil
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

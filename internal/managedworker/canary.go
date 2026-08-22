package managedworker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/platforminstall"
)

const (
	// CanaryReceiptSchemaV1 is the first managed-product canary receipt schema.
	CanaryReceiptSchemaV1 = "gc.canary-receipt.v1"
	// CanaryReceiptRelativePath is the controller-owned current receipt path.
	CanaryReceiptRelativePath = ".gc/runtime/canary/receipt.json"
	// CanaryHistoryRelativeDir is the immutable, digest-addressed receipt archive.
	CanaryHistoryRelativeDir = ".gc/runtime/canary/history"
	// CanaryResultPass is the only result accepted by a dispatch gate.
	CanaryResultPass = "pass"
	// DispatchRefusalCode is the stable machine-readable refusal code.
	DispatchRefusalCode = "managed_product_dispatch_refused"
)

// BinaryPin binds the running gc executable to reviewed source and bytes.
type BinaryPin struct {
	Commit string `json:"commit"`
	SHA256 string `json:"sha256"`
}

// ProfilePin binds one fully resolved worker profile to its digest.
type ProfilePin struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

// CanaryEnvironment is the exact live fingerprint covered by a canary run.
type CanaryEnvironment struct {
	GCBinary                  BinaryPin                     `json:"gc_binary"`
	Pack                      PackPin                       `json:"pack"`
	PermissionRevision        string                        `json:"permission_revision"`
	Profiles                  []ProfilePin                  `json:"profiles"`
	Providers                 []platforminstall.ProviderPin `json:"providers"`
	ProvisioningReceiptSHA256 string                        `json:"provisioning_receipt_sha256"`
	Rules                     FilePin                       `json:"rules"`
	TemplateCommit            string                        `json:"template_commit"`
}

// CanaryScenario records one required clean-run or fault-injection outcome.
type CanaryScenario struct {
	AttentionLatencyCycles int    `json:"attention_latency_cycles"`
	Name                   string `json:"name"`
	Outcome                string `json:"outcome"`
}

// CanaryReceipt is the controller-owned proof consumed before product dispatch.
type CanaryReceipt struct {
	CanaryRunID         string              `json:"canary_run_id"`
	Environment         CanaryEnvironment   `json:"environment"`
	IssuedAt            string              `json:"issued_at"`
	ProvisioningReceipt ProvisioningReceipt `json:"provisioning_receipt"`
	ReceiptSHA256       string              `json:"receipt_sha256,omitempty"`
	Result              string              `json:"result"`
	Runner              FilePin             `json:"runner"`
	Scenarios           []CanaryScenario    `json:"scenarios"`
	Schema              string              `json:"schema"`
}

// DispatchRefusal names the exact stale or missing field that blocked dispatch.
type DispatchRefusal struct {
	Actual   string `json:"actual,omitempty"`
	Code     string `json:"code"`
	Expected string `json:"expected,omitempty"`
	Field    string `json:"field"`
}

func (refusal *DispatchRefusal) Error() string {
	if refusal == nil {
		return DispatchRefusalCode
	}
	return fmt.Sprintf("%s: %s: got %q want %q", refusal.Code, refusal.Field, refusal.Actual, refusal.Expected)
}

// CanaryReceiptPath returns the canonical current receipt path for a city.
func CanaryReceiptPath(cityPath string) string {
	return filepath.Join(cityPath, filepath.FromSlash(CanaryReceiptRelativePath))
}

// CanaryHistoryReceiptPath returns the immutable path for one finalized receipt.
func CanaryHistoryReceiptPath(cityPath, receiptSHA256 string) string {
	return filepath.Join(cityPath, filepath.FromSlash(CanaryHistoryRelativeDir), receiptSHA256+".json")
}

// FinalizeCanaryReceipt validates and self-digests a successful canary receipt.
func FinalizeCanaryReceipt(receipt CanaryReceipt) (CanaryReceipt, []byte, error) {
	if receipt.ReceiptSHA256 != "" {
		return CanaryReceipt{}, nil, errors.New("receipt_sha256 must be empty before finalization")
	}
	receipt = canonicalizeCanaryReceipt(receipt)
	if err := validateCanaryReceiptContent(receipt); err != nil {
		return CanaryReceipt{}, nil, err
	}
	digest, err := canaryReceiptDigest(receipt)
	if err != nil {
		return CanaryReceipt{}, nil, err
	}
	receipt.ReceiptSHA256 = digest
	encoded, err := canonicalJSON(receipt)
	if err != nil {
		return CanaryReceipt{}, nil, fmt.Errorf("marshal finalized canary receipt: %w", err)
	}
	return receipt, encoded, nil
}

// LoadCanaryReceipt strictly decodes and verifies a finalized receipt.
func LoadCanaryReceipt(data []byte) (CanaryReceipt, error) {
	var receipt CanaryReceipt
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return CanaryReceipt{}, fmt.Errorf("decode canary receipt: %w", err)
	}
	var trailer any
	if err := decoder.Decode(&trailer); !errors.Is(err, io.EOF) {
		if err == nil {
			return CanaryReceipt{}, errors.New("decode canary receipt: trailing JSON value")
		}
		return CanaryReceipt{}, fmt.Errorf("decode canary receipt trailer: %w", err)
	}
	if err := validateCanaryReceiptContent(receipt); err != nil {
		return CanaryReceipt{}, err
	}
	if err := validateSHA256("receipt_sha256", receipt.ReceiptSHA256); err != nil {
		return CanaryReceipt{}, err
	}
	want, err := canaryReceiptDigest(receipt)
	if err != nil {
		return CanaryReceipt{}, err
	}
	if !equalDigest(receipt.ReceiptSHA256, want) {
		return CanaryReceipt{}, fmt.Errorf("receipt_sha256 mismatch: got %q want %q", receipt.ReceiptSHA256, want)
	}
	return receipt, nil
}

// VerifyCanaryReceipt requires the receipt fingerprint to equal live state.
func VerifyCanaryReceipt(data []byte, observed CanaryEnvironment) (CanaryReceipt, error) {
	if len(data) == 0 {
		return CanaryReceipt{}, newDispatchRefusal("receipt", "present", "missing")
	}
	receipt, err := LoadCanaryReceipt(data)
	if err != nil {
		return CanaryReceipt{}, newDispatchRefusal("receipt", "valid "+CanaryReceiptSchemaV1, err.Error())
	}
	observed = canonicalizeCanaryEnvironment(observed)
	if err := validateCanaryEnvironment(observed); err != nil {
		return CanaryReceipt{}, newDispatchRefusal("live_fingerprint", "valid", err.Error())
	}
	if refusal := compareCanaryEnvironment(receipt.Environment, observed); refusal != nil {
		return CanaryReceipt{}, refusal
	}
	if receipt.ProvisioningReceipt.ReceiptSHA256 != observed.ProvisioningReceiptSHA256 {
		return CanaryReceipt{}, newDispatchRefusal("provisioning_receipt_sha256", receipt.ProvisioningReceipt.ReceiptSHA256, observed.ProvisioningReceiptSHA256)
	}
	return receipt, nil
}

func canonicalizeCanaryReceipt(receipt CanaryReceipt) CanaryReceipt {
	receipt.Environment = canonicalizeCanaryEnvironment(receipt.Environment)
	receipt.ProvisioningReceipt = canonicalizeReceiptSlices(receipt.ProvisioningReceipt)
	receipt.Scenarios = append([]CanaryScenario(nil), receipt.Scenarios...)
	sort.Slice(receipt.Scenarios, func(i, j int) bool { return receipt.Scenarios[i].Name < receipt.Scenarios[j].Name })
	return receipt
}

func canonicalizeCanaryEnvironment(environment CanaryEnvironment) CanaryEnvironment {
	environment.Providers = append([]platforminstall.ProviderPin(nil), environment.Providers...)
	for index := range environment.Providers {
		environment.Providers[index].VersionArgs = append([]string(nil), environment.Providers[index].VersionArgs...)
	}
	sort.Slice(environment.Providers, func(i, j int) bool { return environment.Providers[i].Name < environment.Providers[j].Name })
	environment.Profiles = append([]ProfilePin(nil), environment.Profiles...)
	sort.Slice(environment.Profiles, func(i, j int) bool { return environment.Profiles[i].Name < environment.Profiles[j].Name })
	return environment
}

func validateCanaryReceiptContent(receipt CanaryReceipt) error {
	if receipt.Schema != CanaryReceiptSchemaV1 {
		return fmt.Errorf("canary receipt schema %q is unsupported", receipt.Schema)
	}
	if strings.TrimSpace(receipt.CanaryRunID) == "" {
		return errors.New("canary_run_id is required")
	}
	if _, err := time.Parse(time.RFC3339, receipt.IssuedAt); err != nil {
		return fmt.Errorf("issued_at must be RFC3339: %w", err)
	}
	if receipt.Result != CanaryResultPass {
		return fmt.Errorf("result must be %q", CanaryResultPass)
	}
	if err := validateFilePin("runner", receipt.Runner); err != nil {
		return err
	}
	if err := validateCanaryEnvironment(receipt.Environment); err != nil {
		return fmt.Errorf("environment: %w", err)
	}
	provisioningBytes, err := canonicalJSON(receipt.ProvisioningReceipt)
	if err != nil {
		return fmt.Errorf("marshal provisioning receipt: %w", err)
	}
	provisioning, err := LoadProvisioningReceipt(provisioningBytes)
	if err != nil {
		return fmt.Errorf("provisioning_receipt: %w", err)
	}
	if provisioning.ReceiptSHA256 != receipt.Environment.ProvisioningReceiptSHA256 {
		return fmt.Errorf("provisioning_receipt_sha256 mismatch: environment %q embedded %q", receipt.Environment.ProvisioningReceiptSHA256, provisioning.ReceiptSHA256)
	}
	if len(receipt.Scenarios) == 0 {
		return errors.New("scenarios must not be empty")
	}
	last := ""
	for index, scenario := range receipt.Scenarios {
		if strings.TrimSpace(scenario.Name) == "" {
			return fmt.Errorf("scenarios[%d].name is required", index)
		}
		if scenario.Name <= last {
			return errors.New("scenarios must be sorted by unique name")
		}
		last = scenario.Name
		if scenario.Outcome != CanaryResultPass {
			return fmt.Errorf("scenarios[%d].outcome must be %q", index, CanaryResultPass)
		}
		if scenario.AttentionLatencyCycles < 0 {
			return fmt.Errorf("scenarios[%d].attention_latency_cycles must not be negative", index)
		}
	}
	return nil
}

func validateCanaryEnvironment(environment CanaryEnvironment) error {
	if err := validateGitCommit("gc_binary.commit", environment.GCBinary.Commit); err != nil {
		return err
	}
	if err := validateSHA256("gc_binary.sha256", environment.GCBinary.SHA256); err != nil {
		return err
	}
	if strings.TrimSpace(environment.Pack.Source) == "" {
		return errors.New("pack.source is required")
	}
	if err := validateGitCommit("pack.commit", environment.Pack.Commit); err != nil {
		return err
	}
	if err := validateSHA256("pack.sha256", environment.Pack.SHA256); err != nil {
		return err
	}
	if err := validateGitCommit("template_commit", environment.TemplateCommit); err != nil {
		return err
	}
	if err := validateFilePin("rules", environment.Rules); err != nil {
		return err
	}
	if err := validateSHA256("permission_revision", environment.PermissionRevision); err != nil {
		return err
	}
	if err := validateSHA256("provisioning_receipt_sha256", environment.ProvisioningReceiptSHA256); err != nil {
		return err
	}
	if len(environment.Providers) == 0 {
		return errors.New("providers must not be empty")
	}
	last := ""
	for index, provider := range environment.Providers {
		if provider.Name <= last {
			return errors.New("providers must be sorted by unique name")
		}
		last = provider.Name
		if err := validateProviderPin(provider); err != nil {
			return fmt.Errorf("providers[%d]: %w", index, err)
		}
	}
	if len(environment.Profiles) == 0 {
		return errors.New("profiles must not be empty")
	}
	last = ""
	for index, profile := range environment.Profiles {
		if strings.TrimSpace(profile.Name) == "" {
			return fmt.Errorf("profiles[%d].name is required", index)
		}
		if profile.Name <= last {
			return errors.New("profiles must be sorted by unique name")
		}
		last = profile.Name
		if err := validateSHA256(fmt.Sprintf("profiles[%d].sha256", index), profile.SHA256); err != nil {
			return err
		}
	}
	return nil
}

func canaryReceiptDigest(receipt CanaryReceipt) (string, error) {
	receipt.ReceiptSHA256 = ""
	encoded, err := canonicalJSON(receipt)
	if err != nil {
		return "", fmt.Errorf("marshal canary receipt digest domain: %w", err)
	}
	return sha256Hex(encoded), nil
}

func compareCanaryEnvironment(expected, actual CanaryEnvironment) *DispatchRefusal {
	for _, comparison := range []struct {
		field, expected, actual string
	}{
		{"gc_binary.commit", expected.GCBinary.Commit, actual.GCBinary.Commit},
		{"gc_binary.sha256", expected.GCBinary.SHA256, actual.GCBinary.SHA256},
		{"pack.source", expected.Pack.Source, actual.Pack.Source},
		{"pack.commit", expected.Pack.Commit, actual.Pack.Commit},
		{"pack.sha256", expected.Pack.SHA256, actual.Pack.SHA256},
		{"template_commit", expected.TemplateCommit, actual.TemplateCommit},
		{"rules.path", expected.Rules.Path, actual.Rules.Path},
		{"rules.sha256", expected.Rules.SHA256, actual.Rules.SHA256},
		{"permission_revision", expected.PermissionRevision, actual.PermissionRevision},
		{"provisioning_receipt_sha256", expected.ProvisioningReceiptSHA256, actual.ProvisioningReceiptSHA256},
	} {
		if comparison.expected != comparison.actual {
			return newDispatchRefusal(comparison.field, comparison.expected, comparison.actual)
		}
	}
	if len(expected.Providers) != len(actual.Providers) {
		return newDispatchRefusal("providers", fmt.Sprint(len(expected.Providers)), fmt.Sprint(len(actual.Providers)))
	}
	for index := range expected.Providers {
		want, got := expected.Providers[index], actual.Providers[index]
		prefix := "providers[" + want.Name + "]."
		for _, comparison := range []struct {
			field, expected, actual string
		}{
			{prefix + "name", want.Name, got.Name},
			{prefix + "path", want.Path, got.Path},
			{prefix + "resolved_path", want.ResolvedPath, got.ResolvedPath},
			{prefix + "sha256", want.SHA256, got.SHA256},
			{prefix + "version", want.Version, got.Version},
			{prefix + "version_args", strings.Join(want.VersionArgs, "\x00"), strings.Join(got.VersionArgs, "\x00")},
		} {
			if comparison.expected != comparison.actual {
				return newDispatchRefusal(comparison.field, comparison.expected, comparison.actual)
			}
		}
	}
	if len(expected.Profiles) != len(actual.Profiles) {
		return newDispatchRefusal("profiles", fmt.Sprint(len(expected.Profiles)), fmt.Sprint(len(actual.Profiles)))
	}
	for index := range expected.Profiles {
		want, got := expected.Profiles[index], actual.Profiles[index]
		if want.Name != got.Name {
			return newDispatchRefusal("profiles["+want.Name+"].name", want.Name, got.Name)
		}
		if want.SHA256 != got.SHA256 {
			return newDispatchRefusal("profiles["+want.Name+"].sha256", want.SHA256, got.SHA256)
		}
	}
	return nil
}

func newDispatchRefusal(field, expected, actual string) *DispatchRefusal {
	return &DispatchRefusal{
		Code:     DispatchRefusalCode,
		Field:    field,
		Expected: expected,
		Actual:   actual,
	}
}

// RefuseDispatch constructs the stable typed error used by composition roots
// when live-fingerprint observation itself cannot be completed. Keeping this
// constructor here ensures CLI, API, and controller callers all expose the
// same machine-readable refusal shape.
func RefuseDispatch(field, expected, actual string) *DispatchRefusal {
	return newDispatchRefusal(field, expected, actual)
}

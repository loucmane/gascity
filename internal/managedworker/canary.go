package managedworker

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/gastownhall/gascity/internal/platforminstall"
)

const (
	// CanaryReceiptSchemaV1 is the first managed-product canary receipt schema.
	CanaryReceiptSchemaV1 = "gc.canary-receipt.v1"
	// CanaryReceiptRelativePath is the controller-owned current receipt path.
	CanaryReceiptRelativePath = ".gc/runtime/canary/receipt.json"
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

// FinalizeCanaryReceipt validates and self-digests a successful canary receipt.
func FinalizeCanaryReceipt(CanaryReceipt) (CanaryReceipt, []byte, error) {
	return CanaryReceipt{}, nil, errors.New("canary receipt finalization not implemented")
}

// LoadCanaryReceipt strictly decodes and verifies a finalized receipt.
func LoadCanaryReceipt([]byte) (CanaryReceipt, error) {
	return CanaryReceipt{}, errors.New("canary receipt loading not implemented")
}

// VerifyCanaryReceipt requires the receipt fingerprint to equal live state.
func VerifyCanaryReceipt(data []byte, _ CanaryEnvironment) (CanaryReceipt, error) {
	if len(data) == 0 {
		return CanaryReceipt{}, &DispatchRefusal{Code: DispatchRefusalCode, Field: "receipt", Expected: "present", Actual: "missing"}
	}
	return CanaryReceipt{}, errors.New("canary receipt verification not implemented")
}

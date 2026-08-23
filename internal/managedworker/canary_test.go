package managedworker

import (
	"errors"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/platforminstall"
)

func TestCanaryReceiptIsStrictSelfDigestedAndPassOnly(t *testing.T) {
	provisioning, _ := finalizedReceipt(t)
	environment := testCanaryEnvironment(provisioning)
	receipt := CanaryReceipt{
		Schema:              CanaryReceiptSchemaV1,
		CanaryRunID:         "canary-20260822-001",
		IssuedAt:            "2026-08-22T08:00:00Z",
		Result:              CanaryResultPass,
		Environment:         environment,
		ProvisioningReceipt: provisioning,
		Runner:              testCanaryRunnerPin(),
		Scenarios: []CanaryScenario{
			{Name: "clean-launcher", Outcome: CanaryResultPass, AttentionLatencyCycles: 0},
			{Name: "missing-provider", Outcome: CanaryResultPass, AttentionLatencyCycles: 1},
		},
	}

	finalized, encoded, err := FinalizeCanaryReceipt(receipt)
	if err != nil {
		t.Fatalf("FinalizeCanaryReceipt: %v", err)
	}
	loaded, err := LoadCanaryReceipt(encoded)
	if err != nil {
		t.Fatalf("LoadCanaryReceipt: %v", err)
	}
	if loaded.ReceiptSHA256 != finalized.ReceiptSHA256 {
		t.Fatalf("receipt digest = %q, want %q", loaded.ReceiptSHA256, finalized.ReceiptSHA256)
	}
	if strings.Contains(string(encoded), "\n") || strings.Contains(string(encoded), ": ") {
		t.Fatalf("canary receipt is not compact canonical JSON: %s", encoded)
	}

	unknown := append([]byte(nil), encoded[:len(encoded)-1]...)
	unknown = append(unknown, []byte(`,"unknown":true}`)...)
	if _, err := LoadCanaryReceipt(unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown-field error = %v", err)
	}

	tampered := strings.Replace(string(encoded), `"permission_revision":"`+environment.PermissionRevision, `"permission_revision":"`+digest("tampered"), 1)
	if _, err := LoadCanaryReceipt([]byte(tampered)); err == nil || !strings.Contains(err.Error(), "receipt_sha256 mismatch") {
		t.Fatalf("tampered receipt error = %v", err)
	}

	receipt.Result = "fail"
	if _, _, err := FinalizeCanaryReceipt(receipt); err == nil || !strings.Contains(err.Error(), "result") {
		t.Fatalf("non-pass finalization error = %v", err)
	}
}

func TestVerifyCanaryReceiptRequiresExactLiveFingerprint(t *testing.T) {
	provisioning, _ := finalizedReceipt(t)
	environment := testCanaryEnvironment(provisioning)
	_, encoded, err := FinalizeCanaryReceipt(CanaryReceipt{
		Schema:              CanaryReceiptSchemaV1,
		CanaryRunID:         "canary-20260822-002",
		IssuedAt:            "2026-08-22T08:00:00Z",
		Result:              CanaryResultPass,
		Environment:         environment,
		ProvisioningReceipt: provisioning,
		Runner:              testCanaryRunnerPin(),
		Scenarios:           []CanaryScenario{{Name: "clean-launcher", Outcome: CanaryResultPass}},
	})
	if err != nil {
		t.Fatalf("FinalizeCanaryReceipt: %v", err)
	}
	if _, err := VerifyCanaryReceipt(encoded, environment); err != nil {
		t.Fatalf("VerifyCanaryReceipt exact match: %v", err)
	}

	tests := map[string]struct {
		field  string
		mutate func(*CanaryEnvironment)
	}{
		"gc commit": {
			field:  "gc_binary.commit",
			mutate: func(got *CanaryEnvironment) { got.GCBinary.Commit = strings.Repeat("e", 40) },
		},
		"gc digest": {
			field:  "gc_binary.sha256",
			mutate: func(got *CanaryEnvironment) { got.GCBinary.SHA256 = digest("other-gc") },
		},
		"pack source": {
			field:  "pack.source",
			mutate: func(got *CanaryEnvironment) { got.Pack.Source = "https://example.invalid/other-pack" },
		},
		"pack commit": {
			field:  "pack.commit",
			mutate: func(got *CanaryEnvironment) { got.Pack.Commit = strings.Repeat("e", 40) },
		},
		"pack digest": {
			field:  "pack.sha256",
			mutate: func(got *CanaryEnvironment) { got.Pack.SHA256 = digest("other-pack") },
		},
		"template commit": {
			field:  "template_commit",
			mutate: func(got *CanaryEnvironment) { got.TemplateCommit = strings.Repeat("e", 40) },
		},
		"provider path": {
			field:  "providers[codex].path",
			mutate: func(got *CanaryEnvironment) { got.Providers[0].Path = "/other/codex" },
		},
		"provider resolved path": {
			field:  "providers[codex].resolved_path",
			mutate: func(got *CanaryEnvironment) { got.Providers[0].ResolvedPath = "/other/resolved/codex" },
		},
		"provider digest": {
			field:  "providers[codex].sha256",
			mutate: func(got *CanaryEnvironment) { got.Providers[0].SHA256 = digest("other-provider") },
		},
		"provider version": {
			field:  "providers[codex].version",
			mutate: func(got *CanaryEnvironment) { got.Providers[0].Version = "codex-cli 999.0.0" },
		},
		"rules digest": {
			field:  "rules.sha256",
			mutate: func(got *CanaryEnvironment) { got.Rules.SHA256 = digest("other-rules") },
		},
		"permission revision": {
			field:  "permission_revision",
			mutate: func(got *CanaryEnvironment) { got.PermissionRevision = digest("other-permission") },
		},
		"worker profile": {
			field:  "profiles[hpfetcher/gc.implementation-worker].sha256",
			mutate: func(got *CanaryEnvironment) { got.Profiles[0].SHA256 = digest("other-profile") },
		},
		"provisioning receipt": {
			field:  "provisioning_receipt_sha256",
			mutate: func(got *CanaryEnvironment) { got.ProvisioningReceiptSHA256 = digest("other-receipt") },
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			observed := cloneCanaryEnvironment(environment)
			test.mutate(&observed)
			_, err := VerifyCanaryReceipt(encoded, observed)
			var refusal *DispatchRefusal
			if !errors.As(err, &refusal) {
				t.Fatalf("VerifyCanaryReceipt error = %T %[1]v, want DispatchRefusal", err)
			}
			if refusal.Code != DispatchRefusalCode || refusal.Field != test.field {
				t.Fatalf("refusal = %+v, want code %q field %q", refusal, DispatchRefusalCode, test.field)
			}
		})
	}
}

func TestVerifyCanaryReceiptRefusesMissingReceiptWithoutOverride(t *testing.T) {
	provisioning, _ := finalizedReceipt(t)
	_, err := VerifyCanaryReceipt(nil, testCanaryEnvironment(provisioning))
	var refusal *DispatchRefusal
	if !errors.As(err, &refusal) {
		t.Fatalf("VerifyCanaryReceipt error = %T %[1]v, want DispatchRefusal", err)
	}
	if refusal.Field != "receipt" {
		t.Fatalf("refusal field = %q, want receipt", refusal.Field)
	}
}

func testCanaryEnvironment(provisioning ProvisioningReceipt) CanaryEnvironment {
	return CanaryEnvironment{
		GCBinary:           BinaryPin{Commit: strings.Repeat("f", 40), SHA256: digest("gc-binary")},
		Pack:               provisioning.Pack,
		TemplateCommit:     provisioning.TemplateCommit,
		Providers:          []platforminstall.ProviderPin{cloneProfile(provisioning.Profiles[0]).Provider},
		Rules:              provisioning.Rules,
		PermissionRevision: provisioning.PermissionRevision,
		Profiles: []ProfilePin{{
			Name:   provisioning.Profiles[0].Name,
			SHA256: provisioning.Profiles[0].WorkerProfileSHA256,
		}},
		ProvisioningReceiptSHA256: provisioning.ReceiptSHA256,
	}
}

func cloneCanaryEnvironment(environment CanaryEnvironment) CanaryEnvironment {
	environment.Providers = append([]platforminstall.ProviderPin(nil), environment.Providers...)
	for index := range environment.Providers {
		environment.Providers[index].VersionArgs = append([]string(nil), environment.Providers[index].VersionArgs...)
	}
	environment.Profiles = append([]ProfilePin(nil), environment.Profiles...)
	return environment
}

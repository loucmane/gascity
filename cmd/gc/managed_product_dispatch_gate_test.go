package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/events"
	"github.com/gastownhall/gascity/internal/managedworker"
	"github.com/gastownhall/gascity/internal/platforminstall"
)

func TestManagedProductDispatchGateIsConfigDrivenAndEmitsImmediateRefusals(t *testing.T) {
	provisioning, canaryData, environment := dispatchGateFixture(t)
	recorder := events.NewFake()
	gate := newManagedProductDispatchGate("/city", &config.City{Rigs: []config.Rig{
		{Name: "product", ManagedProduct: true},
		{Name: "control"},
	}}, provisioning.PermissionRevision, recorder)
	gate.readFile = func(path string) ([]byte, error) {
		if path == managedworker.CanaryReceiptPath("/city") {
			return canaryData, nil
		}
		return nil, os.ErrNotExist
	}
	gate.observe = func(context.Context, string, string) (managedworker.CanaryEnvironment, error) {
		return environment, nil
	}

	if err := gate.Verify("control"); err != nil {
		t.Fatalf("control-plane exemption: %v", err)
	}
	if err := gate.Verify("product"); err != nil {
		t.Fatalf("exact managed-product receipt: %v", err)
	}

	gate.observe = func(context.Context, string, string) (managedworker.CanaryEnvironment, error) {
		stale := environment
		stale.PermissionRevision = dispatchGateDigest("stale")
		return stale, nil
	}
	err := gate.Verify("product")
	var refusal *managedworker.DispatchRefusal
	if !errors.As(err, &refusal) || refusal.Field != "permission_revision" {
		t.Fatalf("stale gate error = %T %[1]v, want permission_revision refusal", err)
	}
	if len(recorder.Events) != 1 || recorder.Events[0].Type != events.ManagedProductDispatchRefused || recorder.Events[0].Subject != "product" {
		t.Fatalf("refusal events = %+v, want one immediate product event", recorder.Events)
	}
}

func TestManagedProductDispatchGateRefusesMissingReceiptBeforeObservation(t *testing.T) {
	recorder := events.NewFake()
	gate := newManagedProductDispatchGate("/city", &config.City{Rigs: []config.Rig{{Name: "product", ManagedProduct: true}}}, dispatchGateDigest("config"), recorder)
	gate.readFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }
	observed := false
	gate.observe = func(context.Context, string, string) (managedworker.CanaryEnvironment, error) {
		observed = true
		return managedworker.CanaryEnvironment{}, nil
	}
	err := gate.Verify("product")
	var refusal *managedworker.DispatchRefusal
	if !errors.As(err, &refusal) || refusal.Field != "receipt" {
		t.Fatalf("missing gate error = %T %[1]v, want receipt refusal", err)
	}
	if observed {
		t.Fatal("live observation ran after missing receipt")
	}
	if len(recorder.Events) != 1 {
		t.Fatalf("events = %d, want one", len(recorder.Events))
	}
}

func dispatchGateFixture(t *testing.T) (managedworker.ProvisioningReceipt, []byte, managedworker.CanaryEnvironment) {
	t.Helper()
	provider := platforminstall.ProviderPin{
		Name: "codex", Path: "/city/bin/codex", ResolvedPath: "/provider/codex",
		SHA256: dispatchGateDigest("provider"), VersionArgs: []string{"--version"}, Version: "codex-cli test",
	}
	profile := managedworker.WorkerProfile{
		Name: "product/gc.implementation-worker", Argv: []string{"/city/bin/codex", "exec"}, WritableRoots: []string{"/worktrees"},
		ApprovalPolicy: "never", SandboxMode: "workspace-write", NetworkPolicy: "disabled",
		Provider: provider, CheckPath: managedworker.FilePin{Path: "/pack/check.sh", SHA256: dispatchGateDigest("check")},
		SignerIdentity: "TEST-SIGNER",
		Environment:    map[string]string{"GOROOT": "/toolchains/go", "GOTOOLCHAIN": "local", "PATH": "/toolchains/go/bin:/usr/bin:/bin"},
		Toolchains: []managedworker.ToolchainPin{{Name: "go", Executable: managedworker.ExecutablePin{
			Path: "/toolchains/go/bin/go", ResolvedPath: "/toolchains/go/bin/go", SHA256: dispatchGateDigest("go"),
			VersionArgs: []string{"version"}, Version: "go version go1.26.7 linux/amd64",
		}}},
	}
	provisioning, _, err := managedworker.FinalizeProvisioningReceipt(managedworker.ProvisioningReceipt{
		Schema:             managedworker.ProvisioningReceiptSchemaV2,
		CanaryRunner:       managedworker.FilePin{Path: "/city/bin/managed-worker-canary", SHA256: dispatchGateDigest("canary-runner")},
		MemberHeads:        []managedworker.MemberHead{{Name: "gct-xnf", Commit: strings.Repeat("a", 40)}},
		TemplateCommit:     strings.Repeat("b", 40),
		Pack:               managedworker.PackPin{Source: "https://example.invalid/pack", Commit: strings.Repeat("c", 40), SHA256: dispatchGateDigest("pack")},
		Rules:              managedworker.FilePin{Path: "/city/rules", SHA256: dispatchGateDigest("rules")},
		PermissionRevision: dispatchGateDigest("config"), Profiles: []managedworker.WorkerProfile{profile},
	})
	if err != nil {
		t.Fatalf("FinalizeProvisioningReceipt: %v", err)
	}
	environment := managedworker.CanaryEnvironment{
		GCBinary: managedworker.BinaryPin{Commit: strings.Repeat("d", 40), SHA256: dispatchGateDigest("gc")},
		Pack:     provisioning.Pack, PermissionRevision: provisioning.PermissionRevision,
		Profiles:  []managedworker.ProfilePin{{Name: provisioning.Profiles[0].Name, SHA256: provisioning.Profiles[0].WorkerProfileSHA256}},
		Providers: []platforminstall.ProviderPin{provider}, ProvisioningReceiptSHA256: provisioning.ReceiptSHA256,
		Rules: provisioning.Rules, TemplateCommit: provisioning.TemplateCommit,
	}
	_, canaryData, err := managedworker.FinalizeCanaryReceipt(managedworker.CanaryReceipt{
		Schema: managedworker.CanaryReceiptSchemaV1, CanaryRunID: "canary-test", IssuedAt: "2026-08-22T08:00:00Z",
		Result: managedworker.CanaryResultPass, Environment: environment, ProvisioningReceipt: provisioning,
		Runner:    managedworker.FilePin{Path: "/city/bin/managed-worker-canary", SHA256: dispatchGateDigest("canary-runner")},
		Scenarios: []managedworker.CanaryScenario{{Name: "clean-launcher", Outcome: managedworker.CanaryResultPass}},
	})
	if err != nil {
		t.Fatalf("FinalizeCanaryReceipt: %v", err)
	}
	return provisioning, canaryData, environment
}

func dispatchGateDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

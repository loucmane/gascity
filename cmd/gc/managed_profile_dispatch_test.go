package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/events"
	"github.com/gastownhall/gascity/internal/managedworker"
)

type exactProfileDispatchVerifier interface {
	VerifyProfile(rigName, profileName string) error
}

func requireProfileDispatchVerifier(t *testing.T, gate *managedProductDispatchGate) exactProfileDispatchVerifier {
	t.Helper()
	verifier, ok := any(gate).(exactProfileDispatchVerifier)
	if !ok {
		t.Fatal("product dispatch has no exact-profile consumer")
	}
	return verifier
}

// Tests this composition's target selection, not the receipt decoder matrix.
// All observations and reads are recording fakes; no live platform is used.
func TestManagedProfileDispatchRequiresSelectedSigningProof(t *testing.T) {
	for _, tc := range []struct {
		name   string
		kind   managedworker.ProfileKind
		target string
		field  string
	}{
		{"signing", managedworker.ProfileKindSigning, "product/custom.builder", ""},
		{"candidate is not product authority", managedworker.ProfileKindCandidate, "product/custom.builder", "profile.profile_kind"},
		{"inventory is not coverage", managedworker.ProfileKindSigning, "product/custom.helper", "profile.name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire, environment := profileDispatchFixture(t, tc.kind)
			recorder := events.NewFake()
			gate := newManagedProductDispatchGate("/city", profileDispatchConfig(), environment.PermissionRevision, recorder)
			var paths []string
			gate.readFile = func(path string) ([]byte, error) { paths = append(paths, path); return wire, nil }
			gate.observe = func(context.Context, string, string) (managedworker.CanaryEnvironment, error) {
				return environment, nil
			}
			err := requireProfileDispatchVerifier(t, gate).VerifyProfile("product", tc.target)
			if tc.field == "" {
				if err != nil {
					t.Fatal(err)
				}
				if len(recorder.Events) != 0 {
					t.Fatal("successful route emitted a refusal")
				}
			} else {
				var refusal *managedworker.DispatchRefusal
				if !errors.As(err, &refusal) || refusal.Field != tc.field {
					t.Fatalf("error=%v, want %s", err, tc.field)
				}
				if len(recorder.Events) != 1 {
					t.Fatalf("refusal events=%d, want 1", len(recorder.Events))
				}
			}
			if len(paths) != 1 || paths[0] != managedworker.ProfileCanaryReceiptPath("/city", tc.target) {
				t.Fatalf("wrong target path: %v", paths)
			}
		})
	}
}

func TestManagedProfileDispatchInvalidScopedReceiptNeverFallsBack(t *testing.T) {
	_, legacy, environment := dispatchGateFixture(t)
	gate := newManagedProductDispatchGate("/city", profileDispatchConfig(), environment.PermissionRevision, nil)
	var paths []string
	gate.readFile = func(path string) ([]byte, error) {
		paths = append(paths, path)
		if path == managedworker.CanaryReceiptPath("/city") {
			return legacy, nil
		}
		return []byte("invalid scoped receipt"), nil
	}
	gate.observe = func(context.Context, string, string) (managedworker.CanaryEnvironment, error) {
		return environment, nil
	}
	if err := requireProfileDispatchVerifier(t, gate).VerifyProfile("product", "product/custom.builder"); err == nil {
		t.Fatal("invalid scoped proof accepted")
	}
	if len(paths) != 1 {
		t.Fatalf("fell back after invalid existing scoped receipt: %v", paths)
	}
}

func TestManagedProfileDispatchLegacyRequiresExactMembership(t *testing.T) {
	receipt, wire, environment := dispatchGateFixture(t)
	cfg := profileDispatchConfig()
	cfg.Agents = append(cfg.Agents, config.Agent{Name: "implementation-worker", BindingName: "gc", Dir: "product"})
	for _, target := range []string{receipt.Profiles[0].Name, "product/custom.helper"} {
		t.Run(target, func(t *testing.T) {
			gate := newManagedProductDispatchGate("/city", cfg, environment.PermissionRevision, nil)
			var paths []string
			gate.readFile = func(path string) ([]byte, error) {
				paths = append(paths, path)
				if path == managedworker.CanaryReceiptPath("/city") {
					return wire, nil
				}
				return nil, os.ErrNotExist
			}
			gate.observe = func(context.Context, string, string) (managedworker.CanaryEnvironment, error) {
				return environment, nil
			}
			err := requireProfileDispatchVerifier(t, gate).VerifyProfile("product", target)
			if target == receipt.Profiles[0].Name {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "profile") {
				t.Fatalf("legacy receipt authorized absent target: %v", err)
			}
			if len(paths) != 2 || paths[0] != managedworker.ProfileCanaryReceiptPath("/city", target) || paths[1] != managedworker.CanaryReceiptPath("/city") {
				t.Fatalf("legacy fallback path=%v", paths)
			}
		})
	}
}

func TestManagedProfileDispatchRefusesInvalidTargetBeforeObservation(t *testing.T) {
	gate := newManagedProductDispatchGate("/city", profileDispatchConfig(), "revision", nil)
	gate.readFile = func(string) ([]byte, error) { t.Fatal("invalid target reached receipt read"); return nil, nil }
	gate.observe = func(context.Context, string, string) (managedworker.CanaryEnvironment, error) {
		t.Fatal("invalid target reached observation")
		return managedworker.CanaryEnvironment{}, nil
	}
	verifier := requireProfileDispatchVerifier(t, gate)
	for _, target := range []string{"", "product/custom.missing", "other/custom.builder", " product/custom.builder"} {
		if err := verifier.VerifyProfile("product", target); err == nil {
			t.Fatalf("invalid target accepted: %q", target)
		}
	}
	if err := verifier.VerifyProfile("control", ""); err != nil {
		t.Fatalf("unmanaged control compatibility: %v", err)
	}
}

func profileDispatchConfig() *config.City {
	return &config.City{
		Rigs: []config.Rig{{Name: "product", ManagedProduct: true}, {Name: "other"}, {Name: "control"}},
		Agents: []config.Agent{
			{Name: "builder", BindingName: "custom", Dir: "product"},
			{Name: "helper", BindingName: "custom", Dir: "product"},
			{Name: "builder", BindingName: "custom", Dir: "other"},
		},
	}
}

func profileDispatchFixture(t *testing.T, kind managedworker.ProfileKind) ([]byte, managedworker.CanaryEnvironment) {
	t.Helper()
	provisioning, _, environment := dispatchGateFixture(t)
	profile := provisioning.Profiles[0]
	profile.Name, profile.ProfileKind = "product/custom.builder", kind
	if kind == managedworker.ProfileKindCandidate {
		profile.SignerIdentity = managedworker.NoSignerIdentity
		profile.ControlPolicy = &managedworker.FilePin{Path: "/policy/candidate.json", SHA256: dispatchGateDigest("synthetic-policy")}
		profile.Argv = append(profile.Argv, "--settings", profile.ControlPolicy.Path)
	}
	other := profile
	other.Name = "product/custom.helper"
	provisioning.Profiles = []managedworker.WorkerProfile{profile, other}
	provisioning = refinalizeProvisioningWithRunner(t, provisioning, provisioning.CanaryRunner)
	environment.ProvisioningReceiptSHA256 = provisioning.ReceiptSHA256
	environment.Profiles = nil
	for _, p := range provisioning.Profiles {
		environment.Profiles = append(environment.Profiles, managedworker.ProfilePin{Name: p.Name, SHA256: p.WorkerProfileSHA256})
	}
	profile = provisioning.Profiles[0]
	names, err := managedworker.RequiredCanaryScenariosFor(kind)
	if err != nil {
		t.Fatal(err)
	}
	var scenarios []managedworker.CanaryScenario
	for _, name := range names {
		scenarios = append(scenarios, managedworker.CanaryScenario{Name: name, Outcome: managedworker.CanaryResultPass})
	}
	_, wire, err := managedworker.FinalizeCanaryReceipt(managedworker.CanaryReceipt{
		Schema: managedworker.CanaryReceiptSchemaV2, CanaryRunID: "synthetic-profile-dispatch", IssuedAt: "2026-09-08T13:40:00Z",
		Result: managedworker.CanaryResultPass, Environment: environment, ProvisioningReceipt: provisioning,
		Profile: &managedworker.CanaryProfile{Name: profile.Name, Kind: kind, SHA256: profile.WorkerProfileSHA256},
		Runner:  provisioning.CanaryRunner, Scenarios: scenarios,
	})
	if err != nil {
		t.Fatal(err)
	}
	return wire, environment
}

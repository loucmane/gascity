package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/gastownhall/gascity/internal/managedworker"
)

func TestPlatformCanaryRunExecutesPinnedMatrixAndPublishesReceipt(t *testing.T) {
	provisioning, _, environment := dispatchGateFixture(t)
	cityPath := filepath.Join(t.TempDir(), "city")
	if err := os.MkdirAll(cityPath, 0o755); err != nil {
		t.Fatal(err)
	}
	runnerPath := filepath.Join(t.TempDir(), "managed-worker-canary")
	runnerBytes := []byte("reviewed runner")
	if err := os.WriteFile(runnerPath, runnerBytes, 0o755); err != nil {
		t.Fatal(err)
	}
	provisioning = refinalizeProvisioningWithRunner(t, provisioning, managedworker.FilePin{Path: runnerPath, SHA256: digestBytes(runnerBytes)})
	environment.ProvisioningReceiptSHA256 = provisioning.ReceiptSHA256

	var calls []platformCanaryScenarioCall
	previousFactory := platformCanaryRuntimeFactory
	platformCanaryRuntimeFactory = func() platformCanaryRuntime {
		return platformCanaryRuntime{
			FS: fsys.OSFS{},
			Now: func() time.Time {
				return time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC)
			},
			ResolveCity: func() (string, error) { return cityPath, nil },
			LoadInputs: func(context.Context, string) (managedworker.ProvisioningReceipt, managedworker.CanaryEnvironment, error) {
				return provisioning, environment, nil
			},
			RunScenario: func(_ context.Context, call platformCanaryScenarioCall) (managedworker.CanaryScenarioEvidence, error) {
				calls = append(calls, call)
				return passingPlatformCanaryEvidence(call.Scenario), nil
			},
		}
	}
	t.Cleanup(func() { platformCanaryRuntimeFactory = previousFactory })

	var stdout, stderr bytes.Buffer
	cmd := newPlatformCmd(&stdout, &stderr)
	cmd.SetArgs([]string{
		"canary",
		"--run-id", "canary-cli-green",
		"--runner", runnerPath,
		"--runner-sha256", digestBytes(runnerBytes),
		"--launcher-source", "/source/launcher",
		"--base-commit", strings.Repeat("a", 40),
		"--scratch-root", "/scratch/canary",
		"--max-wall-time", "12m",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v; stderr=%s", err, stderr.String())
	}
	if len(calls) != len(managedworker.RequiredCanaryScenarios()) {
		t.Fatalf("scenario calls = %d, want %d", len(calls), len(managedworker.RequiredCanaryScenarios()))
	}
	for index, call := range calls {
		if call.Scenario != managedworker.RequiredCanaryScenarios()[index] || call.RunnerPath != runnerPath || call.RunID != "canary-cli-green" || call.CityPath != cityPath || call.LauncherSource != "/source/launcher" || call.BaseCommit != strings.Repeat("a", 40) || call.ScratchRoot != "/scratch/canary" {
			t.Fatalf("scenario call[%d] = %+v", index, call)
		}
		if !reflect.DeepEqual(call.WorkerProfile, provisioning.Profiles[0]) {
			t.Fatalf("scenario call[%d] worker profile = %+v, want %+v", index, call.WorkerProfile, provisioning.Profiles[0])
		}
	}
	receiptBytes, err := os.ReadFile(managedworker.CanaryReceiptPath(cityPath))
	if err != nil {
		t.Fatalf("ReadFile(canary receipt): %v", err)
	}
	receipt, err := managedworker.LoadCanaryReceipt(receiptBytes)
	if err != nil {
		t.Fatalf("LoadCanaryReceipt: %v", err)
	}
	if receipt.CanaryRunID != "canary-cli-green" || receipt.IssuedAt != "2026-08-22T09:00:00Z" {
		t.Fatalf("receipt identity = %+v", receipt)
	}
	for _, want := range []string{"platform canary result=pass", receipt.ReceiptSHA256, managedworker.CanaryReceiptPath(cityPath)} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout missing %q: %s", want, stdout.String())
		}
	}
}

func TestPlatformCanaryRefusesRunnerDigestBeforeAnyScenario(t *testing.T) {
	provisioning, _, environment := dispatchGateFixture(t)
	runnerPath := filepath.Join(t.TempDir(), "runner")
	if err := os.WriteFile(runnerPath, []byte("actual"), 0o755); err != nil {
		t.Fatal(err)
	}
	provisioning = refinalizeProvisioningWithRunner(t, provisioning, managedworker.FilePin{Path: runnerPath, SHA256: digestBytes([]byte("other"))})
	environment.ProvisioningReceiptSHA256 = provisioning.ReceiptSHA256
	called := false
	previousFactory := platformCanaryRuntimeFactory
	platformCanaryRuntimeFactory = func() platformCanaryRuntime {
		return platformCanaryRuntime{
			FS:          fsys.OSFS{},
			Now:         time.Now,
			ResolveCity: func() (string, error) { return t.TempDir(), nil },
			LoadInputs: func(context.Context, string) (managedworker.ProvisioningReceipt, managedworker.CanaryEnvironment, error) {
				return provisioning, environment, nil
			},
			RunScenario: func(context.Context, platformCanaryScenarioCall) (managedworker.CanaryScenarioEvidence, error) {
				called = true
				return managedworker.CanaryScenarioEvidence{}, nil
			},
		}
	}
	t.Cleanup(func() { platformCanaryRuntimeFactory = previousFactory })

	cmd := newPlatformCmd(&bytes.Buffer{}, &bytes.Buffer{})
	cmd.SetArgs([]string{
		"canary", "--run-id", "digest-refusal", "--runner", runnerPath,
		"--runner-sha256", digestBytes([]byte("other")), "--launcher-source", "/launcher",
		"--base-commit", strings.Repeat("b", 40), "--scratch-root", "/scratch", "--max-wall-time", "1m",
	})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "runner sha256 mismatch") {
		t.Fatalf("Execute error = %v, want runner digest refusal", err)
	}
	if called {
		t.Fatal("scenario ran after runner digest refusal")
	}
}

func TestPlatformCanaryRefusesRunnerNotBoundToProvisioningReceipt(t *testing.T) {
	provisioning, _, environment := dispatchGateFixture(t)
	runnerPath := filepath.Join(t.TempDir(), "unreviewed-runner")
	runnerBytes := []byte("operator-selected runner")
	if err := os.WriteFile(runnerPath, runnerBytes, 0o755); err != nil {
		t.Fatal(err)
	}
	called := false
	previousFactory := platformCanaryRuntimeFactory
	platformCanaryRuntimeFactory = func() platformCanaryRuntime {
		return platformCanaryRuntime{
			FS:          fsys.OSFS{},
			Now:         time.Now,
			ResolveCity: func() (string, error) { return t.TempDir(), nil },
			LoadInputs: func(context.Context, string) (managedworker.ProvisioningReceipt, managedworker.CanaryEnvironment, error) {
				return provisioning, environment, nil
			},
			RunScenario: func(context.Context, platformCanaryScenarioCall) (managedworker.CanaryScenarioEvidence, error) {
				called = true
				return managedworker.CanaryScenarioEvidence{}, nil
			},
		}
	}
	t.Cleanup(func() { platformCanaryRuntimeFactory = previousFactory })

	cmd := newPlatformCmd(&bytes.Buffer{}, &bytes.Buffer{})
	cmd.SetArgs([]string{
		"canary", "--run-id", "unreviewed-runner", "--runner", runnerPath,
		"--runner-sha256", digestBytes(runnerBytes), "--launcher-source", "/launcher",
		"--base-commit", strings.Repeat("c", 40), "--scratch-root", "/scratch", "--max-wall-time", "1m",
	})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "runner is not bound to provisioning receipt") {
		t.Fatalf("Execute error = %v, want provisioning-bound runner refusal", err)
	}
	if called {
		t.Fatal("scenario ran with a runner not bound to the provisioning receipt")
	}
}

func TestPlatformCanaryScenarioOutputIsStrictJSON(t *testing.T) {
	evidence := passingPlatformCanaryEvidence(managedworker.CanaryScenarioMissingProvider)
	encoded, err := encodePlatformCanaryScenarioEvidence(evidence)
	if err != nil {
		t.Fatalf("encodePlatformCanaryScenarioEvidence: %v", err)
	}
	loaded, err := decodePlatformCanaryScenarioEvidence(encoded)
	if err != nil {
		t.Fatalf("decodePlatformCanaryScenarioEvidence: %v", err)
	}
	if !reflect.DeepEqual(loaded, evidence) {
		t.Fatalf("round trip = %+v, want %+v", loaded, evidence)
	}
	unknown := append([]byte(nil), encoded[:len(encoded)-1]...)
	unknown = append(unknown, []byte(`,"unknown":true}`)...)
	if _, err := decodePlatformCanaryScenarioEvidence(unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}
}

func TestRootRegistersPlatformCanaryCommand(t *testing.T) {
	root := newRootCmdWithOptions(&bytes.Buffer{}, &bytes.Buffer{}, rootCommandOptions{})
	command, _, err := root.Find([]string{"platform", "canary"})
	if err != nil || command == nil || command.Name() != "canary" {
		t.Fatalf("platform canary command = %v, err=%v", command, err)
	}
}

func passingPlatformCanaryEvidence(name string) managedworker.CanaryScenarioEvidence {
	evidence := managedworker.CanaryScenarioEvidence{
		Name:       name,
		Resolution: managedworker.CanaryResolutionRecovered,
	}
	switch name {
	case managedworker.CanaryScenarioCleanLauncher:
		evidence.Resolution = managedworker.CanaryResolutionCompleted
		evidence.CompletedSteps = managedworker.RequiredCleanLauncherSteps()
		evidence.SignedCandidate = true
	case managedworker.CanaryScenarioFailedFinalize:
		evidence.FinalizeOutcome = managedworker.CanaryFinalizeFailed
		evidence.PublisherOutcome = managedworker.CanaryPublisherBlocked
	case managedworker.CanaryScenarioSuccessfulFinalize:
		evidence.FinalizeOutcome = managedworker.CanaryFinalizePassed
		evidence.PublisherOutcome = managedworker.CanaryPublisherNoop
	}
	return evidence
}

func digestBytes(data []byte) string {
	return dispatchGateDigest(string(data))
}

func refinalizeProvisioningWithRunner(t *testing.T, receipt managedworker.ProvisioningReceipt, runner managedworker.FilePin) managedworker.ProvisioningReceipt {
	t.Helper()
	receipt.ReceiptSHA256 = ""
	receipt.CanaryRunner = runner
	for index := range receipt.Profiles {
		receipt.Profiles[index].WorkerProfileSHA256 = ""
	}
	finalized, _, err := managedworker.FinalizeProvisioningReceipt(receipt)
	if err != nil {
		t.Fatalf("FinalizeProvisioningReceipt: %v", err)
	}
	return finalized
}

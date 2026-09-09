package main

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/gastownhall/gascity/internal/managedworker"
)

func TestPlatformCanaryExplicitProfileUsesActualIdentity(t *testing.T) {
	for _, test := range []struct {
		name        string
		kind        managedworker.ProfileKind
		policyFault string
	}{
		{"candidate", managedworker.ProfileKindCandidate, ""},
		{"signing", managedworker.ProfileKindSigning, ""},
		{"missing policy", managedworker.ProfileKindCandidate, "missing"},
		{"changed policy", managedworker.ProfileKindCandidate, "changed"},
		{"symlink policy", managedworker.ProfileKindCandidate, "symlink"},
		{"policy changed during scenario", managedworker.ProfileKindCandidate, "after"},
	} {
		t.Run(test.name, func(t *testing.T) {
			kind := test.kind
			policyBytes := []byte(`{"_gc":{"schema":"gc.candidate-control-policy.v1","profile_kind":"candidate"}}`)
			provisioning, _, environment := dispatchGateFixture(t)
			profile := &provisioning.Profiles[0]
			profile.Name = "build/unbranded.agent"
			profile.ProfileKind = kind
			if kind == managedworker.ProfileKindCandidate {
				profile.SignerIdentity = managedworker.NoSignerIdentity
				profile.ControlPolicy = &managedworker.FilePin{Path: "/policies/candidate.json", SHA256: dispatchGateDigest(string(policyBytes))}
				profile.Provider.Name = "claude"
				profile.Provider.Path = "/city/bin/claude"
				profile.Provider.ResolvedPath = "/provider/claude"
				profile.Provider.Version = "synthetic-claude 1"
				profile.Provider.SHA256 = dispatchGateDigest("synthetic-claude")
				profile.Argv = []string{profile.Provider.Path, "--settings", profile.ControlPolicy.Path}
			}
			provisioning = refinalizeProvisioningWithRunner(t, provisioning, provisioning.CanaryRunner)
			profile = &provisioning.Profiles[0]
			environment.Profiles = []managedworker.ProfilePin{{Name: profile.Name, SHA256: profile.WorkerProfileSHA256}}
			environment.ProvisioningReceiptSHA256 = provisioning.ReceiptSHA256
			environment.Providers[0] = profile.Provider
			filesystem := fsys.NewFake()
			filesystem.Files[provisioning.CanaryRunner.Path] = []byte("canary-runner")
			filesystem.Modes[provisioning.CanaryRunner.Path] = 0o755
			if profile.ControlPolicy != nil {
				path := profile.ControlPolicy.Path
				filesystem.Files[path] = policyBytes
				switch test.policyFault {
				case "missing":
					delete(filesystem.Files, path)
				case "changed":
					filesystem.Files[path] = []byte("changed")
				case "symlink":
					delete(filesystem.Files, path)
					filesystem.Files["/policy-target.json"] = policyBytes
					filesystem.Symlinks[path] = "/policy-target.json"
				}
			}
			var calls []platformCanaryScenarioCall
			previous := platformCanaryRuntimeFactory
			platformCanaryRuntimeFactory = func() platformCanaryRuntime {
				return platformCanaryRuntime{
					FS: filesystem, Now: func() time.Time { return time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC) },
					ResolveCity: func() (string, error) { return "/city", nil },
					LoadInputs: func(context.Context, string) (managedworker.ProvisioningReceipt, managedworker.CanaryEnvironment, error) {
						return provisioning, environment, nil
					},
					RunScenario: func(_ context.Context, call platformCanaryScenarioCall) (managedworker.CanaryScenarioEvidence, error) {
						calls = append(calls, call)
						if test.policyFault == "after" {
							filesystem.Files[profile.ControlPolicy.Path] = []byte("changed during scenario")
						}
						if call.Scenario == managedworker.CanaryScenarioCandidateLauncher {
							return managedworker.CanaryScenarioEvidence{
								Name: call.Scenario, Resolution: managedworker.CanaryResolutionCompleted, CompletedSteps: managedworker.RequiredCandidateLauncherSteps(),
							}, nil
						}
						return passingPlatformCanaryEvidence(call.Scenario), nil
					},
				}
			}
			t.Cleanup(func() { platformCanaryRuntimeFactory = previous })
			var stdout bytes.Buffer
			command := newPlatformCmd(&stdout, &bytes.Buffer{})
			command.SetArgs([]string{
				"canary", "--run-id", "typed-cli", "--runner", provisioning.CanaryRunner.Path,
				"--runner-sha256", provisioning.CanaryRunner.SHA256, "--launcher-source", "/source", "--base-commit", strings.Repeat("a", 40),
				"--scratch-root", "/scratch", "--profile", profile.Name, "--profile-kind", string(kind),
			})
			err := command.Execute()
			if test.policyFault != "" {
				expectedCalls := 0
				if test.policyFault == "after" {
					expectedCalls = 1
				}
				if err == nil || !strings.Contains(err.Error(), "control_policy") || len(calls) != expectedCalls {
					t.Fatalf("policy failure missed its boundary: err=%v calls=%d want=%d", err, len(calls), expectedCalls)
				}
				for _, call := range filesystem.Calls {
					if call.Method == "WriteFile" || call.Method == "Rename" || call.Method == "MkdirAll" {
						t.Fatalf("policy failure wrote canary state: %+v", call)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			names, _ := managedworker.RequiredCanaryScenariosFor(kind)
			if len(calls) != len(names) {
				t.Fatalf("calls=%d want=%d", len(calls), len(names))
			}
			for _, call := range calls {
				if !reflect.DeepEqual(call.WorkerProfile, *profile) {
					t.Fatal("runner did not receive the exact selected profile")
				}
			}
			path := managedworker.ProfileCanaryReceiptPath("/city", profile.Name)
			if _, err := managedworker.VerifyProfileCanaryReceipt(filesystem.Files[path], environment, profile.Name, kind); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(stdout.String(), path) {
				t.Fatal("CLI reported the wrong receipt path")
			}
			if _, exists := filesystem.Files[managedworker.CanaryReceiptPath("/city")]; exists {
				t.Fatal("typed canary wrote legacy product authority")
			}
		})
	}
}

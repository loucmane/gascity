package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/managedworker"
	"github.com/gastownhall/gascity/internal/platforminstall"
)

// These fixtures use the production policy reader without starting a provider,
// probing credentials, or invoking the signer.
func TestManagedWorkerDefaultPolicyReaderRefusesUnsafeFilesBeforeProvider(t *testing.T) {
	for _, shape := range []string{"regular", "leaf symlink", "parent symlink", "group writable", "world writable"} {
		t.Run(shape, func(t *testing.T) {
			root := t.TempDir()
			policy := []byte(`{"_gc":{"schema":"gc.candidate-control-policy.v1","profile_kind":"candidate"}}`)
			actual := filepath.Join(root, "actual.json")
			if err := os.WriteFile(actual, policy, 0o600); err != nil {
				t.Fatal(err)
			}
			path := actual
			switch shape {
			case "leaf symlink":
				path = filepath.Join(root, "policy.json")
				if err := os.Symlink(actual, path); err != nil {
					t.Fatal(err)
				}
			case "parent symlink":
				alias := filepath.Join(root, "alias")
				if err := os.Symlink(root, alias); err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(alias, "actual.json")
			case "group writable":
				if err := os.Chmod(actual, 0o660); err != nil {
					t.Fatal(err)
				}
			case "world writable":
				if err := os.Chmod(actual, 0o606); err != nil {
					t.Fatal(err)
				}
			}
			provisioning, _, _ := dispatchGateFixture(t)
			provisioning.ReceiptSHA256 = ""
			profile := &provisioning.Profiles[0]
			profile.WorkerProfileSHA256 = ""
			profile.Name = "build/unbranded.agent"
			profile.ProfileKind = managedworker.ProfileKindCandidate
			profile.SignerIdentity = managedworker.NoSignerIdentity
			profile.ControlPolicy = &managedworker.FilePin{Path: path, SHA256: dispatchGateDigest(string(policy))}
			profile.Argv = []string{"/fixture/claude", "--settings", path}
			profile.Provider = platforminstall.ProviderPin{
				Name: "claude", Path: "/fixture/claude", ResolvedPath: "/fixture/claude",
				SHA256: dispatchGateDigest("claude"), Version: "fixture-claude", VersionArgs: []string{"--version"},
			}
			provisioning, wire, err := managedworker.FinalizeProvisioningReceipt(provisioning)
			if err != nil {
				t.Fatal(err)
			}
			profile = &provisioning.Profiles[0]
			probes := defaultManagedWorkerPreflightProbes()
			read := probes.ReadFile
			probes.ReadFile = func(name string) ([]byte, error) {
				if name == provisioning.Rules.Path {
					return []byte("rules"), nil
				}
				if name == profile.CheckPath.Path {
					return []byte("check"), nil
				}
				return read(name)
			}
			providerCalls := 0
			probes.InspectProvider = func(context.Context, platforminstall.ProviderPin) error { providerCalls++; return nil }
			probes.InspectToolchain = func(context.Context, managedworker.ToolchainPin, map[string]string) error { return nil }
			probes.ProbeReadiness = func(context.Context, string) error { return nil }
			probes.ProbeSigner = func(context.Context, string) error { t.Fatal("candidate reached signer"); return nil }
			report, err := managedworker.Preflight(context.Background(), managedworker.PreflightRequest{
				Receipt: wire, ProfileName: profile.Name, ObservedProfile: *profile,
				PermissionRevision: provisioning.PermissionRevision, CheckPath: profile.CheckPath.Path,
			}, probes)
			if shape == "regular" {
				if err != nil || !report.OK || providerCalls != 1 {
					t.Fatalf("regular policy: %+v, %v, providers=%d", report, err, providerCalls)
				}
			} else if err == nil || !strings.Contains(err.Error(), "control_policy") || providerCalls != 0 {
				t.Fatalf("unsafe policy not refused before provider: report=%+v error=%v providers=%d", report, err, providerCalls)
			}
		})
	}
}

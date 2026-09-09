package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/managedworker"
	"github.com/gastownhall/gascity/internal/platforminstall"
	"github.com/gastownhall/gascity/internal/runtime"
	sessionpkg "github.com/gastownhall/gascity/internal/session"
	"github.com/gastownhall/gascity/internal/shellquote"
)

// This is the controller composition proof: real receipt/policy files and the
// resolved config/work snapshot reach production preflight, then only a fake
// runtime. No provider binary, credential probe, signer or live city is used.
func TestConfigureManagedWorkerTypedLaunchBindsResolvedContract(t *testing.T) {
	for _, tc := range []struct {
		name      string
		kind      managedworker.ProfileKind
		wantError string
	}{
		{"candidate", managedworker.ProfileKindCandidate, ""},
		{"signing", managedworker.ProfileKindSigning, ""},
		{"argv drift", managedworker.ProfileKindCandidate, "worker_profile_sha256 mismatch"},
		{"environment drift", managedworker.ProfileKindCandidate, "worker_profile_sha256 mismatch"},
		{"check path drift", managedworker.ProfileKindCandidate, "gc.check_path mismatch"},
		{"policy drift", managedworker.ProfileKindCandidate, "control_policy sha256 mismatch"},
		{"unrelated route", managedworker.ProfileKindCandidate, "gc.check_path is not stamped"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cityPath := t.TempDir()
			write := func(path string, data []byte, mode os.FileMode) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, mode); err != nil {
					t.Fatal(err)
				}
			}
			cfg := &config.City{Agents: []config.Agent{{Name: "builder", BindingName: "custom", Dir: "assembly"}}}
			profileName := cfg.Agents[0].QualifiedName()
			if profileName != "assembly/custom.builder" {
				t.Fatalf("unexpected qualified identity: %q", profileName)
			}
			policyPath := filepath.Join(cityPath, "policies", "worker.json")
			policy := []byte(`{"_gc":{"schema":"gc.worker-control-policy.v1","profile_kind":"` + string(tc.kind) + `"}}`)
			checkPath := filepath.Join(cityPath, "checks", "task.sh")
			rulesPath := filepath.Join(cityPath, "rules", "worker.rules")
			write(policyPath, policy, 0o600)
			write(checkPath, []byte("check"), 0o500)
			write(rulesPath, []byte("rules"), 0o400)

			receipt, _, _ := dispatchGateFixture(t)
			receipt.ReceiptSHA256 = ""
			receipt.Rules = managedworker.FilePin{Path: rulesPath, SHA256: preflightDigest("rules")}
			profile := receipt.Profiles[0]
			profile.Name, profile.WorkerProfileSHA256 = profileName, ""
			profile.ProfileKind = tc.kind
			profile.SignerIdentity = managedworker.NoSignerIdentity
			if tc.kind == managedworker.ProfileKindSigning {
				profile.SignerIdentity = "SYNTHETIC-SIGNER"
			}
			profile.ControlPolicy = &managedworker.FilePin{Path: policyPath, SHA256: preflightDigest(string(policy))}
			profile.CheckPath = managedworker.FilePin{Path: checkPath, SHA256: preflightDigest("check")}
			profile.WritableRoots = []string{filepath.Join(cityPath, "task")}
			profile.Provider = platforminstall.ProviderPin{
				Name: "claude", Path: "/fixture/claude", ResolvedPath: "/fixture/claude",
				SHA256: preflightDigest("claude"), Version: "fixture-claude", VersionArgs: []string{"--version"},
			}
			profile.Argv = []string{profile.Provider.Path, "--settings", policyPath}
			receipt.Profiles = []managedworker.WorkerProfile{profile}
			receipt, wire, err := managedworker.FinalizeProvisioningReceipt(receipt)
			if err != nil {
				t.Fatal(err)
			}
			write(managedworker.ProvisioningReceiptPath(cityPath), wire, 0o400)

			environment := make(map[string]string, len(profile.Environment))
			for key, value := range profile.Environment {
				environment[key] = value
			}
			item := preparedStart{
				candidate: startCandidate{
					info: sessionpkg.Info{SessionName: "synthetic-typed", SessionNameMetadata: "synthetic-typed"},
					tp:   TemplateParams{TemplateName: profileName, RigName: "assembly"},
				},
				cfg: runtime.Config{
					Command: shellquote.Join(append(append([]string(nil), profile.Argv...), "--session-id", "synthetic-id")),
					WorkDir: profile.WritableRoots[0], Env: environment,
				},
				managedWorkerArgv: append([]string(nil), profile.Argv...),
			}
			if got := managedWorkerProfileName(item.candidate, cfg); got != profileName {
				t.Fatalf("resolved profile = %q, want exact config identity %q", got, profileName)
			}
			work := []beads.Bead{{ID: "synthetic-work", Status: "open", Metadata: map[string]string{
				beadmeta.RoutedToMetadataKey: profileName, beadmeta.CheckPathMetadataKey: checkPath,
			}}}
			switch tc.name {
			case "argv drift":
				item.managedWorkerArgv = append(item.managedWorkerArgv, "--unreviewed")
			case "environment drift":
				item.cfg.Env["PATH"] = "/unreviewed:/usr/bin"
			case "check path drift":
				work[0].Metadata[beadmeta.CheckPathMetadataKey] = filepath.Join(cityPath, "other-check")
			case "policy drift":
				write(policyPath, []byte("changed policy"), 0o600)
			case "unrelated route":
				work[0].Metadata[beadmeta.RoutedToMetadataKey] = "other/custom.builder"
			}
			providers, readiness, signers := 0, 0, 0
			probes := defaultManagedWorkerPreflightProbes()
			probes.InspectProvider = func(_ context.Context, pin platforminstall.ProviderPin) error {
				providers++
				if !reflect.DeepEqual(pin, profile.Provider) {
					t.Fatalf("provider pin changed: %+v", pin)
				}
				return nil
			}
			probes.InspectToolchain = func(_ context.Context, _ managedworker.ToolchainPin, env map[string]string) error {
				if !reflect.DeepEqual(env, profile.Environment) {
					t.Fatalf("toolchain environment changed: %+v", env)
				}
				return nil
			}
			probes.ProbeReadiness = func(_ context.Context, provider string) error {
				readiness++
				if provider != profile.Provider.Name {
					t.Fatalf("wrong readiness provider %q", provider)
				}
				return nil
			}
			probes.ProbeSigner = func(_ context.Context, identity string) error {
				signers++
				if tc.kind != managedworker.ProfileKindSigning || identity != profile.SignerIdentity {
					t.Fatalf("unexpected signer probe %q", identity)
				}
				return nil
			}
			configureManagedWorkerPreflight(&item, cityPath, cfg, receipt.PermissionRevision, newTaskCheckPathResolver(work), &probes)
			if item.preflight == nil {
				t.Fatal("no production preflight installed")
			}
			sp := runtime.NewFake()
			started, err := startPreparedStartCandidate(context.Background(), item, cityPath, nil, sp, cfg, nil, nil)
			starts := sp.CountCalls("Start", "synthetic-typed")
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) || started || starts != 0 || providers != 0 || readiness != 0 || signers != 0 {
					t.Fatalf("drift reached provider: error=%v started=%v starts=%d providers=%d readiness=%d signers=%d", err, started, starts, providers, readiness, signers)
				}
				return
			}
			wantSigners := 0
			if tc.kind == managedworker.ProfileKindSigning {
				wantSigners = 1
			}
			if err != nil || !started || starts != 1 || providers != 1 || readiness != 1 || signers != wantSigners {
				t.Fatalf("typed launch failed: error=%v started=%v starts=%d providers=%d readiness=%d signers=%d", err, started, starts, providers, readiness, signers)
			}
			if got := sp.LastStartConfig("synthetic-typed"); got == nil || got.Command != item.cfg.Command {
				t.Fatalf("session-specific launch command lost: %+v", got)
			}
		})
	}
}

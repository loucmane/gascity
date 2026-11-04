package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/managedworker"
	"github.com/gastownhall/gascity/internal/runtime"
	sessionpkg "github.com/gastownhall/gascity/internal/session"
)

func TestPreparedStartRefusesInvalidProviderAllowlistAuthority(t *testing.T) {
	city := t.TempDir()
	dir := filepath.Join(city, ".gc", "platform")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "install-manifest.json"), []byte(`{"integrity":{"provider_allowlist":{}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	item := preparedStart{}
	configureManagedWorkerPreflight(&item, city, nil, "", nil, nil)
	if err := item.preflight(context.Background(), &item.cfg); err == nil || !strings.Contains(err.Error(), "provider allowlist") {
		t.Fatalf("invalid authority must refuse even without provisioning receipt: %v", err)
	}
}

func TestPreparedRuntimeLaunchUsesPreflightResolvedCommand(t *testing.T) {
	provider := runtime.NewFake()
	item := preparedStart{
		candidate: startCandidate{info: sessionpkg.Info{SessionName: "native", SessionNameMetadata: "native"}, tp: TemplateParams{TemplateName: "worker"}},
		cfg:       runtime.Config{Command: "/unapproved/wrapper", Env: map[string]string{"PATH": "/hostile"}},
	}
	item.preflight = func(_ context.Context, launch *runtime.Config) error {
		launch.Command = "/usr/bin/python3 -I /signed/helper.py"
		return nil
	}
	started, err := startPreparedStartCandidate(context.Background(), item, t.TempDir(), nil, provider, nil, nil, nil, nil)
	if err != nil || !started {
		t.Fatalf("runtime-only start = %v, %v", started, err)
	}
	if got := provider.LastStartConfig("native"); got == nil || got.Command != "/usr/bin/python3 -I /signed/helper.py" {
		t.Fatalf("runtime received command before resolution: %+v", got)
	}
}

func TestLegacyLaunchDriftKeepsExistingManagedReceiptBehavior(t *testing.T) {
	env, _, bead := setupLaunchDriftResumeEnv(t)
	city := t.TempDir()
	receipt, _, _ := dispatchGateFixture(t)
	receipt.Profiles[0].Name = "worker"
	receipt.ReceiptSHA256 = ""
	receipt.Profiles[0].WorkerProfileSHA256 = ""
	_, data, err := managedworker.FinalizeProvisioningReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	path := managedworker.ProvisioningReceiptPath(city)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	reconcileSessionBeadsAtPath(
		context.Background(), city, []beads.Bead{bead}, env.desiredState,
		configuredSessionNames(env.cfg, "", env.store), env.cfg, env.sp, env.store,
		nil, nil, nil, nil, env.dt, map[string]int{"worker": 1}, false, nil, "",
		nil, env.clk, env.rec, 0, 0, &env.stdout, &env.stderr, env.startOptions...,
	)
	if got := env.sp.CountCalls("Relaunch", "worker"); got != 1 {
		t.Fatalf("legacy relaunch changed: %d; %s", got, env.stderr.String())
	}
}

func TestLaunchDriftRelaunchRefusesInvalidProviderAuthority(t *testing.T) {
	env, _, bead := setupLaunchDriftResumeEnv(t)
	city := t.TempDir()
	dir := filepath.Join(city, ".gc", "platform")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "install-manifest.json"), []byte(`{"integrity":{"provider_allowlist":{}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	reconcileSessionBeadsAtPath(
		context.Background(), city, []beads.Bead{bead}, env.desiredState,
		configuredSessionNames(env.cfg, "", env.store), env.cfg, env.sp, env.store,
		nil, nil, nil, nil, env.dt, map[string]int{"worker": 1}, false, nil, "",
		nil, env.clk, env.rec, 0, 0, &env.stdout, &env.stderr, env.startOptions...,
	)
	if got := env.sp.CountCalls("Relaunch", "worker"); got != 0 {
		t.Fatalf("launched with invalid authority: %d", got)
	}
	if got := env.sp.CountCalls("Start", "worker"); got != 1 {
		t.Fatalf("fallback started with invalid authority: %d (one fixture start expected)", got)
	}
	if !strings.Contains(env.stderr.String(), "provider allowlist") {
		t.Fatalf("missing refusal: %s", env.stderr.String())
	}
}

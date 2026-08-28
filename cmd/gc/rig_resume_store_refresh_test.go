package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/gastownhall/gascity/internal/suspensionstate"
)

// TestDoRigResumeRefreshesRunningControllerStoreRegistry pins the missing
// handoff between the direct runtime-state fallback and a live controller.
// The state must be durable before the reload is requested so the controller
// rebuilds the rig store with the resumed refresh policy on that same run.
func TestDoRigResumeRefreshesRunningControllerStoreRegistry(t *testing.T) {
	prevAlive := apiRouteControllerAliveHook
	prevReload := rigReloadControllerConfig
	t.Cleanup(func() {
		apiRouteControllerAliveHook = prevAlive
		rigReloadControllerConfig = prevReload
	})

	f := fsys.NewFake()
	cityPath := "/city"
	cfg := &config.City{
		Workspace: config.Workspace{Name: "city"},
		Rigs: []config.Rig{{
			Name:             "hpfetcher",
			Path:             "/rigs/hpfetcher",
			Prefix:           "hpf",
			SuspendedOnStart: true,
		}},
	}
	data, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	f.Files[filepath.Join(cityPath, "city.toml")] = data

	apiRouteControllerAliveHook = func(string) int { return 4242 }
	reloads := 0
	rigReloadControllerConfig = func(gotCityPath string) error {
		reloads++
		if gotCityPath != cityPath {
			t.Fatalf("reload city path = %q, want %q", gotCityPath, cityPath)
		}
		state, err := suspensionstate.Load(f, cityPath)
		if err != nil {
			t.Fatalf("load suspension state during reload: %v", err)
		}
		if resumed, ok := suspensionstate.ExplicitRig(state, "hpfetcher"); !ok || resumed {
			t.Fatalf("reload observed rig override = (%t, %t), want explicit resumed", resumed, ok)
		}
		return nil
	}

	var stdout, stderr bytes.Buffer
	if code := doRigResume(f, cityPath, "hpfetcher", &stdout, &stderr); code != 0 {
		t.Fatalf("doRigResume code = %d, stderr=%q", code, stderr.String())
	}
	if reloads != 1 {
		t.Fatalf("controller reloads = %d, want exactly 1", reloads)
	}
}

// TestResumedRigStoreRebuildSeesRoutedWorkWithoutRestart composes the other
// half of the handoff: a runtime suspension flip changes the store signature,
// rebuilding the formerly dormant cache so routed work is visible immediately
// and its reconciler is armed without replacing the controller process.
func TestResumedRigStoreRebuildSeesRoutedWorkWithoutRestart(t *testing.T) {
	t.Setenv("GC_BEADS", "")
	t.Setenv("GC_BEADS_SCOPE_ROOT", "")
	t.Setenv("GC_AGENT", "hpfetcher-controller")

	prevOpen := controllerStateOpenRigStoreAtForCity
	t.Cleanup(func() { controllerStateOpenRigStoreAtForCity = prevOpen })

	cityDir := t.TempDir()
	rigDir := filepath.Join(cityDir, "hpfetcher")
	if err := os.MkdirAll(filepath.Join(rigDir, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cityDir, "city.toml"), []byte("[workspace]\nname = \"city\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigDir, ".beads", "metadata.json"), []byte(`{"database":"dolt","backend":"dolt","dolt_mode":"embedded","dolt_database":"hpf"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	backing := beads.NewMemStore()
	controllerStateOpenRigStoreAtForCity = func(_ context.Context, _ beads.StoreOpenOptions) (beads.StoreOpenResult, error) {
		return beads.StoreOpenResult{Store: backing}, nil
	}

	cfg := &config.City{
		Workspace: config.Workspace{Name: "city"},
		Rigs: []config.Rig{{
			Name:             "hpfetcher",
			Path:             rigDir,
			Prefix:           "hpf",
			SuspendedOnStart: true,
		}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cs := &controllerState{cityPath: cityDir, cfg: cfg, cacheCtx: ctx}

	beforeStores := cs.buildStores(cfg)
	before := underlyingPolicyStoreForTest(beforeStores["hpfetcher"]).(*beads.CachingStore)
	if got := before.Stats().State; got != "partial" {
		t.Fatalf("suspended cache state = %q, want partial (no reconciler)", got)
	}
	created, err := backing.Create(beads.Bead{
		ID:       "hpf-work",
		Title:    "routed design work",
		Status:   "open",
		Type:     "task",
		Metadata: beads.StringMap{"gc.routed_to": "hpfetcher/gc.design-author"},
	})
	if err != nil {
		t.Fatalf("create routed work: %v", err)
	}
	if _, err := before.Get(created.ID); err == nil {
		t.Fatal("suspended startup cache unexpectedly observed backing-store work")
	}

	resumed := false
	if err := suspensionstate.SetRigSuspended(fsys.OSFS{}, cityDir, "hpfetcher", &resumed); err != nil {
		t.Fatalf("record runtime resume: %v", err)
	}
	afterStores := cs.buildStores(cfg)
	after := underlyingPolicyStoreForTest(afterStores["hpfetcher"]).(*beads.CachingStore)
	defer after.StopReconciler()
	if after == before {
		t.Fatal("runtime resume reused dormant rig-store cache")
	}
	if _, err := after.Get(created.ID); err != nil {
		t.Fatalf("resumed rig store did not expose routed work: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for after.Stats().State != "live" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if stats := after.Stats(); stats.State != "live" {
		t.Fatalf("resumed cache state = %q, want live reconciler-backed cache", stats.State)
	}
}

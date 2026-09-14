package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/beads/contract"
	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/gastownhall/gascity/internal/taskattempt"
)

// TestTaskAttemptConfiguredNativeStore protects the real configured rig-store
// composition: the control-dispatch resolver deliberately selects BdStore, whose
// installed CLI may lack fencing, even when native metadata CAS is available.
// The existing real-Dolt fixture owns the process and gates this process lane.
func TestTaskAttemptConfiguredNativeStore(t *testing.T) {
	// startPasswordedDoltServer owns the process-lane gate before any spawn.
	// Keep that shared resource owner instead of duplicating its skip marker.
	clearInheritedBeadsEnv(t)
	resetFlags(t)
	t.Setenv("GC_HOME", t.TempDir())
	cityPath := t.TempDir()
	rigPath, err := writeManagedBdWaitTestCityScaffold(cityPath)
	if err != nil {
		t.Fatal(err)
	}
	requireNoLeakedDoltAfterForPaths(t, cityPath)
	const projectID = "native-task-attempt-fixture"
	setupQueries := append(seedDatabaseProjectIDQueries(projectID),
		"CALL DOLT_ADD('.')",
		"CALL DOLT_COMMIT('-m', 'test: seed native task identity', '--author', 'gascity-test <test@gascity.local>')")
	_, port, _, cleanup := startPasswordedDoltServer(t, filepath.Join(t.TempDir(), "fe"), setupQueries...)
	defer cleanup()
	// Environment projection first resolves the city endpoint, even for rig
	// work. Bind it to this fixture server so no managed-provider discovery is
	// needed. Leave hq unprovisioned: accidentally opening city work must fail.
	if err := os.MkdirAll(filepath.Join(cityPath, ".beads"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := contract.EnsureCanonicalMetadata(fsys.OSFS{}, filepath.Join(cityPath, ".beads", "metadata.json"), contract.MetadataState{
		Database: "dolt", Backend: "dolt", DoltMode: "server", DoltDatabase: "hq",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ensureCanonicalScopeConfigState(fsys.OSFS{}, cityPath, contract.ConfigState{
		IssuePrefix: "gc", EndpointOrigin: contract.EndpointOriginCityCanonical,
		EndpointStatus: contract.EndpointStatusVerified, DoltHost: "127.0.0.1",
		DoltPort: strconv.Itoa(port), DoltUser: "root", DoltMode: "server",
	}); err != nil {
		t.Fatal(err)
	}
	if err := contract.WriteProjectIdentity(fsys.OSFS{}, rigPath, projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := contract.EnsureCanonicalMetadata(fsys.OSFS{}, filepath.Join(rigPath, ".beads", "metadata.json"), contract.MetadataState{
		Database: "dolt", Backend: "dolt", DoltMode: "server", DoltDatabase: "fe",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ensureCanonicalScopeConfigState(fsys.OSFS{}, rigPath, contract.ConfigState{
		IssuePrefix: "fe", EndpointOrigin: contract.EndpointOriginExplicit,
		EndpointStatus: contract.EndpointStatusVerified, DoltHost: "127.0.0.1",
		DoltPort: strconv.Itoa(port), DoltUser: "root", DoltMode: "server",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigPath, ".beads", ".env"), []byte("BEADS_DOLT_PASSWORD=secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BEADS_DOLT_PASSWORD", "secret")
	t.Setenv("GC_BEADS", "bd")
	t.Setenv("GC_CITY", cityPath)
	t.Setenv("GC_CITY_PATH", cityPath)
	cfg, err := loadCityConfig(cityPath, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	env, err := nativeDoltOpenEnvForScope(cityPath, cfg, rigPath)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := beads.OpenNativeStorage(context.Background(), rigPath, env)
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.SetConfig(context.Background(), "issue_prefix", "fe"); err != nil {
		_ = storage.Close()
		t.Fatal(err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}

	// Only the optional bd context/help surface is unavailable. Database
	// identity, schema, native opens, transactions and persistence remain real.
	previousRunner := beadsExecCommandRunnerWithEnv
	beadsExecCommandRunnerWithEnv = func(map[string]string) beads.CommandRunner {
		return func(string, string, ...string) ([]byte, error) {
			return nil, errors.New("fixture bd has no conditional-write CLI")
		}
	}
	t.Cleanup(func() { beadsExecCommandRunnerWithEnv = previousRunner })
	access := attemptWorkStoreAccess(cityPath, cfg)
	var id, policy, token string
	var owned *beads.NativeDoltStore
	err = access("rig:frontend", func(s beads.Store) error {
		base, _, _ := unwrapBeadPolicyStore(s)
		var ok bool
		owned, ok = base.(*beads.NativeDoltStore)
		if !ok {
			return errors.New("attempt selected non-native store despite eligible configured Dolt")
		}
		b, err := s.Create(beads.Bead{Title: "one bounded native start", Metadata: map[string]string{"sibling": "preserved"}})
		if err != nil {
			return err
		}
		id = b.ID
		policy, err = taskattempt.Request("rig:frontend", id, "frontend/worker")
		if err != nil {
			return err
		}
		if err := s.SetMetadata(id, beadmeta.NativeAttemptMetadataKey, policy); err != nil {
			return err
		}
		token, err = taskattempt.Reserve(s, id, "rig:frontend", "frontend/worker")
		return err
	})
	if err != nil || token == "" {
		t.Fatalf("configured native reservation: token=%q error=%v", token, err)
	}
	if _, err := owned.Get(id); err == nil {
		t.Fatal("owned native handle remained open after callback")
	}
	// A fresh resolver/handle must see the durable reservation, not the
	// session store or an in-process counter. It may authorize one start only.
	access = attemptWorkStoreAccess(cityPath, cfg)
	if err := access("rig:frontend", func(s beads.Store) error {
		_, err := taskattempt.Reserve(s, id, "rig:frontend", "frontend/worker")
		return err
	}); !errors.Is(err, taskattempt.ErrRefused) {
		t.Fatalf("reopened reservation: %v", err)
	}
	for i := range 2 {
		err := attemptWorkStoreAccess(cityPath, cfg)("rig:frontend", func(s beads.Store) error {
			return taskattempt.Start(s, id, "rig:frontend", "frontend/worker", token, "synthetic-session")
		})
		if i == 0 && err != nil || i == 1 && !errors.Is(err, taskattempt.ErrRefused) {
			t.Fatalf("start %d: %v", i+1, err)
		}
	}
	var startedPolicy string
	if err := access("rig:frontend", func(s beads.Store) error {
		b, err := s.Get(id)
		if err == nil && b.Metadata["sibling"] != "preserved" {
			return errors.New("metadata CAS changed sibling key")
		}
		startedPolicy = b.Metadata[beadmeta.NativeAttemptMetadataKey]
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// A broken scope identity must not bypass native preflight or acquire a
	// weaker writer. The fallback cannot execute even a single-key CAS.
	if err := contract.WriteProjectIdentity(fsys.OSFS{}, rigPath, "wrong-project"); err != nil {
		t.Fatal(err)
	}
	// Native preflight compares the generated metadata identity with the
	// database. Changing only the tracked TOML would not change that input.
	if _, err := contract.EnsureCanonicalMetadata(fsys.OSFS{}, filepath.Join(rigPath, ".beads", "metadata.json"), contract.MetadataState{
		Database: "dolt", Backend: "dolt", DoltMode: "server", DoltDatabase: "fe",
	}); err != nil {
		t.Fatal(err)
	}
	preflight, err := newBeadsPreflightChecker(cityPath, "bd").Check(rigPath)
	if err != nil || preflight.NativeStoreEligible {
		t.Fatalf("identity-mismatch fixture did not refuse native preflight: %+v, %v", preflight, err)
	}
	err = access("rig:frontend", func(s beads.Store) error {
		base, _, _ := unwrapBeadPolicyStore(s)
		if _, ok := base.(*beads.NativeDoltStore); ok {
			t.Fatal("identity mismatch bypassed native preflight")
		}
		w, ok := beads.MetadataCASWriterFor(s)
		if !ok {
			return beads.ErrConditionalWriteUnsupported
		}
		changed, err := w.CompareAndSetMetadataKey(id, beadmeta.NativeAttemptMetadataKey, startedPolicy, "forbidden")
		if changed {
			t.Fatal("unsupported fallback mutated task metadata")
		}
		return err
	})
	if !errors.Is(err, beads.ErrConditionalWriteUnsupported) {
		t.Fatalf("unsupported fallback: %v", err)
	}
	if err := contract.WriteProjectIdentity(fsys.OSFS{}, rigPath, projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := contract.EnsureCanonicalMetadata(fsys.OSFS{}, filepath.Join(rigPath, ".beads", "metadata.json"), contract.MetadataState{
		Database: "dolt", Backend: "dolt", DoltMode: "server", DoltDatabase: "fe",
	}); err != nil {
		t.Fatal(err)
	}
	callbackErr := errors.New("fixture callback refusal")
	err = access("rig:frontend", func(s beads.Store) error {
		base, _, _ := unwrapBeadPolicyStore(s)
		owned = base.(*beads.NativeDoltStore)
		b, err := s.Get(id)
		if err != nil {
			return err
		}
		if b.Metadata[beadmeta.NativeAttemptMetadataKey] != startedPolicy || b.Metadata["sibling"] != "preserved" {
			return errors.New("refused preflight changed task metadata")
		}
		return callbackErr
	})
	if !errors.Is(err, callbackErr) {
		t.Fatalf("callback error not preserved: %v", err)
	}
	if _, err := owned.Get(id); err == nil {
		t.Fatal("owned native handle remained open after callback error")
	}
}

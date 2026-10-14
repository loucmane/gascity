//go:build integration

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/session"
)

// TestProxiedNativeClaimCAS exercises the production factory and policy wrapper
// with a disposable bd-owned proxy. Run on the host with the CAS backport bd.
func TestProxiedNativeClaimCAS(t *testing.T) {
	if os.Getenv("GC_TEST_PROXIED_CAS") != "1" {
		t.Skip("set GC_TEST_PROXIED_CAS=1 and GC_ACCEPTANCE_BD_BIN with real dolt on PATH")
	}
	// testscript.Main prepends its fake bd to PATH before m.Run. An explicit
	// binary is required so this opt-in test cannot pass against that double
	// or silently choose a host bd without the revision-CAS backport.
	bd := strings.TrimSpace(os.Getenv("GC_ACCEPTANCE_BD_BIN"))
	if !filepath.IsAbs(bd) || isTestscriptShim(bd) {
		t.Fatal("set GC_ACCEPTANCE_BD_BIN to the absolute path of the real bd with --if-revision support; PATH contains the testscript bd shim")
	}
	bd, err := exec.LookPath(bd)
	if err != nil {
		t.Fatalf("GC_ACCEPTANCE_BD_BIN: %v", err)
	}
	t.Logf("CAS bd binary: %s", bd)
	if _, err := exec.LookPath("dolt"); err != nil {
		t.Fatal(err)
	}
	configureIsolatedRuntimeEnv(t)
	for _, entry := range gcBeadsBdTestHomeEnv(t) {
		key, value, _ := strings.Cut(entry, "=")
		t.Setenv(key, value)
	}
	if err := writeTestDoltIdentity(os.Getenv("HOME")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOLT_ROOT_PATH", os.Getenv("HOME"))
	t.Setenv("GC_BEADS", "bd")
	t.Setenv("GC_BEADS_FORCE_FALLBACK", "0")
	t.Setenv("GC_BEADS_PROXIED_NATIVE", "")
	t.Setenv("BD_BIN", bd)
	scope := t.TempDir()
	t.Setenv("BEADS_DIR", filepath.Join(scope, ".beads"))
	t.Setenv("BEADS_DOLT_PROXIED_SERVER", "1")
	t.Setenv("BEADS_DOLT_SHARED_SERVER", "0")
	t.Setenv("BEADS_DOLT_SERVER_MODE", "0")
	t.Setenv("BEADS_DOLT_SERVER_PORT", "")
	t.Setenv("BEADS_DOLT_PORT", "")
	runPackReleaseGit(t, scope, "init", "-q")
	if err := os.WriteFile(filepath.Join(scope, "city.toml"), []byte("[workspace]\nname = \"vfug\"\n[beads]\nprovider = \"bd\"\nconditional_writes = \"require\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		runRawBDFromDir(t, bd, scope, "dolt", "stop")
	})
	runRawBDFromDir(t, bd, scope, "init", "--proxied-server", "--prefix", "vfug", "--quiet")
	runRawBDFromDir(t, bd, scope, "config", "set", "types.custom", "session")
	result, err := openStoreResultAtForCity(scope, scope)
	if err != nil {
		t.Fatal(err)
	}
	store := result.Store
	t.Cleanup(func() {
		if err := closeBeadStoreHandle(store); err != nil {
			t.Error(err)
		}
	})
	view, ok := beads.ProxiedStoreFrom(store)
	if !ok || view.Report().Demoted || result.Diagnostic.Proxied == nil {
		t.Fatalf("factory did not select native proxied reads: %+v", result.Diagnostic)
	}
	work, err := store.Create(beads.Bead{Title: "work fence", Type: "task"})
	if err != nil {
		t.Fatal(err)
	}
	live, err := store.Get(work.ID)
	if err != nil || live.Revision == 0 {
		t.Fatalf("native read after CLI create: %+v, %v", live, err)
	}
	writer, ok := beads.RequiredConditionalWriterFor(store)
	if !ok {
		t.Fatal("policy wrapper lost required conditional writer")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, attempt := range []string{"first", "second"} {
		wg.Go(func() {
			results <- writer.UpdateIfMatch(work.ID, live.Revision, beads.UpdateOpts{Metadata: map[string]string{"gc.claim_attempt": attempt}})
		})
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			wins++
		case beads.IsPreconditionFailed(err):
			conflicts++
		default:
			t.Fatalf("CAS contender: %v", err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("CAS wins/conflicts = %d/%d, want 1/1", wins, conflicts)
	}
	if got, err := store.Get(work.ID); err != nil || got.Revision == live.Revision || got.Metadata["gc.claim_attempt"] == "" {
		t.Fatalf("native read after CLI CAS: %+v, %v", got, err)
	}
	row, err := store.Create(beads.Bead{Title: "session claim", Type: session.BeadType, Metadata: map[string]string{
		"state": "active", "instance_token": "fixture-token", "generation": "7",
		"session_name": "runtime-own", "template": "rig/worker", "alias": "rig/worker-1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	front := session.NewStore(beads.SessionStore{Store: store})
	verified, err := front.VerifySession(row.ID, "fixture-token", 7)
	if err != nil {
		t.Fatal(err)
	}
	if err := front.CompareAndSetCurrentClaim(verified, "", work.ID); err != nil {
		t.Fatal(err)
	}
	if err := front.CompareAndSetCurrentClaim(verified, "", "other"); !errors.Is(err, session.ErrClaimPrecondition) {
		t.Fatalf("conflicting session claim: %v", err)
	}
	if err := front.CompareAndSetCurrentClaim(verified, work.ID, ""); err != nil {
		t.Fatal(err)
	}
	if claim, err := front.CurrentClaimVerified(verified); err != nil || claim != "" {
		t.Fatalf("read after claim clear: %q, %v", claim, err)
	}
}

package sling

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/runtime"
)

// TestAttachFormulaToBeadEntryShapes exercises the two attachment entry points
// that share attachFormulaToBead — --on-formula and default-formula — and
// pins the per-path pieces the wrappers select: the sling method and the
// error-label prefix ("formula" vs "default formula"). This is the drift the
// S13 consolidation eliminated: before the merge these copies could diverge
// independently, so the test asserts both success method and error prefix for
// each entry shape.
func TestAttachFormulaToBeadEntryShapes(t *testing.T) {
	newDeps := func(t *testing.T) (SlingDeps, string) {
		t.Helper()
		cfg := &config.City{Workspace: config.Workspace{Name: "test"}}
		deps := testDeps(cfg, runtime.NewFake(), newFakeRunner().run)
		b, err := deps.Store.Create(beads.Bead{Title: "work", Type: "task", Status: "open"})
		if err != nil {
			t.Fatal(err)
		}
		return deps, b.ID
	}

	t.Run("on-formula success", func(t *testing.T) {
		deps, beadID := newDeps(t)
		a := config.Agent{Name: "mayor", MaxActiveSessions: intPtr(1)}
		result, err := DoSling(SlingOpts{Target: a, BeadOrFormula: beadID, OnFormula: "code-review"}, deps, deps.Store)
		if err != nil {
			t.Fatalf("DoSling on-formula: %v", err)
		}
		if result.Method != "on-formula" {
			t.Errorf("Method = %q, want on-formula", result.Method)
		}
		if result.FormulaName != "code-review" {
			t.Errorf("FormulaName = %q, want code-review", result.FormulaName)
		}
		if result.WispRootID == "" {
			t.Error("expected non-empty WispRootID")
		}
	})

	t.Run("default-formula success", func(t *testing.T) {
		deps, beadID := newDeps(t)
		a := config.Agent{Name: "mayor", MaxActiveSessions: intPtr(1), DefaultSlingFormula: stringPtr("code-review")}
		result, err := DoSling(SlingOpts{Target: a, BeadOrFormula: beadID}, deps, deps.Store)
		if err != nil {
			t.Fatalf("DoSling default-formula: %v", err)
		}
		if result.Method != "default-on-formula" {
			t.Errorf("Method = %q, want default-on-formula", result.Method)
		}
		if result.FormulaName != "code-review" {
			t.Errorf("FormulaName = %q, want code-review", result.FormulaName)
		}
		if result.WispRootID == "" {
			t.Error("expected non-empty WispRootID")
		}
	})

	t.Run("on-formula error label", func(t *testing.T) {
		deps, beadID := newDeps(t)
		a := config.Agent{Name: "mayor", MaxActiveSessions: intPtr(1)}
		_, err := DoSling(SlingOpts{Target: a, BeadOrFormula: beadID, OnFormula: "nonexistent-formula"}, deps, deps.Store)
		if err == nil {
			t.Fatal("expected instantiation error for nonexistent on-formula")
		}
		if want := `instantiating formula "nonexistent-formula" on`; !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want prefix %q", err.Error(), want)
		}
	})

	t.Run("default-formula error label", func(t *testing.T) {
		deps, beadID := newDeps(t)
		a := config.Agent{Name: "mayor", MaxActiveSessions: intPtr(1), DefaultSlingFormula: stringPtr("nonexistent-formula")}
		_, err := DoSling(SlingOpts{Target: a, BeadOrFormula: beadID}, deps, deps.Store)
		if err == nil {
			t.Fatal("expected instantiation error for nonexistent default formula")
		}
		if want := `instantiating default formula "nonexistent-formula" on`; !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want prefix %q", err.Error(), want)
		}
	})
}

// TestDoSlingDependencyCycleFamilies pins the plain-bead preflight contract:
// a parent waiting on its own child routes, a genuine single-family cycle is
// refused before any routing mutation, and dry-run and force still bypass the
// cycle check.
func TestDoSlingDependencyCycleFamilies(t *testing.T) {
	a := config.Agent{Name: "worker", MaxActiveSessions: intPtr(1)}
	parentWaitsOnChild := []beads.Dep{
		{IssueID: "parent", DependsOnID: "child", Type: "waits-for"},
		{IssueID: "child", DependsOnID: "parent", Type: "parent-child"},
	}
	withCycle := append(slices.Clone(parentWaitsOnChild),
		beads.Dep{IssueID: "child", DependsOnID: "sibling", Type: "blocks"},
		beads.Dep{IssueID: "sibling", DependsOnID: "child", Type: "blocks"},
	)
	setup := func(t *testing.T, parentStatus, parentAssignee string, edges []beads.Dep) (SlingDeps, *fakeRunner) {
		t.Helper()
		seed := []beads.Bead{
			{ID: "parent", Title: "parent", Type: "task", Status: parentStatus, Assignee: parentAssignee, Metadata: map[string]string{}},
			{ID: "child", Title: "child", Type: "task", Status: "open", Metadata: map[string]string{}},
			{ID: "sibling", Title: "sibling", Type: "task", Status: "open", Metadata: map[string]string{}},
		}
		store := beads.NewMemStoreFrom(0, seed, nil)
		for _, e := range edges {
			if err := store.DepAdd(e.IssueID, e.DependsOnID, e.Type); err != nil {
				t.Fatal(err)
			}
		}
		runner := newFakeRunner()
		deps := testDeps(&config.City{Workspace: config.Workspace{Name: "test-city"}}, runtime.NewFake(), runner.run)
		deps.Store = store
		return deps, runner
	}

	t.Run("parent waiting on child routes", func(t *testing.T) {
		deps, runner := setup(t, "open", "", parentWaitsOnChild)
		if _, err := DoSling(testOpts(a, "parent"), deps, deps.Store); err != nil {
			t.Fatalf("DoSling: %v", err)
		}
		if len(runner.calls) != 1 {
			t.Fatalf("got %d runner calls, want 1", len(runner.calls))
		}
	})

	t.Run("genuine cycle refused before mutation", func(t *testing.T) {
		// Reassign would reopen and unassign the bead; the refusal must precede it.
		deps, runner := setup(t, "in_progress", "someone", withCycle)
		opts := testOpts(a, "parent")
		opts.Reassign = true
		_, err := DoSling(opts, deps, deps.Store)
		var ce *CycleError
		if !errors.As(err, &ce) || !slices.Equal(ce.Path, []string{"child", "sibling", "child"}) {
			t.Fatalf("DoSling error = %v, want execution cycle child → sibling → child", err)
		}
		if len(runner.calls) != 0 {
			t.Errorf("runner calls = %v, want none", runner.calls)
		}
		got, err := deps.Store.Get("parent")
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != "in_progress" || got.Assignee != "someone" {
			t.Errorf("parent = %s/%q, want untouched in_progress/%q", got.Status, got.Assignee, "someone")
		}
	})

	t.Run("dry-run bypasses the cycle check", func(t *testing.T) {
		deps, runner := setup(t, "open", "", withCycle)
		opts := testOpts(a, "parent")
		opts.DryRun = true
		result, err := DoSling(opts, deps, deps.Store)
		if err != nil || !result.DryRun {
			t.Fatalf("DoSling dry-run = (DryRun %v, %v), want (true, nil)", result.DryRun, err)
		}
		if len(runner.calls) != 0 {
			t.Errorf("runner calls = %v, want none", runner.calls)
		}
	})

	t.Run("force bypasses the cycle check", func(t *testing.T) {
		deps, runner := setup(t, "open", "", withCycle)
		opts := testOpts(a, "parent")
		opts.Force = true
		if _, err := DoSling(opts, deps, deps.Store); err != nil {
			t.Fatalf("DoSling force: %v", err)
		}
		if len(runner.calls) != 1 {
			t.Fatalf("got %d runner calls, want 1", len(runner.calls))
		}
	})
}

// TestDoSlingDefaultFormulaFallsBackToPlainRouteWhenMoleculeAttached pins the
// REPAIR SCOPE for ga-ueugmi: a bare sling (no explicit --on/--formula) to a
// target whose config carries a default_sling_formula must not hard-fail when
// the bead already has a live attached molecule from an unrelated workflow --
// the caller never asked for a formula attach, so the implicit default should
// yield to plain routing (and warn) instead of blocking the sling and leaving
// gc.routed_to unset. An explicit --on/--formula request must keep
// hard-failing in this situation (TestOnFormulaExistingMoleculeErrors in
// cmd/gc/cmd_sling_test.go) -- only the implicit default-formula path falls
// back.
func TestDoSlingDefaultFormulaFallsBackToPlainRouteWhenMoleculeAttached(t *testing.T) {
	store := beads.NewMemStoreFrom(0, []beads.Bead{
		{ID: "BL-1", Type: "task", Status: "open", Assignee: "reviewer-session"},
		{ID: "MOL-1", Type: "molecule", Status: "open", ParentID: "BL-1"},
	}, nil)
	cfg := &config.City{Workspace: config.Workspace{Name: "test"}}
	runner := newFakeRunner()
	deps := testDeps(cfg, runtime.NewFake(), runner.run)
	deps.Store = store

	a := config.Agent{Name: "builder", MaxActiveSessions: intPtr(1), DefaultSlingFormula: stringPtr("code-review")}
	opts := SlingOpts{Target: a, BeadOrFormula: "BL-1", NoConvoy: true}

	result, err := DoSling(opts, deps, deps.Store)
	if err != nil {
		t.Fatalf("DoSling default-formula with attached molecule: expected no error (fallback to plain route), got %v", err)
	}
	if result.Method != "bead" {
		t.Errorf("Method = %q, want %q (fell back to plain bead routing)", result.Method, "bead")
	}
	if result.FormulaName != "" {
		t.Errorf("FormulaName = %q, want empty (formula was not attached)", result.FormulaName)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1 (plain route must still execute, so gc.routed_to gets set)", len(runner.calls))
	}

	var warned bool
	for _, w := range result.BeadWarnings {
		if strings.Contains(w, "MOL-1") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("BeadWarnings = %v, want a warning naming the skipped attachment MOL-1", result.BeadWarnings)
	}

	// Falling back to a plain route leaves BL-1 routed to builder but still
	// claimed by reviewer-session with no molecule to drive it -- nothing the
	// target queries can reach it. The fallback must say so distinctly
	// (gm-2kyaqy), not just report the skipped attachment.
	var undeliverable bool
	for _, w := range result.BeadWarnings {
		if strings.Contains(w, "still assigned to") && strings.Contains(w, "reviewer-session") {
			undeliverable = true
		}
	}
	if !undeliverable {
		t.Errorf("BeadWarnings = %v, want a warning that BL-1 is undeliverable to builder while reviewer-session holds it", result.BeadWarnings)
	}
}

// TestDoSlingDefaultFormulaFallsBackToPlainRouteWhenMoleculeAttachedGraphV2Formula
// extends the ga-ueugmi repair scope (see the test above) to the graph.v2
// branch of attachFormulaToBead: when the target's default_sling_formula
// resolves through the graph.v2 compiler (isGraph == true), a pre-existing
// non-workflow molecule/wisp attachment must fall back to plain routing the
// same way the legacy branch does, instead of hard-failing through
// CheckNoMoleculeChildrenAllowLiveWorkflow. It also pins that the synthetic
// input convoy prepareGraphV2FormulaInvocation mints before that check runs
// gets closed on the fallback path rather than leaked as an open,
// claim-attracting convoy (refs ga-9azvzg).
func TestDoSlingDefaultFormulaFallsBackToPlainRouteWhenMoleculeAttachedGraphV2Formula(t *testing.T) {
	dir := t.TempDir()
	content := "formula = \"graph-review\"\nversion = 1\ncontract = \"graph.v2\"\ntype = \"workflow\"\n\n[[steps]]\nid = \"work\"\ntitle = \"Work\"\n"
	if err := os.WriteFile(filepath.Join(dir, "graph-review.toml"), []byte(content), 0o644); err != nil {
		t.Fatalf("writing graph-review fixture: %v", err)
	}

	store := beads.NewMemStoreFrom(0, []beads.Bead{
		{ID: "BL-1", Type: "task", Status: "open", Assignee: "reviewer-session"},
		{ID: "MOL-1", Type: "molecule", Status: "open", ParentID: "BL-1"},
	}, nil)
	cfg := &config.City{
		Workspace:     config.Workspace{Name: "test"},
		FormulaLayers: config.FormulaLayers{City: []string{dir}},
	}
	runner := newFakeRunner()
	deps := testDeps(cfg, runtime.NewFake(), runner.run)
	deps.Store = store

	a := config.Agent{Name: "builder", MaxActiveSessions: intPtr(1), DefaultSlingFormula: stringPtr("graph-review")}
	opts := SlingOpts{Target: a, BeadOrFormula: "BL-1", NoConvoy: true}

	result, err := DoSling(opts, deps, deps.Store)
	if err != nil {
		t.Fatalf("DoSling default-formula (graph.v2) with attached molecule: expected no error (fallback to plain route), got %v", err)
	}
	if result.Method != "bead" {
		t.Errorf("Method = %q, want %q (fell back to plain bead routing)", result.Method, "bead")
	}
	if result.FormulaName != "" {
		t.Errorf("FormulaName = %q, want empty (formula was not attached)", result.FormulaName)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1 (plain route must still execute, so gc.routed_to gets set)", len(runner.calls))
	}

	var warned bool
	for _, w := range result.BeadWarnings {
		if strings.Contains(w, "MOL-1") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("BeadWarnings = %v, want a warning naming the skipped attachment MOL-1", result.BeadWarnings)
	}

	// Falling back to a plain route leaves BL-1 routed to builder but still
	// claimed by reviewer-session with no molecule to drive it -- nothing the
	// target queries can reach it. The fallback must say so distinctly
	// (gm-2kyaqy), not just report the skipped attachment.
	var undeliverable bool
	for _, w := range result.BeadWarnings {
		if strings.Contains(w, "still assigned to") && strings.Contains(w, "reviewer-session") {
			undeliverable = true
		}
	}
	if !undeliverable {
		t.Errorf("BeadWarnings = %v, want the graph.v2 fallback to warn that BL-1 is undeliverable (parity with the legacy branch)", result.BeadWarnings)
	}

	convoys, err := deps.Store.List(beads.ListQuery{Type: "convoy", IncludeClosed: true})
	if err != nil {
		t.Fatalf("List(convoy): %v", err)
	}
	if len(convoys) != 1 {
		t.Fatalf("convoys = %d, want 1 (the synthetic input convoy minted by prepareGraphV2FormulaInvocation)", len(convoys))
	}
	if convoys[0].Status != "closed" {
		t.Errorf("synthetic input convoy %s status = %q, want closed (fallback must close it, not leak it)", convoys[0].ID, convoys[0].Status)
	}
}

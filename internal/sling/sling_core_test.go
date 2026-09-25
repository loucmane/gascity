package sling

import (
	"errors"
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

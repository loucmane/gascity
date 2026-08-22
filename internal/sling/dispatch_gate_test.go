package sling

import (
	"errors"
	"testing"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/runtime"
)

func TestDoSlingDispatchGateRefusesBeforeAnyMutation(t *testing.T) {
	cfg := &config.City{
		Rigs: []config.Rig{{Name: "product", Prefix: "pd", Path: "/srv/product", ManagedProduct: true}},
	}
	runner := newFakeRunner()
	deps := testDeps(cfg, runtime.NewFake(), runner.run)
	deps.Store = seededStore("PD-42")
	gateErr := errors.New("managed product canary stale: permission_revision")
	var calls []string
	deps.DispatchGate = func(rigName string) error {
		calls = append(calls, rigName)
		return gateErr
	}

	_, err := DoSling(SlingOpts{
		Target:        config.Agent{Name: "worker", Dir: "product", MaxActiveSessions: intPtr(1)},
		BeadOrFormula: "PD-42",
	}, deps, deps.Store)
	if !errors.Is(err, gateErr) {
		t.Fatalf("DoSling error = %v, want gate error", err)
	}
	if len(calls) != 1 || calls[0] != "product" {
		t.Fatalf("gate calls = %v, want [product]", calls)
	}
	bead, getErr := deps.Store.Get("PD-42")
	if getErr != nil {
		t.Fatalf("Get: %v", getErr)
	}
	if len(bead.Metadata) != 0 || bead.Assignee != "" || bead.Status != "open" {
		t.Fatalf("bead mutated before refusal: %+v", bead)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %v, want none", runner.calls)
	}
}

func TestDoSlingWithoutConfiguredGatePreservesControlPlaneCompatibility(t *testing.T) {
	cfg := &config.City{Rigs: []config.Rig{{Name: "control", Prefix: "ct", Path: "/srv/control"}}}
	deps := testDeps(cfg, runtime.NewFake(), newFakeRunner().run)
	deps.Store = seededStore("CT-42")

	if _, err := DoSling(SlingOpts{
		Target:        config.Agent{Name: "worker", Dir: "control", MaxActiveSessions: intPtr(1)},
		BeadOrFormula: "CT-42",
	}, deps, deps.Store); err != nil {
		t.Fatalf("DoSling control compatibility: %v", err)
	}
}

func TestDoSlingBatchRunsDispatchGateExactlyOnce(t *testing.T) {
	cfg := &config.City{Rigs: []config.Rig{{Name: "product", Prefix: "pd", Path: "/srv/product", ManagedProduct: true}}}
	deps := testDeps(cfg, runtime.NewFake(), newFakeRunner().run)
	deps.Store = seededStore("PD-42")
	var calls int
	deps.DispatchGate = func(rigName string) error {
		calls++
		if rigName != "product" {
			t.Fatalf("gate rig = %q, want product", rigName)
		}
		return nil
	}
	if _, err := DoSlingBatch(SlingOpts{
		Target:        config.Agent{Name: "worker", Dir: "product", MaxActiveSessions: intPtr(1)},
		BeadOrFormula: "PD-42",
	}, deps, deps.Store); err != nil {
		t.Fatalf("DoSlingBatch: %v", err)
	}
	if calls != 1 {
		t.Fatalf("gate calls = %d, want 1", calls)
	}
}

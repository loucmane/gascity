//go:build integration

package t3bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/runtime/runtimetest"
)

func TestT3SeamConformance(t *testing.T) {
	var counter int64
	runtimetest.RunProviderTests(t, func(caseT *testing.T) (runtime.Provider, runtime.Config, string) {
		return NewSeamBacked(), t3ConformanceConfig(caseT), fmt.Sprintf("t3-contract-%d", atomic.AddInt64(&counter, 1))
	})
}

func t3ConformanceConfig(t *testing.T) runtime.Config {
	t.Helper()
	return runtime.Config{
		WorkDir: t.TempDir(), PromptSuffix: "synthetic contract startup",
		Env: map[string]string{"GC_AGENT": "contract-agent", "GC_TEMPLATE": "contract-agent", "GC_PROVIDER": "codex", "GC_MODEL": "synthetic-contract-model"},
	}
}

func TestT3PersistentStopRestartPreservesThread(t *testing.T) {
	p := NewSeamBacked()
	name, cfg := "t3-persistent-contract", t3ConformanceConfig(t)
	if err := p.Start(context.Background(), name, cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Stop(name); err != nil {
			t.Error(err)
		}
	})
	before := t3ContractServer.threadFor(t, name)
	if err := p.Stop(name); err != nil {
		t.Fatal(err)
	}
	if p.IsRunning(name) || p.ProcessAlive(name, nil) {
		t.Error("detached seam is still reported live")
	}
	listed, err := p.ListRunning(name)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(listed, name) {
		t.Error("detached seam is still listed")
	}
	raw := p.(*seamBackedProvider).raw
	if !raw.IsRunning(name) {
		t.Error("raw persistent T3 thread was stopped")
	}
	after := t3ContractServer.threadFor(t, name)
	if before["id"] != after["id"] || after["deletedAt"] != nil {
		t.Fatal("persistent thread was replaced or archived")
	}
	if err := p.Start(context.Background(), name, cfg); err != nil {
		t.Fatalf("reattach after Stop: %v", err)
	}
	if !p.IsRunning(name) {
		t.Error("reattached seam is not live")
	}
	raw.mu.Lock()
	_, watching := raw.watchers[name]
	raw.mu.Unlock()
	if !watching {
		t.Error("reattach did not restore event watcher")
	}
	count := t3ContractServer.commandCount()
	if err := p.Start(context.Background(), name, cfg); !errors.Is(err, runtime.ErrSessionExists) {
		t.Fatalf("duplicate after reattach: %v", err)
	}
	if t3ContractServer.commandCount() != count {
		t.Error("duplicate after reattach mutated thread")
	}
	if got := t3ContractServer.threadFor(t, name); got["id"] != before["id"] {
		t.Error("reattach changed thread identity")
	}
}

func TestT3PersistentTransitionRefusals(t *testing.T) {
	p := NewSeamBacked()
	name, cfg := "t3-transition-refusal", t3ConformanceConfig(t)
	if err := p.Start(context.Background(), name, cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		t3ContractServer.refuseState("")
		if err := p.Stop(name); err != nil {
			t.Error(err)
		}
	})
	raw := p.(*seamBackedProvider).raw
	var watcherCancelled atomic.Bool
	raw.mu.Lock()
	cancel, watching := raw.watchers[name]
	if watching {
		raw.watchers[name] = func() {
			watcherCancelled.Store(true)
			cancel()
		}
	}
	raw.mu.Unlock()
	if !watching {
		t.Fatal("Start did not register the event watcher")
	}
	t3ContractServer.refuseState("persistent-alive")
	if err := p.Stop(name); err == nil {
		t.Fatal("Stop accepted a refused detach")
	}
	if !p.IsRunning(name) {
		t.Fatal("failed detach hid a live session")
	}
	raw.mu.Lock()
	_, watching = raw.watchers[name]
	raw.mu.Unlock()
	if !watching || watcherCancelled.Load() {
		t.Fatal("refused detach cancelled or removed the existing event watcher")
	}
	t3ContractServer.refuseState("")
	if err := p.Stop(name); err != nil {
		t.Fatal(err)
	}
	if !watcherCancelled.Load() {
		t.Fatal("successful detach did not cancel the event watcher")
	}
	t3ContractServer.refuseState("active")
	if err := p.Start(context.Background(), name, cfg); err == nil {
		t.Fatal("Start accepted a refused reattach")
	}
	if p.IsRunning(name) {
		t.Fatal("failed reattach reported an active session")
	}
	t3ContractServer.refuseState("")
	if err := p.Start(context.Background(), name, cfg); err != nil {
		t.Fatalf("retry after proved metadata refusal: %v", err)
	}
}

func TestT3PersistentReadyRestartDoesNotStopThread(t *testing.T) {
	p := NewSeamBacked()
	name, cfg := "t3-ready-restart", t3ConformanceConfig(t)
	if err := p.Start(context.Background(), name, cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Stop(name); err != nil {
			t.Error(err)
		}
	})
	if err := p.Interrupt(name); err != nil {
		t.Fatal(err)
	}
	if err := p.Stop(name); err != nil {
		t.Fatal(err)
	}
	if err := p.Start(context.Background(), name, cfg); err != nil {
		t.Fatal(err)
	}
	thread := t3ContractServer.threadFor(t, name)
	if got := thread["session"].(map[string]interface{})["status"]; got != "ready" {
		t.Fatalf("reattach stopped an idle persistent runtime: status=%v, want ready", got)
	}
}

func TestT3AssignedStopTerminatesSession(t *testing.T) {
	p := NewSeamBacked()
	name, cfg := "t3-assigned-contract", t3ConformanceConfig(t)
	envelope := StartupEnvelope{
		GC:         GCSection{Agent: "contract-agent", Template: "contract-agent", SessionName: name},
		Runtime:    RuntimeSection{WorkDir: cfg.WorkDir, Provider: "codex", Model: "synthetic-contract-model"},
		Startup:    StartupSection{StartupPrompt: "synthetic assigned start"},
		Assignment: AssignmentSection{BeadID: "synthetic-contract-bead"},
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Env["GC_STARTUP_ENVELOPE"] = string(encoded)
	if err := p.Start(context.Background(), name, cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Stop(name); err != nil {
			t.Error(err)
		}
	})
	if err := p.Stop(name); err != nil {
		t.Fatal(err)
	}
	if p.IsRunning(name) || p.(*seamBackedProvider).raw.IsRunning(name) {
		t.Fatal("assigned session remains alive")
	}
	thread := t3ContractServer.threadFor(t, name)
	if got := thread["session"].(map[string]interface{})["status"]; got != "stopped" {
		t.Fatalf("assigned runtime status = %v", got)
	}
	listed, err := p.ListRunning(name)
	if err != nil || slices.Contains(listed, name) {
		t.Fatalf("assigned session still listed: %v, %v", listed, err)
	}
}

func TestT3DrainMetadataCannotReattachStoppedSeam(t *testing.T) {
	p := NewSeamBacked()
	name, cfg := "t3-drain-metadata", t3ConformanceConfig(t)
	if err := p.Start(context.Background(), name, cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Stop(name); err != nil {
			t.Error(err)
		}
	})
	if err := p.Stop(name); err != nil {
		t.Fatal(err)
	}
	if err := p.SetMeta(name, "GC_DRAIN", "1"); err != nil {
		t.Fatal(err)
	}
	if NewSeamBacked().IsRunning(name) {
		t.Error("drain metadata reattached a stopped seam")
	}
	if err := p.RemoveMeta(name, "GC_DRAIN"); err != nil {
		t.Fatal(err)
	}
	if NewSeamBacked().IsRunning(name) {
		t.Error("drain removal reattached a stopped seam")
	}
}

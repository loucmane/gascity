//go:build unix

package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/testutil"
)

type generatedTitleObservingStore struct {
	beads.Store
	generated chan struct{}
	once      sync.Once
}

func (s *generatedTitleObservingStore) Update(id string, opts beads.UpdateOpts) error {
	err := s.Store.Update(id, opts)
	if err == nil && opts.Title != nil && *opts.Title == "Generated Title" {
		s.once.Do(func() { close(s.generated) })
	}
	return err
}

func TestSupervisorShutdownCancelsAndJoinsNestedSessionTitleGeneration(t *testing.T) {
	dir := t.TempDir()
	controlFIFO := filepath.Join(dir, "control.fifo")
	startedFIFO := filepath.Join(dir, "started.fifo")
	if err := syscall.Mkfifo(controlFIFO, 0o600); err != nil {
		t.Fatalf("Mkfifo(control): %v", err)
	}
	if err := syscall.Mkfifo(startedFIFO, 0o600); err != nil {
		t.Fatalf("Mkfifo(started): %v", err)
	}

	script := filepath.Join(dir, "slow-title-provider")
	scriptBody := fmt.Sprintf("#!/bin/sh\nexec 3<%q\nprintf ready >%q\nexec cat <&3\n", controlFIFO, startedFIFO)
	if err := os.WriteFile(script, []byte(scriptBody), 0o755); err != nil {
		t.Fatalf("write slow title provider: %v", err)
	}

	providerStarted := make(chan error, 1)
	go func() {
		_, err := os.ReadFile(startedFIFO)
		providerStarted <- err
	}()
	bootstrap, err := os.OpenFile(controlFIFO, os.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatalf("open title-provider bootstrap FIFO: %v", err)
	}

	state := newSessionFakeState(t)
	observedStore := &generatedTitleObservingStore{
		Store:     state.cityBeadStore,
		generated: make(chan struct{}),
	}
	state.cityBeadStore = observedStore
	state.cfg.Workspace.Provider = "slow-title"
	state.cfg.Providers["slow-title"] = config.ProviderSpec{
		DisplayName: "Slow title",
		Command:     script,
		PathCheck:   script,
		PrintArgs:   []string{"--print"},
	}

	srv := New(state)
	srv.LookPathFunc = func(binary string) (string, error) { return binary, nil }
	if _, err := srv.humaCreateProviderSession(context.Background(), state.SessionsBeadStore(), sessionCreateBody{
		Kind:    "provider",
		Name:    "test-agent",
		Message: "a title request that must not escape shutdown",
	}, "test-agent"); err != nil {
		t.Fatalf("humaCreateProviderSession: %v", err)
	}
	select {
	case err := <-providerStarted:
		if err != nil {
			t.Fatalf("wait for title provider start: %v", err)
		}
	case <-time.After(testutil.GoroutineRaceTimeout):
		t.Fatal("timed out waiting for slow title provider")
	}
	keeper, err := os.OpenFile(controlFIFO, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		bootstrap.Close() //nolint:errcheck
		t.Fatalf("open title-provider keeper FIFO: %v", err)
	}
	bootstrap.Close() //nolint:errcheck

	shutdownCtx, cancel := context.WithTimeout(context.Background(), testutil.GoroutineRaceTimeout)
	defer cancel()
	if err := srv.shutdownBackground(shutdownCtx); err != nil {
		t.Fatalf("shutdownBackground: %v", err)
	}

	_, err = keeper.WriteString("Generated Title\n")
	keeper.Close() //nolint:errcheck
	if err == nil {
		select {
		case <-observedStore.generated:
			t.Fatal("title generation mutated the city store after Server shutdown joined its tracked tasks")
		case <-time.After(testutil.GoroutineRaceTimeout):
			t.Fatal("escaped title provider stayed live after release without publishing its generated title")
		}
	}
	if !errors.Is(err, syscall.EPIPE) {
		t.Fatalf("write title-provider FIFO after shutdown: %v, want no live reader", err)
	}
}

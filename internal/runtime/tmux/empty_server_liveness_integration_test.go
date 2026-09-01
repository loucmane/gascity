//go:build integration

package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	gcruntime "github.com/gastownhall/gascity/internal/runtime"
)

func TestProviderCloseSession_RealServerLifecycle(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	newProvider := func(t *testing.T, suffix string) *Provider {
		t.Helper()
		cfg := DefaultConfig()
		// Keep the full Unix-domain socket path comfortably below sockaddr_un's
		// platform limit; testSocketName already carries a long nanosecond suffix.
		cfg.SocketName = fmt.Sprintf("gctc-%s-%d-%d", suffix, os.Getpid(), time.Now().UnixNano())
		socketPath := namedSocketPath(cfg.SocketName)
		if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("remove prior socket %q: %v", socketPath, err)
		}
		p := NewProviderWithConfig(cfg)
		t.Cleanup(func() {
			_ = p.TeardownServer()
			_ = os.Remove(socketPath)
		})
		return p
	}

	t.Run("last-session-retires-server", func(t *testing.T) {
		p := newProvider(t, "last")
		if err := p.tm.NewSessionWithCommand("reviewer", t.TempDir(), "sleep 300"); err != nil {
			t.Fatalf("NewSessionWithCommand: %v", err)
		}
		if err := p.tm.ConfigureServer(); err != nil {
			t.Fatalf("ConfigureServer: %v", err)
		}

		if err := p.CloseSession("reviewer"); err != nil {
			t.Fatalf("CloseSession: %v", err)
		}
		if _, err := p.tm.run("display-message", "-p", "#{pid}"); !errors.Is(err, ErrNoServer) {
			t.Fatalf("server observation after last close = %v, want ErrNoServer", err)
		}
	})

	t.Run("remaining-session-preserves-server", func(t *testing.T) {
		p := newProvider(t, "sibling")
		for _, name := range []string{"reviewer", "sibling"} {
			if err := p.tm.NewSessionWithCommand(name, t.TempDir(), "sleep 300"); err != nil {
				t.Fatalf("NewSessionWithCommand(%s): %v", name, err)
			}
		}
		if err := p.tm.ConfigureServer(); err != nil {
			t.Fatalf("ConfigureServer: %v", err)
		}
		serverPID, err := p.tm.run("display-message", "-p", "#{pid}")
		if err != nil {
			t.Fatalf("server pid before close: %v", err)
		}

		if err := p.CloseSession("reviewer"); err != nil {
			t.Fatalf("CloseSession: %v", err)
		}
		hasSibling, err := p.tm.HasSession("sibling")
		if err != nil || !hasSibling {
			t.Fatalf("sibling exists = %t, err = %v", hasSibling, err)
		}
		afterPID, err := p.tm.run("display-message", "-p", "#{pid}")
		if err != nil || afterPID != serverPID {
			t.Fatalf("server pid after close = %q, err = %v; want %q", afterPID, err, serverPID)
		}
		if got := mustExitEmpty(t, p.tm); got != "off" {
			t.Fatalf("exit-empty after sibling-preserving close = %q, want off", got)
		}
	})

	t.Run("already-absent-session-retires-empty-server", func(t *testing.T) {
		p := newProvider(t, "absent")
		if err := p.tm.NewSessionWithCommand("seed", t.TempDir(), "sleep 300"); err != nil {
			t.Fatalf("NewSessionWithCommand: %v", err)
		}
		if err := p.tm.ConfigureServer(); err != nil {
			t.Fatalf("ConfigureServer: %v", err)
		}
		if err := p.tm.KillSession("seed"); err != nil {
			t.Fatalf("KillSession(seed): %v", err)
		}

		if err := p.CloseSession("already-gone"); err != nil {
			t.Fatalf("CloseSession(already-gone): %v", err)
		}
		if _, err := p.tm.run("display-message", "-p", "#{pid}"); !errors.Is(err, ErrNoServer) {
			t.Fatalf("server observation after absent close = %v, want ErrNoServer", err)
		}
	})
}

// The ga-jnavd production shape, against a real tmux server: a city server
// configured with `exit-empty off` outlives its last session, and `list-panes
// -a` then exits non-zero with "no current target". The state cache must read
// that as an empty fleet, not as a runtime outage.
func TestStateCache_RealEmptyServerObservesEmptyFleet(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	cfg := DefaultConfig()
	cfg.SocketName = testSocketName + "-empty"
	tm := NewTmuxWithConfig(cfg)
	t.Cleanup(func() { _ = tm.TeardownServer() })

	const session = "gc-test-empty-server"
	if err := tm.NewSessionWithCommand(session, t.TempDir(), "sleep 300"); err != nil {
		t.Fatalf("NewSessionWithCommand: %v", err)
	}
	// exit-empty off is what keeps the server alive with zero sessions; without
	// it the server exits and the failure degenerates into plain ErrNoServer.
	if err := tm.ConfigureServer(); err != nil {
		t.Fatalf("ConfigureServer: %v", err)
	}

	fetcher := &tmuxFetcher{tm: tm}
	snap, err := fetcher.FetchState(context.Background())
	if err != nil {
		t.Fatalf("FetchState with one live session: %v", err)
	}
	if !snap.Sessions[session].Running {
		t.Fatalf("FetchState did not see the live session; sessions = %v", snap.Sessions)
	}

	if err := tm.KillSession(session); err != nil {
		t.Fatalf("KillSession: %v", err)
	}

	// kill-session completes on the server before the client returns, so the
	// very next observation must already see the empty-but-alive server.
	snap, err = fetcher.FetchState(context.Background())
	if err != nil {
		t.Fatalf("FetchState against an alive empty server: err = %v (ErrRuntimeUnavailable=%t), want a successful empty observation",
			err, errors.Is(err, gcruntime.ErrRuntimeUnavailable))
	}
	if snap.Sessions == nil {
		t.Fatal("FetchState Sessions = nil for an alive empty server, want an empty non-nil map")
	}
	if len(snap.Sessions) != 0 {
		t.Fatalf("FetchState Sessions = %v after the last session was killed, want empty", snap.Sessions)
	}

	// Recovery: a new session on the same server must be observed again with no
	// restart and no cache reset.
	if err := tm.NewSessionWithCommand(session, t.TempDir(), "sleep 300"); err != nil {
		t.Fatalf("NewSessionWithCommand after empty: %v", err)
	}
	snap, err = fetcher.FetchState(context.Background())
	if err != nil {
		t.Fatalf("FetchState after the server refilled: %v", err)
	}
	if !snap.Sessions[session].Running {
		t.Fatalf("session not observed after the server refilled; sessions = %v", snap.Sessions)
	}
}

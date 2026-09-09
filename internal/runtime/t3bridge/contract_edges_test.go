//go:build integration

package t3bridge

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gorilla/websocket"
)

// These are focused production-constructor contract regressions, not a claim
// that the full runtime conformance suite or a live T3 deployment is proved.
func TestT3SeamStartRejectsLiveDuplicate(t *testing.T) {
	for _, state := range []string{"ready", "running"} {
		t.Run(state, func(t *testing.T) {
			p, server, cfg := isolatedT3ContractFixture(t, state)
			if err := p.Start(context.Background(), "contract-active", cfg); !errors.Is(err, runtime.ErrSessionExists) {
				t.Fatalf("Start(live duplicate) = %v, want ErrSessionExists", err)
			}
			if got := server.commandTypes(); len(got) != 0 {
				t.Fatalf("duplicate Start mutated the existing thread: %v", got)
			}
		})
	}
}

func TestT3SeamStartMayReuseStoppedThread(t *testing.T) {
	p, server, cfg := isolatedT3ContractFixture(t, "none")
	if err := p.Start(context.Background(), "contract-active", cfg); err != nil {
		t.Fatalf("Start(stopped thread) = %v", err)
	}
	if got := server.commandTypes(); slices.Contains(got, "thread.create") {
		t.Fatalf("stopped reusable thread was replaced: %v", got)
	}
}

func TestT3SeamListExcludesNonRunningThreads(t *testing.T) {
	p, server, _ := isolatedT3ContractFixture(t, "ready")
	threads := server.snapshot["threads"].([]interface{})
	for _, state := range []string{"running", "none", "gone", "stopped", "error"} {
		threads = append(threads, map[string]interface{}{
			"id": "thread-" + state, "projectId": "project-contract",
			"customMetadata": map[string]interface{}{"gc.sessionName": "contract-" + state},
			"session":        map[string]interface{}{"status": state},
		})
	}
	// The fixture is completely populated before its first request; no server
	// goroutine and test goroutine access the snapshot concurrently.
	server.snapshot["threads"] = threads
	// Raw enumeration retains historical discovery semantics; only the seam
	// contract filters this inventory to running sessions.
	rawNames, err := p.(*seamBackedProvider).raw.ListRunning("contract-")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(rawNames)
	if want := []string{"contract-active", "contract-error", "contract-gone", "contract-none", "contract-running", "contract-stopped"}; !slices.Equal(rawNames, want) {
		t.Fatalf("raw ListRunning lost inventory rows: %v, want %v", rawNames, want)
	}
	got, err := p.ListRunning("contract-")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if want := []string{"contract-active", "contract-running"}; !slices.Equal(got, want) {
		t.Fatalf("ListRunning = %v, want only live sessions %v", got, want)
	}
	// Retain the existing, bounded startup grace for a not-yet-materialized
	// session, but do not turn an error/stopped state into a live session.
	for _, name := range []string{"contract-none", "contract-gone", "contract-error", "contract-stopped"} {
		p.(*seamBackedProvider).raw.setRecentStart(name, time.Now())
	}
	got, err = p.ListRunning("contract-")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if want := []string{"contract-active", "contract-gone", "contract-none", "contract-running"}; !slices.Equal(got, want) {
		t.Fatalf("ListRunning during startup grace = %v, want %v", got, want)
	}
}

func TestT3LegacyPersistentThreadRemainsAttached(t *testing.T) {
	p, server, cfg := isolatedT3ContractFixture(t, "ready")
	thread := server.snapshot["threads"].([]interface{})[0].(map[string]interface{})
	metadata := thread["customMetadata"].(map[string]interface{})
	metadata["gc.state"] = "persistent-alive"
	// The legacy thread deliberately has no gc.seamDetached marker. Populate
	// this state before the fixture serves any request.
	if !p.IsRunning("contract-active") {
		t.Fatal("legacy persistent thread was silently detached")
	}
	if err := p.Start(context.Background(), "contract-active", cfg); !errors.Is(err, runtime.ErrSessionExists) {
		t.Fatalf("legacy persistent duplicate = %v, want ErrSessionExists", err)
	}
	if commands := server.commandTypes(); len(commands) != 0 {
		t.Fatalf("legacy discovery or duplicate admission mutated thread: %v", commands)
	}
}

func isolatedT3ContractFixture(t *testing.T, state string) (runtime.Provider, *t3BridgeTestServer, runtime.Config) {
	t.Helper()
	root := t.TempDir()
	// The real constructor must never inspect or remove host legacy state.
	t.Setenv("HOME", root)
	t.Setenv("T3_HOME", filepath.Join(root, ".t3"))
	t.Setenv("GC_EXEC_STATE_DIR", filepath.Join(root, "legacy"))
	t.Setenv("GC_T3BRIDGE_STATE_DIR", filepath.Join(root, "native"))
	t.Setenv("T3_BEARER_TOKEN", "synthetic-contract-token")
	resetBridgeAuthCacheForTest(t)
	server := newT3BridgeTestServer(t, map[string]interface{}{
		"projects": []interface{}{map[string]interface{}{
			"id": "project-contract", "workspaceRoot": root,
		}},
		"threads": []interface{}{map[string]interface{}{
			"id": "thread-contract", "projectId": "project-contract",
			"customMetadata": map[string]interface{}{
				"gc.sessionName": "contract-active", "gc.agent": "contract-agent",
				"gc.startupTemplate": "contract-agent",
				"gc.startupWorkDir":  root, "gc.runtimeProvider": "codex",
				"gc.startupModel": "synthetic-contract-model",
			},
			"session": map[string]interface{}{"status": state},
		}},
	})
	t.Cleanup(server.Close)
	t.Setenv("T3_WS_URL", server.wsURL())
	endpoint, err := url.Parse(server.wsURL())
	if err != nil {
		t.Fatal(err)
	}
	// Even a fixture failure cannot fall back to the real T3 loopback ports.
	previous := websocket.DefaultDialer
	dialer := *previous
	dialer.Proxy = nil
	dialer.NetDial = nil
	dialer.NetDialTLSContext = nil
	dialer.NetDialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != endpoint.Host {
			return nil, fmt.Errorf("T3 contract fixture refuses external address %q", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	websocket.DefaultDialer = &dialer
	t.Cleanup(func() { websocket.DefaultDialer = previous })
	p := NewSeamBacked()
	t.Cleanup(func() { p.(*seamBackedProvider).raw.stopEventWatcher("contract-active") })
	return p, server, runtime.Config{
		WorkDir: root,
		Env: map[string]string{
			"GC_AGENT": "contract-agent", "GC_PROVIDER": "codex", "GC_MODEL": "synthetic-contract-model",
			"GC_TEMPLATE": "contract-agent",
		},
	}
}

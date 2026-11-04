package main

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/testutil/providerlaunch"
	"github.com/gorilla/websocket"
)

func TestDelegatedT3RegistryRetainsCity(t *testing.T) {
	for _, name := range []string{"t3bridge", "exec:/fixture/gc-session-t3"} {
		t.Run(name, func(t *testing.T) {
			f := providerlaunch.New(t, "codex")
			t.Setenv("GC_T3BRIDGE_STATE_DIR", t.TempDir())
			t.Setenv("GC_EXEC_STATE_DIR", t.TempDir())
			t.Setenv("T3_BEARER_TOKEN", "")
			t.Setenv("T3_WS_URL", "ws://127.0.0.1:1/ws")
			previous := websocket.DefaultDialer
			calls := 0
			websocket.DefaultDialer = &websocket.Dialer{NetDialContext: func(context.Context, string, string) (net.Conn, error) {
				calls++
				return nil, errors.New("fixture transport stop")
			}}
			t.Cleanup(func() { websocket.DefaultDialer = previous })
			sp, err := newSessionProviderForCityByName(nil, name, config.SessionConfig{}, "city", f.City)
			if err != nil {
				t.Fatal(err)
			}
			err = sp.Start(context.Background(), "approval-session", runtime.Config{Command: f.Stable, WorkDir: f.City})
			if err == nil || !strings.Contains(err.Error(), "provider allowlist") || !strings.Contains(err.Error(), "t3bridge") {
				t.Errorf("city-bound transport without caller env: %v", err)
			}
			if calls != 0 {
				t.Errorf("delegated registry launch reached %d transport calls", calls)
			}
			err = sp.Nudge("pre-adoption-thread", []runtime.ContentBlock{{Type: "text", Text: "continue"}})
			if err == nil || !strings.Contains(err.Error(), "provider allowlist") {
				t.Errorf("city-bound nudge lost authority: %v", err)
			}
			if calls != 0 {
				t.Errorf("delegated nudge reached %d transport calls", calls)
			}
		})
	}
}

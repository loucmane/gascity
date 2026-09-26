package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/runtime"
	sessionpkg "github.com/gastownhall/gascity/internal/session"
)

// ga-6umo: a handle rebuilt from a session record without a config-resolved
// runtime may be looked up (to observe, nudge or stop the session) but never
// launches the stored command.
func TestSessionByIDWithoutResolvedRuntimeRefusesLaunch(t *testing.T) {
	for _, tc := range []struct {
		name     string
		resolver SessionRuntimeResolver
	}{
		{name: "no resolver"},
		{name: "resolver returns nil", resolver: func(sessionpkg.Info, string, map[string]string) (*ResolvedRuntime, error) {
			return nil, nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := beads.NewMemStore()
			sp := runtime.NewFake()
			manager := sessionpkg.NewManagerWithOptions(store, sp)
			info, err := manager.CreateSession(context.Background(), sessionpkg.CreateOptions{BeadOnly: true, Template: "worker", Title: "Probe", Command: "sh -c evil", WorkDir: t.TempDir(), Provider: "stored-provider"})
			if err != nil {
				t.Fatalf("CreateBeadOnly: %v", err)
			}
			factory, err := NewFactory(FactoryConfig{Store: store, Provider: sp, ResolveSessionRuntime: tc.resolver})
			if err != nil {
				t.Fatalf("NewFactory: %v", err)
			}
			handle, err := factory.SessionByID(info.ID)
			if err != nil {
				t.Fatalf("SessionByID(%q): %v, want a handle", info.ID, err)
			}
			if err := handle.Start(context.Background()); !errors.Is(err, ErrHandleConfig) {
				t.Fatalf("Start error = %v, want %v", err, ErrHandleConfig)
			}
			for _, call := range sp.Calls {
				if call.Method == "Start" {
					t.Fatalf("runtime Start was called with %q", call.Config.Command)
				}
			}
		})
	}
}

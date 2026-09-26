package api

import (
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/session"
)

// ga-6umo: an unknown override key or a bogus stored transport must not make
// the API resume paths fall back to a bare command without the configured
// permission default.
func TestAPIResumeKeepsPermissionDefaultOnBadMetadata(t *testing.T) {
	fs := newSessionFakeState(t)
	claude := config.BuiltinProviders()["claude"]
	claude.PathCheck = "true"
	fs.cfg = &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Agents:    []config.Agent{{Name: "mayor", Provider: "claude"}},
		Providers: map[string]config.ProviderSpec{"claude": claude},
	}
	srv := New(fs)
	for _, tc := range []struct {
		name      string
		transport string
		metadata  map[string]string
	}{
		{"unknown override key", "", map[string]string{"template_overrides": `{"x":"1"}`}},
		{"bogus transport", "bogus", nil},
	} {
		info := session.Info{ID: "gc-1", Template: "mayor", Provider: "claude", Transport: tc.transport, Command: "claude"}
		runtimeCfg, err := srv.resolveWorkerSessionRuntimeWithMetadata(info, "", tc.metadata)
		if err != nil {
			t.Fatalf("%s: resolveWorkerSessionRuntimeWithMetadata: %v", tc.name, err)
		}
		if runtimeCfg == nil || !strings.Contains(runtimeCfg.Command, "--dangerously-skip-permissions") {
			t.Fatalf("%s: runtime = %+v, want the configured permission default", tc.name, runtimeCfg)
		}
	}
}

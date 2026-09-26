package workdir

import (
	"path/filepath"
	"testing"

	"github.com/gastownhall/gascity/internal/config"
)

// ga-6umo: pack routing lands beside the configured work_dir.

func TestPackSiblingRoot(t *testing.T) {
	city := t.TempDir()
	cases := []struct {
		configured string
		want       string
	}{
		{filepath.Join(city, ".gc", "workspaces", "worker"), filepath.Join(city, ".gc", "workspaces")},
		{filepath.Join(city, ".gc", "worktrees", "gascity", "builder"), filepath.Join(city, ".gc", "worktrees", "gascity")},
		{filepath.Join(city, ".gc", "worker"), ""}, // parent is the .gc directory
		{filepath.Join(city, "worker"), ""},        // parent is the city root
		{"/elsewhere/worktrees/worker", ""},        // parent outside the city
	}
	for _, tc := range cases {
		if got := PackSiblingRoot(city, tc.configured, nil); got != tc.want {
			t.Errorf("PackSiblingRoot(%q) = %q, want %q", tc.configured, got, tc.want)
		}
	}
}

func TestSessionLaunchWorkDirAcceptsPackSibling(t *testing.T) {
	for _, key := range []string{"GC_WORKTREES_DIR", "T3CODE_WORKTREES_DIR", "T3CODE_HOME"} {
		t.Setenv(key, "")
	}
	city := t.TempDir()
	a := config.Agent{Name: "worker", WorkDir: ".gc/workspaces/{{.AgentBase}}"}
	cfg := &config.City{Agents: []config.Agent{a}}
	pack := filepath.Join(city, ".gc", "workspaces", "gascity", "dip-42")
	if got, refused := SessionLaunchWorkDir(pack, city, cfg, &a, ""); refused || got != pack {
		t.Fatalf("SessionLaunchWorkDir(pack) = %q, refused=%v, want %q accepted", got, refused, pack)
	}
	outside := filepath.Join(city, "rigs", "other")
	want := filepath.Join(city, ".gc", "workspaces", "worker")
	if got, refused := SessionLaunchWorkDir(outside, city, cfg, &a, ""); !refused || got != want {
		t.Fatalf("SessionLaunchWorkDir(outside) = %q, refused=%v, want configured %q and refused", got, refused, want)
	}
}

// ga-6umo: a worker-writable agent_name must not move the containment roots.
func TestSafeAgentIdentity(t *testing.T) {
	a := config.Agent{Name: "ant", Dir: "myrig"}
	for _, tc := range []struct{ in, want string }{
		{"myrig/ant-adhoc-123", "myrig/ant-adhoc-123"},
		{"myrig/fenrir", "myrig/fenrir"},
		{"", "myrig/ant"},
		{"myrig/..", "myrig/ant"},
		{"x/..", "myrig/ant"},
		{"../../../../..", "myrig/ant"},
		{"other/ant", "myrig/ant"},
		{"myrig/a b", "myrig/ant"},
		{"myrig/.hidden", "myrig/ant"},
		{"myrig/ant/extra", "myrig/ant"},
	} {
		if got := SafeAgentIdentity("", a, nil, tc.in); got != tc.want {
			t.Errorf("SafeAgentIdentity(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	city := config.Agent{Name: "dog"}
	if got := SafeAgentIdentity("", city, nil, "x/.."); got != "dog" {
		t.Errorf("SafeAgentIdentity(city agent, x/..) = %q, want dog", got)
	}
}

func TestSessionLaunchWorkDirIgnoresAgentNameTraversal(t *testing.T) {
	for _, key := range []string{"GC_WORKTREES_DIR", "T3CODE_WORKTREES_DIR", "T3CODE_HOME"} {
		t.Setenv(key, "")
	}
	city := t.TempDir()
	dog := config.Agent{Name: "dog", WorkDir: ".gc/agents/{{.AgentBase}}"}
	cfg := &config.City{Agents: []config.Agent{dog}}
	gcDir := filepath.Join(city, ".gc")
	configured := filepath.Join(city, ".gc", "agents", "dog")
	// "x/.." would expand the template to <city>/.gc: the session must not
	// launch there, and the fallback is the configured dir of the real agent.
	for _, name := range []string{"x/..", "../../../../../../..", ".."} {
		got, refused := SessionLaunchWorkDir(gcDir, city, cfg, &dog, name)
		if !refused || got != configured {
			t.Errorf("agent_name %q: SessionLaunchWorkDir(.gc) = %q, refused=%v, want configured %q", name, got, refused, configured)
		}
		got, refused = SessionLaunchWorkDir("/", city, cfg, &dog, name)
		if !refused || got != configured {
			t.Errorf("agent_name %q: SessionLaunchWorkDir(/) = %q, refused=%v, want configured %q", name, got, refused, configured)
		}
	}
	// A work_dir template on {{.Agent}} with a deep traversal must not
	// widen the root to / either.
	w := config.Agent{Name: "w", WorkDir: "worktrees/{{.Agent}}"}
	if got, refused := SessionLaunchWorkDir("/home", city, &config.City{Agents: []config.Agent{w}}, &w, "../../../../../../../.."); !refused || got == "/home" {
		t.Errorf("{{.Agent}} traversal: SessionLaunchWorkDir(/home) = %q, refused=%v, want refused", got, refused)
	}
}

func TestUsableConfiguredRoot(t *testing.T) {
	city := t.TempDir()
	for _, tc := range []struct {
		root string
		want bool
	}{
		{filepath.Join(city, ".gc", "agents", "dog"), true},
		{city, true},
		{"/srv/worktrees", true},
		{filepath.Join(city, ".gc"), false},
		{filepath.Dir(city), false},
		{"/", false},
		{"relative/dir", false},
		{filepath.Join(city, "a b"), false},
	} {
		if got := UsableConfiguredRoot(city, tc.root); got != tc.want {
			t.Errorf("UsableConfiguredRoot(%q) = %v, want %v", tc.root, got, tc.want)
		}
	}
}

// ga-6umo round 4: a pack parent that holds a rig root would reach whole
// other rigs.
func TestPackSiblingRootRefusesParentHoldingRigs(t *testing.T) {
	city := t.TempDir()
	rigs := []config.Rig{{Name: "gascity", Path: "rigs/gascity"}, {Name: "other", Path: filepath.Join(city, "rigs", "other")}}
	if got := PackSiblingRoot(city, filepath.Join(city, "rigs", "gascity"), rigs); got != "" {
		t.Fatalf("PackSiblingRoot(rig root) = %q, want empty", got)
	}
	if got, want := PackSiblingRoot(city, filepath.Join(city, ".gc", "workspaces", "worker"), rigs), filepath.Join(city, ".gc", "workspaces"); got != want {
		t.Fatalf("PackSiblingRoot(workspace) = %q, want %q", got, want)
	}
}

// ga-6umo round 4: the resume fallback is the configured work_dir even when
// it cannot serve as a containment root, never the wider city root.
func TestSessionLaunchWorkDirFallsBackToConfiguredPathWithSpace(t *testing.T) {
	for _, key := range []string{"GC_WORKTREES_DIR", "T3CODE_WORKTREES_DIR", "T3CODE_HOME"} {
		t.Setenv(key, "")
	}
	city := t.TempDir()
	configured := filepath.Join(t.TempDir(), "My Repo")
	a := config.Agent{Name: "w", WorkDir: configured}
	got, refused := SessionLaunchWorkDir("/etc", city, &config.City{Agents: []config.Agent{a}}, &a, "")
	if !refused || got != configured {
		t.Fatalf("SessionLaunchWorkDir = %q, refused=%v, want configured %q", got, refused, configured)
	}
}

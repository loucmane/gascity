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
		if got := PackSiblingRoot(city, tc.configured); got != tc.want {
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

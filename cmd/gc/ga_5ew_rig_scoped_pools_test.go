package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/runtime"
)

func TestResolveSessionTemplate_QualifiedGenericRigScope(t *testing.T) {
	rigPath := filepath.Join(t.TempDir(), "blog")
	cfg := &config.City{
		Rigs: []config.Rig{{Name: "blog", Path: rigPath}},
		Agents: []config.Agent{{
			Name:  "builder",
			Scope: "rig",
		}},
	}

	got, ok := resolveSessionTemplate(cfg, "blog/builder", "")
	if !ok {
		t.Fatal("resolveSessionTemplate(blog/builder) failed for generic scope=rig template")
	}
	if got.QualifiedName() != "blog/builder" {
		t.Fatalf("QualifiedName() = %q, want blog/builder", got.QualifiedName())
	}
	if got.Dir != "blog" {
		t.Fatalf("Dir = %q, want blog", got.Dir)
	}

	workDir, err := resolveWorkDir(t.TempDir(), cfg, &got)
	if err != nil {
		t.Fatalf("resolveWorkDir: %v", err)
	}
	if workDir != rigPath {
		t.Fatalf("workDir = %q, want registered rig root %q", workDir, rigPath)
	}
}

func TestBuildDesiredState_GenericRigScopedPoolRecoversRoutedUnassigned(t *testing.T) {
	cityPath := t.TempDir()
	blogPath := filepath.Join(cityPath, "rigs", "blog")
	gasCityPath := filepath.Join(cityPath, "rigs", "gascity")
	for _, dir := range []string{blogPath, gasCityPath} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	minSessions, maxSessions := 0, 2
	cfg := &config.City{
		Workspace: config.Workspace{Name: "city"},
		Rigs: []config.Rig{
			{Name: "blog", Path: blogPath},
			{Name: "gascity", Path: gasCityPath},
		},
		Agents: []config.Agent{{
			Name:              "builder",
			Scope:             "rig",
			Provider:          "mock",
			MinActiveSessions: &minSessions,
			MaxActiveSessions: &maxSessions,
		}},
		Providers: map[string]config.ProviderSpec{"mock": {Command: "true"}},
	}
	cityStore := beads.NewMemStore()
	blogStore := beads.NewMemStore()
	gasCityStore := beads.NewMemStore()
	if _, err := blogStore.Create(beads.Bead{
		ID:       "blog-zcz",
		Title:    "Fullbleed implementation",
		Type:     "task",
		Status:   "open",
		Metadata: map[string]string{"gc.routed_to": "blog/builder"},
	}); err != nil {
		t.Fatalf("create routed bead: %v", err)
	}

	result := buildDesiredStateWithSessionBeads(
		"city",
		cityPath,
		time.Now().UTC(),
		cfg,
		runtime.NewFake(),
		cityStore,
		map[string]beads.Store{"blog": blogStore, "gascity": gasCityStore},
		&sessionBeadSnapshot{},
		nil,
		io.Discard,
	)

	if got := result.ScaleCheckCounts["blog/builder"]; got != 1 {
		t.Fatalf("ScaleCheckCounts[blog/builder] = %d, want 1; all=%v", got, result.ScaleCheckCounts)
	}
	if got := result.ScaleCheckCounts["gascity/builder"]; got != 0 {
		t.Fatalf("ScaleCheckCounts[gascity/builder] = %d, want 0; all=%v", got, result.ScaleCheckCounts)
	}
	if len(result.State) != 1 {
		t.Fatalf("desired sessions = %d, want 1; state=%v", len(result.State), result.State)
	}
	for _, params := range result.State {
		if params.TemplateName != "blog/builder" {
			t.Fatalf("desired TemplateName = %q, want blog/builder", params.TemplateName)
		}
		if params.WorkDir != blogPath {
			t.Fatalf("desired WorkDir = %q, want %q", params.WorkDir, blogPath)
		}
	}
}

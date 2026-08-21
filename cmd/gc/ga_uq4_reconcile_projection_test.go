package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/gastownhall/gascity/internal/hooks"
)

// TestPrepareTemplateResolutionKeepsManagedCodexRepairAfterRecurringOverlay
// reproduces the live ga-uq4 follow-up at 488da6df...: session-start staging
// repaired the managed commands, then the next desired-state tick reapplied the
// provider overlay during pre-fingerprint template preparation and restored
// the duplicated/PATH-dependent commands. The recurring reconciliation writer
// must leave the same repaired shape as the launch writer.
func TestPrepareTemplateResolutionKeepsManagedCodexRepairAfterRecurringOverlay(t *testing.T) {
	cityDir := t.TempDir()
	rigDir := t.TempDir()
	packOverlay := t.TempDir()
	hookPath := filepath.Join(rigDir, ".codex", "hooks.json")

	captured, err := os.ReadFile(filepath.Join("..", "..", "internal", "runtime", "tmux", "testdata", "ga_uq4_postprojection_hooks.json"))
	if err != nil {
		t.Fatalf("read captured post-projection fixture: %v", err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		t.Fatalf("filepath.Abs(executable): %v", err)
	}
	captured = bytes.ReplaceAll(captured, []byte("/opt/gascity/bin/gc"), []byte(executable))
	captured = bytes.ReplaceAll(captured, []byte("/srv/gascity/city"), []byte(cityDir))

	writeScaffoldFixture(t, hookPath, string(captured))
	if err := hooks.FinalizeProjectedCodexHooks(fsys.OSFS{}, cityDir, rigDir); err != nil {
		t.Fatalf("establish repaired pre-tick hooks: %v", err)
	}
	repaired, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read repaired hooks: %v", err)
	}

	projectedOverlay := filepath.Join(packOverlay, "per-provider", "codex", ".codex", "hooks.json")
	writeScaffoldFixture(t, projectedOverlay, string(captured))

	bp := &agentBuildParams{
		cityPath:        cityDir,
		workspace:       &config.Workspace{Provider: "codex"},
		providers:       config.BuiltinProviders(),
		lookPath:        func(string) (string, error) { return executable, nil },
		fs:              fsys.OSFS{},
		rigs:            []config.Rig{{Name: "blog", Path: rigDir}},
		packOverlayDirs: []string{packOverlay},
		stderr:          io.Discard,
	}
	agent := &config.Agent{Name: "builder", Dir: "blog", Provider: "codex"}
	prepareTemplateResolution(bp, agent, "blog/builder", io.Discard)

	projected, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read hooks after recurring template preparation: %v", err)
	}
	assertReconciledManagedCodexHooks(t, projected, executable, cityDir)
	if !bytes.Equal(projected, repaired) {
		t.Fatalf("recurring template preparation changed repaired hook bytes (live regression 488c075e... -> 488da6df...)")
	}
}

func assertReconciledManagedCodexHooks(t *testing.T, data []byte, executable, cityDir string) {
	t.Helper()
	managed := make([]string, 0, 4)
	for _, command := range gaUQ4CodexHookCommands(t, data) {
		if strings.Contains(command, "prime --hook") || strings.Contains(command, "handoff --auto") || strings.Contains(command, "nudge drain --inject") || strings.Contains(command, "mail check --inject") {
			managed = append(managed, command)
		}
	}
	if len(managed) != 4 {
		t.Fatalf("managed command count = %d, want 4: %q", len(managed), managed)
	}
	for _, command := range managed {
		if !strings.Contains(command, executable) {
			t.Errorf("managed command retained PATH-dependent gc: %q", command)
		}
		if !strings.Contains(command, "--city ") || !strings.Contains(command, cityDir) {
			t.Errorf("managed command lacks explicit city binding: %q", command)
		}
	}
}

func gaUQ4CodexHookCommands(t *testing.T, data []byte) []string {
	t.Helper()
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse hooks JSON: %v", err)
	}
	var commands []string
	var walk func(any)
	walk = func(value any) {
		switch node := value.(type) {
		case map[string]any:
			for key, child := range node {
				if key == "command" {
					if command, ok := child.(string); ok {
						commands = append(commands, command)
					}
					continue
				}
				walk(child)
			}
		case []any:
			for _, child := range node {
				walk(child)
			}
		}
	}
	walk(root)
	return commands
}

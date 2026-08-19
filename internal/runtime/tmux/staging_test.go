package tmux

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/gastownhall/gascity/internal/hooks"
	"github.com/gastownhall/gascity/internal/runtime"
)

// TestStageStartFilesKeepsManagedCodexRepairAsLastWriter is the ga-uq4
// regression captured from the Blog rig. The live transition was repaired SHA
// 488c075e... -> post-projection SHA 488da6df... while the Aegis command digest
// stayed c12b56c7.... The checked-in JSON is that post-projection shape with
// host paths sanitized: provider overlay projection merged legacy bare-gc
// handoff/drain/mail commands back beside a pinned SessionStart and duplicated
// the managed SessionStart entry.
func TestStageStartFilesKeepsManagedCodexRepairAsLastWriter(t *testing.T) {
	workDir := t.TempDir()
	cityDir := t.TempDir()
	packOverlay := t.TempDir()
	hookPath := filepath.Join(workDir, ".codex", "hooks.json")
	captured, err := os.ReadFile(filepath.Join("testdata", "ga_uq4_postprojection_hooks.json"))
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
	// The fixture sanitizes the live host paths. Rebind them to this exact test
	// binary and city so the managed-command classifier sees the same identity
	// relationship as the captured installed-binary run.
	captured = bytes.ReplaceAll(captured, []byte("/opt/gascity/bin/gc"), []byte(executable))
	captured = bytes.ReplaceAll(captured, []byte("/srv/gascity/city"), []byte(cityDir))
	writeTmuxScaffoldFixture(t, hookPath, string(captured))

	// Establish the pre-launch repaired state with the real managed installer.
	if err := hooks.Install(fsys.OSFS{}, cityDir, workDir, []string{"codex"}); err != nil {
		t.Fatalf("repair managed Codex hooks: %v", err)
	}
	repaired, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read repaired hooks: %v", err)
	}
	repairedAegis := aegisCommandsFromCodexHooks(t, repaired)

	// Re-apply the captured layer through the real local-session projection path.
	projectedOverlay := filepath.Join(packOverlay, "per-provider", "codex", ".codex", "hooks.json")
	writeTmuxScaffoldFixture(t, projectedOverlay, string(captured))
	if err := stageStartFiles(runtime.Config{
		WorkDir:             workDir,
		ProviderName:        "codex",
		ProviderOverlayName: "codex",
		PackOverlayDirs:     []string{packOverlay},
		Env:                 map[string]string{"GC_CITY_PATH": cityDir},
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("stageStartFiles: %v", err)
	}
	projected, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read projected hooks: %v", err)
	}

	if got := aegisCommandsFromCodexHooks(t, projected); !bytes.Equal(got, repairedAegis) {
		t.Fatalf("user-owned Aegis hooks changed across projection\nrepaired: %s\nprojected: %s", repairedAegis, got)
	}
	assertProjectedManagedCodexHooks(t, projected, cityDir)
	if !bytes.Equal(projected, repaired) {
		t.Fatalf("managed Codex repair was not the last writer: repaired SHA %x -> projected SHA %x (live capture 488c075e... -> 488da6df...)", sha256.Sum256(repaired), sha256.Sum256(projected))
	}
}

func aegisCommandsFromCodexHooks(t *testing.T, data []byte) []byte {
	t.Helper()
	commands := codexHookCommands(t, data)
	aegis := commands[:0]
	for _, command := range commands {
		if strings.Contains(command, "AEGIS_INVOKING_AGENT=codex") {
			aegis = append(aegis, command)
		}
	}
	sort.Strings(aegis)
	return []byte(strings.Join(aegis, "\n"))
}

func assertProjectedManagedCodexHooks(t *testing.T, data []byte, cityDir string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		t.Fatalf("filepath.Abs(executable): %v", err)
	}
	managed := make([]string, 0, 4)
	for _, command := range codexHookCommands(t, data) {
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

func codexHookCommands(t *testing.T, data []byte) []string {
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

func TestStageStartFilesSurfacesKiroPreservationWarning(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	packOverlay := t.TempDir()

	fallbackInstructions := filepath.Join(packOverlay, "per-provider", "kiro", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(fallbackInstructions), 0o755); err != nil {
		t.Fatalf("mkdir Kiro overlay: %v", err)
	}
	if err := os.WriteFile(fallbackInstructions, []byte("fallback instructions"), 0o644); err != nil {
		t.Fatalf("write Kiro fallback instructions: %v", err)
	}
	projectInstructions := filepath.Join(workDir, "AGENTS.md")
	if err := os.WriteFile(projectInstructions, []byte("project instructions"), 0o600); err != nil {
		t.Fatalf("write project instructions: %v", err)
	}

	var warnings bytes.Buffer
	err := stageStartFiles(runtime.Config{
		WorkDir:         workDir,
		ProviderName:    "kiro",
		PackOverlayDirs: []string{packOverlay},
	}, &warnings)
	if err != nil {
		t.Fatalf("stageStartFiles: %v", err)
	}
	if got := warnings.String(); !strings.Contains(got, "overlay: preserving existing") || !strings.Contains(got, "AGENTS.md") {
		t.Fatalf("warnings = %q, want Kiro preservation warning", got)
	}
	data, err := os.ReadFile(projectInstructions)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if string(data) != "project instructions" {
		t.Fatalf("AGENTS.md = %q, want project instructions preserved", string(data))
	}
}

func TestStageStartFilesKeepsScaffoldOutOfSpawnerCWD(t *testing.T) {
	root := t.TempDir()
	sharedWorktree := filepath.Join(root, "shared-builder")
	beadSlug := "ga-ajw1no-1-as-a-maintainer-i-can-reproduce-stray-session-scaffold-leakage"
	leakedWorkDir := filepath.Join(sharedWorktree, beadSlug)
	workDir := filepath.Join(root, "city", ".gc", "worktrees", "gascity", "builder", beadSlug)
	packOverlay := filepath.Join(root, "city", "packs", "core", "overlay")

	writeTmuxScaffoldFixture(t, filepath.Join(packOverlay, ".claude", "skills", "triage", "SKILL.md"), "---\nname: triage\n---\n")
	writeTmuxScaffoldFixture(t, filepath.Join(packOverlay, ".codex", "hooks.json"), `{"hooks":{"SessionStart":[]}}`+"\n")
	writeTmuxScaffoldFixture(t, filepath.Join(packOverlay, ".gc", "settings.json"), "{}\n")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", workDir, err)
	}
	if err := os.MkdirAll(sharedWorktree, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", sharedWorktree, err)
	}
	t.Chdir(sharedWorktree)

	var warnings bytes.Buffer
	err := stageStartFiles(runtime.Config{
		WorkDir:             workDir,
		ProviderName:        "codex",
		ProviderOverlayName: "codex",
		PackOverlayDirs:     []string{packOverlay},
	}, &warnings)
	if err != nil {
		t.Fatalf("stageStartFiles: %v", err)
	}

	for _, rel := range []string{
		filepath.Join(".claude", "skills", "triage", "SKILL.md"),
		filepath.Join(".codex", "hooks.json"),
	} {
		if _, err := os.Stat(filepath.Join(workDir, rel)); err != nil {
			t.Errorf("target scaffold %s missing under workdir %q: %v", rel, workDir, err)
		}
	}
	// A top-level .gc/ in the overlay source is a runtime mirror and must never
	// be staged into a session workdir (overlay.skipRuntimeMirror). The session's
	// own .gc/settings.json is staged separately through the hook-file path, not
	// copied verbatim from the pack overlay.
	if _, err := os.Stat(filepath.Join(workDir, ".gc", "settings.json")); !os.IsNotExist(err) {
		t.Errorf("overlay .gc runtime mirror must not be staged under workdir %q (stat err = %v)", workDir, err)
	}
	if _, err := os.Stat(leakedWorkDir); err == nil {
		t.Fatalf("shared cwd contains stray bead-slug scaffold directory %q; scaffold must stay under %q", leakedWorkDir, workDir)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat leaked workdir %q: %v", leakedWorkDir, err)
	}
}

func writeTmuxScaffoldFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

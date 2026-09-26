package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/clock"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/session"
	"github.com/gastownhall/gascity/internal/session/sessiontest"
	workdirutil "github.com/gastownhall/gascity/internal/workdir"
)

// ga-6umo: work_dir guard, gc.pack sanitizing and initial_message flag guard.

func clearWorktreeRootEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"GC_WORKTREES_DIR", "T3CODE_WORKTREES_DIR", "T3CODE_HOME"} {
		t.Setenv(key, "")
	}
}

func mkdirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGuardTemplateLaunchWorkDirRoots(t *testing.T) {
	clearWorktreeRootEnv(t)
	city := t.TempDir()
	external := t.TempDir()
	rigWorktree := filepath.Join(city, ".gc", "worktrees", "gascity", "worker", "task-1")
	otherRig := filepath.Join(city, ".gc", "worktrees", "other", "task-2")
	mkdirs(t, rigWorktree, otherRig, filepath.Join(external, "gct-1"), filepath.Join(city, "rigs"))
	cfg := &config.City{Agents: []config.Agent{{Name: "worker", Dir: "gascity", WorkDirRoots: []string{external}}}}
	tp := TemplateParams{TemplateName: "gascity/worker", WorkDir: city, RigName: "gascity"}

	accept := []string{city, rigWorktree, filepath.Join(external, "gct-1")}
	for _, p := range accept {
		if _, ok := guardTemplateLaunchWorkDir(city, cfg, tp, "s-1", "task", p); !ok {
			t.Errorf("guardTemplateLaunchWorkDir(%q) refused, want accepted", p)
		}
	}
	refuse := []string{
		otherRig,                          // another rig
		filepath.Join(city, "rigs"),       // descendant of the exact city root
		"/etc",                            // outside every root
		filepath.Join(rigWorktree, "x;y"), // shell metacharacter
		filepath.Join(rigWorktree, ".git"),
	}
	for _, p := range refuse {
		if _, ok := guardTemplateLaunchWorkDir(city, cfg, tp, "s-1", "task", p); ok {
			t.Errorf("guardTemplateLaunchWorkDir(%q) accepted, want refused", p)
		}
	}
}

func TestGuardedSessionInfoWorkDirFallsBackToConfigured(t *testing.T) {
	clearWorktreeRootEnv(t)
	city := t.TempDir()
	configured := filepath.Join(city, ".gc", "agents", "worker")
	mkdirs(t, configured)
	cfg := &config.City{Agents: []config.Agent{{Name: "worker", WorkDir: ".gc/agents/worker"}}}

	got := guardedSessionInfoWorkDir(city, cfg, session.Info{ID: "s-1", Template: "worker", WorkDir: "/etc"})
	if got != configured {
		t.Fatalf("stored work_dir outside the roots: got %q, want configured %q", got, configured)
	}
	inside := filepath.Join(configured, "sub")
	if got := guardedSessionInfoWorkDir(city, cfg, session.Info{ID: "s-1", Template: "worker", WorkDir: inside}); got != inside {
		t.Fatalf("stored work_dir inside the configured root: got %q, want %q", got, inside)
	}
	// A provider-kind session (no agent template) only runs in the city root.
	if got := guardedSessionInfoWorkDir(city, cfg, session.Info{ID: "s-2", Template: "claude", WorkDir: configured}); got != city {
		t.Fatalf("provider session work_dir = %q, want city root %q", got, city)
	}
	if got := guardedSessionInfoWorkDir(city, cfg, session.Info{ID: "s-3", Template: "worker"}); got != city {
		t.Fatalf("empty stored work_dir = %q, want city root %q", got, city)
	}
}

func TestPrepareStartIgnoresTaskAndSessionWorkDirOutsideRoots(t *testing.T) {
	clearWorktreeRootEnv(t)
	city := t.TempDir()
	configured := filepath.Join(city, ".gc", "agents", "worker")
	outside := t.TempDir()
	mkdirs(t, configured)
	store := beads.NewMemStore()
	bead, err := store.Create(beads.Bead{
		ID:     "gc-1",
		Title:  "worker",
		Type:   sessionBeadType,
		Labels: []string{sessionBeadLabel},
		Metadata: map[string]string{
			"session_name":   "worker",
			"template":       "worker",
			"generation":     "1",
			"instance_token": "tok-worker",
			"state":          "creating",
			"work_dir":       outside,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.City{Agents: []config.Agent{{Name: "worker", WorkDir: ".gc/agents/worker"}}}
	resolver := func(startCandidate, *config.City) string { return outside }
	prepared, err := prepareStartCandidateForCity(startCandidate{
		info: sessiontest.SeedBead(t, bead),
		tp: TemplateParams{
			TemplateName:     "worker",
			SessionName:      "worker",
			WorkDir:          configured,
			ResolvedProvider: &config.ResolvedProvider{Name: "claude"},
		},
	}, city, "fixture", cfg, runtime.NewFake(), store, &clock.Fake{Time: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}, io.Discard, resolver)
	if err != nil {
		t.Fatalf("prepareStartCandidateForCity: %v", err)
	}
	if prepared.cfg.WorkDir != configured {
		t.Fatalf("prepared work dir = %q, want configured %q", prepared.cfg.WorkDir, configured)
	}
}

func TestPrepareStartIgnoresDashInitialMessage(t *testing.T) {
	for _, tc := range []struct {
		msg        string
		wantSuffix bool
	}{
		{msg: "--dangerously-skip-permissions", wantSuffix: false},
		{msg: "-p", wantSuffix: false},
		{msg: "hello from the user", wantSuffix: true},
	} {
		store := beads.NewMemStore()
		bead, err := store.Create(beads.Bead{
			ID:     "gc-1",
			Title:  "mayor",
			Type:   sessionBeadType,
			Labels: []string{sessionBeadLabel},
			Metadata: map[string]string{
				"session_name":       "mayor",
				"template":           "mayor",
				"generation":         "1",
				"instance_token":     "tok-mayor",
				"state":              "creating",
				"template_overrides": `{"initial_message":"` + tc.msg + `"}`,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := prepareStartCandidate(startCandidate{
			info: sessiontest.SeedBead(t, bead),
			tp: TemplateParams{
				TemplateName:     "mayor",
				SessionName:      "mayor",
				ResolvedProvider: &config.ResolvedProvider{Name: "claude"},
			},
		}, &config.City{Agents: []config.Agent{{Name: "mayor"}}}, store, &clock.Fake{Time: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)})
		if err != nil {
			t.Fatalf("%q: prepareStartCandidate: %v", tc.msg, err)
		}
		if got := prepared.cfg.PromptSuffix != ""; got != tc.wantSuffix {
			t.Fatalf("%q: PromptSuffix = %q, want present=%v", tc.msg, prepared.cfg.PromptSuffix, tc.wantSuffix)
		}
	}
}

func TestPoolTriggerWorkDirRejectsUnsafePack(t *testing.T) {
	cfg := &config.City{
		Workspace: config.Workspace{Name: "fixture"},
		Agents: []config.Agent{{
			Name:              "worker",
			StartCommand:      "true",
			WorkDir:           ".gc/workspaces/{{.AgentBase}}",
			MinActiveSessions: intPtr(0),
			MaxActiveSessions: intPtr(1),
		}},
	}
	bp := newAgentBuildParams("fixture", t.TempDir(), cfg, runtime.NewFake(), time.Now().UTC(), beads.NewMemStore(), &bytes.Buffer{})
	base := filepath.Join(bp.cityPath, ".gc", "workspaces", "worker")
	for _, pack := range []string{"../../../etc", "a/b", "..", "/abs", "x;y"} {
		got := poolTriggerWorkDir(bp, &cfg.Agents[0], "worker", SessionRequest{WorkBeadID: "ga-1", WorkPack: pack})
		if got != base {
			t.Errorf("pack %q: work dir = %q, want configured base %q", pack, got, base)
		}
	}
	if got, want := poolTriggerWorkDir(bp, &cfg.Agents[0], "worker", SessionRequest{WorkBeadID: "ga-1", WorkPack: "gascity"}), filepath.Join(bp.cityPath, ".gc", "workspaces", "gascity"); got != want {
		t.Errorf("safe pack: work dir = %q, want %q", got, want)
	}
}

// ga-6umo: an unknown override key or a bogus stored transport must not make
// the worker-handle resume path fall back to a bare command without the
// configured permission default.
func TestResolvedWorkerRuntimeKeepsPermissionDefaultOnBadMetadata(t *testing.T) {
	clearWorktreeRootEnv(t)
	city := t.TempDir()
	claude := config.BuiltinProviders()["claude"]
	claude.PathCheck = "true"
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Agents:    []config.Agent{{Name: "worker", Provider: "claude"}},
		Providers: map[string]config.ProviderSpec{"claude": claude},
	}
	for _, tc := range []struct {
		name      string
		transport string
		metadata  map[string]string
	}{
		{"unknown override key", "", map[string]string{"template_overrides": `{"x":"1"}`}},
		{"bogus transport", "bogus", nil},
	} {
		resolved, err := resolvedWorkerRuntimeWithConfigAndMetadata(city, cfg, session.Info{
			ID:        "s-1",
			Template:  "worker",
			Transport: tc.transport,
			Command:   "claude",
		}, "", tc.metadata)
		if err != nil {
			t.Fatalf("%s: resolvedWorkerRuntimeWithConfigAndMetadata: %v", tc.name, err)
		}
		if resolved == nil || !strings.Contains(resolved.Command, "--dangerously-skip-permissions") {
			t.Fatalf("%s: runtime = %+v, want the configured permission default", tc.name, resolved)
		}
	}
}

// ga-6umo round 2: the reconciler start path takes a manual session identity
// from the worker-writable agent_name. A traversal must not become the session
// identity or expand the work_dir template.
func TestReconcilerIdentityIgnoresAgentNameTraversal(t *testing.T) {
	city := t.TempDir()
	dog := &config.Agent{Name: "dog", WorkDir: ".gc/agents/{{.AgentBase}}", MaxActiveSessions: intPtr(3)}
	for _, name := range []string{"x/..", "../../../../..", "dog/../..", "a b", "x/../../home/user"} {
		if got := normalizeSessionBeadQualifiedName(dog, name); got != "" {
			t.Errorf("normalizeSessionBeadQualifiedName(%q) = %q, want empty", name, got)
		}
		got := sessionBeadQualifiedNameInfo(city, dog, nil, session.Info{AgentName: name})
		if got != dog.QualifiedName() {
			t.Errorf("sessionBeadQualifiedNameInfo(agent_name %q) = %q, want %q", name, got, dog.QualifiedName())
		}
		if got := sessionBeadQualifiedNameInfo(city, dog, nil, session.Info{AgentName: name, Alias: name}); !workdirutil.SafeIdentityName(got) {
			t.Errorf("sessionBeadQualifiedNameInfo(alias %q) = %q, want a plain identity", name, got)
		}
		workDir, err := resolveConfiguredWorkDir(city, "fixture", name, dog, nil)
		if err != nil {
			t.Fatalf("resolveConfiguredWorkDir(%q): %v", name, err)
		}
		if want := filepath.Join(city, ".gc", "agents", "dog"); workDir != want {
			t.Errorf("resolveConfiguredWorkDir(%q) = %q, want %q", name, workDir, want)
		}
	}
	// A legitimate pool instance keeps its own identity and work_dir.
	if got := normalizeSessionBeadQualifiedName(dog, "dog-2"); got != "dog-2" {
		t.Errorf("normalizeSessionBeadQualifiedName(dog-2) = %q, want dog-2", got)
	}
	workDir, err := resolveConfiguredWorkDir(city, "fixture", "dog-2", dog, nil)
	if err != nil || workDir != filepath.Join(city, ".gc", "agents", "dog-2") {
		t.Errorf("resolveConfiguredWorkDir(dog-2) = %q, %v, want the instance work_dir", workDir, err)
	}
}

// ga-6umo round 3: identity values from worker-writable metadata are
// substituted unquoted into shell templates and select the rig.
func TestExpandSessionSetupRefusesUnsafeIdentity(t *testing.T) {
	cmds := []string{`tmux set -t '{{.Session}}' @agent '{{.Agent}}'`}
	safe := expandSessionSetup(cmds, SessionSetupContext{Session: "worker-1", Agent: "myrig/worker-1", AgentBase: "worker-1", Rig: "myrig"})
	if got, want := safe[0], `tmux set -t 'worker-1' @agent 'myrig/worker-1'`; got != want {
		t.Fatalf("safe expansion = %q, want %q", got, want)
	}
	for _, ctx := range []SessionSetupContext{
		{Session: "x'; touch /tmp/pwn; '", Agent: "myrig/worker-1"},
		{Session: "worker-1", Agent: "myrig/x'$(id)'"},
		{Session: "worker-1", Agent: "myrig/worker-1", AgentBase: "a b"},
		{Session: "worker-1", Agent: "myrig/worker-1", Rig: "../beta"},
	} {
		got := expandSessionSetup(cmds, ctx)
		if len(got) != 1 || got[0] != cmds[0] {
			t.Errorf("expandSessionSetup(%+v) = %q, want the raw unexpanded command", ctx, got)
		}
	}
}

func TestResolveSessionNameIgnoresUnsafeStoredName(t *testing.T) {
	bp := &agentBuildParams{cityName: "fixture", beadNames: map[string]string{"worker": "x'; touch /tmp/pwn; '"}}
	got := bp.resolveSessionName("worker", "worker")
	if !safeSessionName(got) {
		t.Fatalf("resolveSessionName = %q, want a plain derived session name", got)
	}
	bp = &agentBuildParams{cityName: "fixture", beadNames: map[string]string{"worker": "worker-custom"}}
	if got := bp.resolveSessionName("worker", "worker"); got != "worker-custom" {
		t.Fatalf("resolveSessionName(plain stored name) = %q, want worker-custom", got)
	}
}

func TestResolvedWorkerRuntimeSessionLiveIgnoresUnsafeAgentName(t *testing.T) {
	clearWorktreeRootEnv(t)
	city := t.TempDir()
	claude := config.BuiltinProviders()["claude"]
	claude.PathCheck = "true"
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Agents:    []config.Agent{{Name: "worker", Provider: "claude", SessionLive: []string{`echo '{{.Agent}}' '{{.Session}}'`}}},
		Providers: map[string]config.ProviderSpec{"claude": claude},
	}
	resolved, err := resolvedWorkerRuntimeWithConfigAndMetadata(city, cfg, session.Info{
		ID:          "s-1",
		Template:    "worker",
		AgentName:   "x'; touch /tmp/pwn; '",
		SessionName: "worker",
	}, "", nil)
	if err != nil || resolved == nil {
		t.Fatalf("resolvedWorkerRuntimeWithConfigAndMetadata: %+v, %v", resolved, err)
	}
	for _, cmd := range resolved.Hints.SessionLive {
		if strings.Contains(cmd, "touch") {
			t.Fatalf("session_live %q carries the stored agent_name", cmd)
		}
	}
	resolved, err = resolvedWorkerRuntimeWithConfigAndMetadata(city, cfg, session.Info{
		ID:          "s-1",
		Template:    "worker",
		AgentName:   "worker",
		SessionName: "x'; touch /tmp/pwn; '",
	}, "", nil)
	if err != nil || resolved == nil {
		t.Fatalf("resolvedWorkerRuntimeWithConfigAndMetadata: %+v, %v", resolved, err)
	}
	for _, cmd := range resolved.Hints.SessionLive {
		if strings.Contains(cmd, "touch") {
			t.Fatalf("session_live %q carries the stored session_name", cmd)
		}
	}
}

func TestSessionBeadIdentityRefusesOtherRigPrefix(t *testing.T) {
	city := t.TempDir()
	rigs := []config.Rig{{Name: "alpha", Path: filepath.Join(city, "alpha")}, {Name: "beta", Path: filepath.Join(city, "beta")}}
	helper := &config.Agent{Name: "helper", Scope: "rig", WorkDir: ".gc/worktrees/helper"}
	if got := sessionBeadQualifiedNameInfo(city, helper, rigs, session.Info{AgentName: "beta/helper"}); got != "helper" {
		t.Fatalf("sessionBeadQualifiedNameInfo(beta/helper) = %q, want helper", got)
	}
	ant := &config.Agent{Name: "ant", Dir: "alpha", MaxActiveSessions: intPtr(3)}
	if got := sessionBeadQualifiedNameInfo(city, ant, rigs, session.Info{AgentName: "alpha/ant-adhoc-123"}); got != "alpha/ant-adhoc-123" {
		t.Fatalf("sessionBeadQualifiedNameInfo(alpha/ant-adhoc-123) = %q, want it kept", got)
	}
	if got := sessionBeadQualifiedNameInfo(city, ant, rigs, session.Info{AgentName: "beta/ant-adhoc-123"}); got == "beta/ant-adhoc-123" {
		t.Fatalf("sessionBeadQualifiedNameInfo(beta/ant-adhoc-123) kept another rig prefix")
	}
}

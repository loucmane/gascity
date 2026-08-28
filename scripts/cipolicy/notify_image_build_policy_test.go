package cipolicy

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNotifyImageBuildFailsClosedWithoutHostedToken(t *testing.T) {
	workflowPath := filepath.Join("..", "..", ".github", "workflows", "notify-image-build.yaml")
	workflow := readYAMLMap(t, workflowPath)

	permissions, ok := workflow["permissions"].(map[string]any)
	if !ok || len(permissions) != 0 {
		t.Fatalf("notify workflow permissions = %#v, want empty mapping", workflow["permissions"])
	}
	jobs, ok := workflow["jobs"].(map[string]any)
	if !ok {
		t.Fatal("notify workflow jobs is not a mapping")
	}
	notify, ok := jobs["notify"].(map[string]any)
	if !ok {
		t.Fatal("notify workflow has no notify job")
	}
	steps, ok := notify["steps"].([]any)
	if !ok || len(steps) != 2 {
		t.Fatalf("notify steps = %#v, want gate + dispatch", notify["steps"])
	}

	gate, ok := steps[0].(map[string]any)
	if !ok || gate["id"] != "dispatch_gate" {
		t.Fatalf("first notify step = %#v, want dispatch_gate", steps[0])
	}
	gateEnv, ok := gate["env"].(map[string]any)
	if !ok {
		t.Fatal("dispatch gate env is not a mapping")
	}
	if gateEnv["SOURCE_REPOSITORY"] != "${{ github.repository }}" {
		t.Fatalf("SOURCE_REPOSITORY = %#v", gateEnv["SOURCE_REPOSITORY"])
	}
	if gateEnv["HOSTED_TOKEN"] != "${{ secrets.GASCITY_HOSTED_TOKEN }}" {
		t.Fatalf("HOSTED_TOKEN = %#v", gateEnv["HOSTED_TOKEN"])
	}
	gateScript, ok := gate["run"].(string)
	if !ok || strings.TrimSpace(gateScript) == "" {
		t.Fatal("dispatch gate has no shell program")
	}

	for _, snippet := range []string{
		`if [[ "$SOURCE_REPOSITORY" != "gascity/gascity" ]]; then`,
		"decision=skip\n  reason=fork",
		`elif [[ -z "$HOSTED_TOKEN" ]]; then`,
		"decision=skip\n  reason=missing-token",
		"decision=dispatch\n  reason=configured",
		`printf 'decision=%s\nreason=%s\n' "$decision" "$reason" >> "$GITHUB_OUTPUT"`,
	} {
		if !strings.Contains(gateScript, snippet) {
			t.Fatalf("dispatch gate is missing exact policy fragment %q:\n%s", snippet, gateScript)
		}
	}
	if got := strings.Count(gateScript, "$HOSTED_TOKEN"); got != 1 {
		t.Fatalf("dispatch gate references hosted token %d times, want only the emptiness check", got)
	}

	dispatch, ok := steps[1].(map[string]any)
	if !ok {
		t.Fatal("dispatch step is not a mapping")
	}
	if dispatch["if"] != "steps.dispatch_gate.outputs.decision == 'dispatch'" {
		t.Fatalf("dispatch if = %#v", dispatch["if"])
	}
	dispatchEnv, ok := dispatch["env"].(map[string]any)
	if !ok || dispatchEnv["GH_TOKEN"] != "${{ secrets.GASCITY_HOSTED_TOKEN }}" {
		t.Fatalf("dispatch GH_TOKEN = %#v", dispatch["env"])
	}
	run, ok := dispatch["run"].(string)
	if !ok || strings.Count(run, "repos/gascity/gasworks-control-plane/dispatches") != 1 ||
		strings.Count(run, "event_type=runtime-dep-updated") != 1 {
		t.Fatalf("dispatch run must send exactly one pinned repository_dispatch:\n%s", run)
	}
}

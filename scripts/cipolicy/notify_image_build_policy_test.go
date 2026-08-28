package cipolicy

import (
	"os"
	"os/exec"
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

	for _, tc := range []struct {
		name       string
		repository string
		token      string
		decision   string
		reason     string
	}{
		{name: "fork with token", repository: "loucmane/gascity", token: "present", decision: "skip", reason: "fork"},
		{name: "upstream without token", repository: "gascity/gascity", decision: "skip", reason: "missing-token"},
		{name: "upstream with token", repository: "gascity/gascity", token: "present", decision: "dispatch", reason: "configured"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			output := filepath.Join(dir, "output")
			summary := filepath.Join(dir, "summary")
			cmd := exec.Command("bash", "-c", gateScript)
			cmd.Env = append(os.Environ(),
				"SOURCE_REPOSITORY="+tc.repository,
				"HOSTED_TOKEN="+tc.token,
				"GITHUB_OUTPUT="+output,
				"GITHUB_STEP_SUMMARY="+summary,
			)
			combined, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("dispatch gate failed: %v\n%s", err, combined)
			}
			got, err := os.ReadFile(output)
			if err != nil {
				t.Fatalf("read gate output: %v", err)
			}
			want := "decision=" + tc.decision + "\nreason=" + tc.reason + "\n"
			if string(got) != want {
				t.Fatalf("gate output = %q, want %q", got, want)
			}
			if strings.Contains(string(combined), tc.token) && tc.token != "" {
				t.Fatal("dispatch gate logged the hosted token")
			}
		})
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

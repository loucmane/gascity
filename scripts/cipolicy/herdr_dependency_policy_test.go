package cipolicy

import (
	"reflect"
	"testing"
)

// Removing only the exact new dependency step must recover the prior complete
// execution projection. This pins the delta, not just a newly accepted hash.
func TestHerdrDependencyIsOnlyCIExecutionDelta(t *testing.T) {
	docs := loadPolicyDocuments(t)
	integration := job(t, docs.ci, "integration-shards")
	steps, ok := integration["steps"].([]any)
	if !ok {
		t.Fatal("integration steps are not a sequence")
	}
	want := map[string]any{
		"name": "Provision pinned Herdr conformance dependency",
		"if":   "startsWith(matrix.shard_name, 'packages-core-')",
		"run": "PYTHONDONTWRITEBYTECODE=1 python3 scripts/test_install_herdr_test_dependency.py\n" +
			"herdr_dir=\"$(python3 scripts/install_herdr_test_dependency.py)\"\n" +
			"printf '%s\\n' \"$herdr_dir\" >> \"$GITHUB_PATH\"\n",
	}
	found := -1
	for i, value := range steps {
		candidate, ok := value.(map[string]any)
		if !ok {
			t.Fatal("integration step is not a mapping")
		}
		if candidate["name"] == want["name"] {
			if found != -1 || !reflect.DeepEqual(candidate, want) {
				t.Fatal("Herdr dependency step is duplicated or its closed execution shape changed")
			}
			found = i
		}
	}
	if found < 1 || found+1 >= len(steps) {
		t.Fatal("Herdr dependency must follow setup and precede the integration run")
	}
	before := steps[found-1].(map[string]any)
	after := steps[found+1].(map[string]any)
	with, ok := before["with"].(map[string]any)
	if !ok || with["install-claude-cli"] != "false" ||
		after["run"] != "${{ matrix.command }}" {
		t.Fatal("Herdr dependency is not between inert CLI setup and the selected shard")
	}
	// The actual workflow, including the exact dependency step, must pass all
	// existing provider-ownership, trigger, setup, nightly and shape checks.
	if err := validate(docs.ci, docs.nightly, docs.action); err != nil {
		t.Fatal(err)
	}
	integration["steps"] = append(append([]any(nil), steps[:found]...), steps[found+1:]...)
	// Prior projection re-pinned for the reviewed BD_VERSION v1.2.2 -> v1.3.0 env delta.
	const priorExecution = "f79914b93a5215e70421fa2aa7b1dadc73cf823dabf0a7779653528cd181f23a"
	if err := assertWorkflowExecution("CI without only the Herdr step", docs.ci, priorExecution); err != nil {
		t.Fatal(err)
	}
}

package cipolicy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMetadataToolchainIsOnlySetupExecutionDelta(t *testing.T) {
	docs := loadPolicyDocuments(t)
	if got := input(t, docs.action, "go-version")["default"]; got != "1.26.7" {
		t.Fatalf("Ubuntu custody toolchain = %v, want 1.26.7", got)
	}
	if err := validate(docs.ci, docs.nightly, docs.action); err != nil {
		t.Fatal(err)
	}
	// Reverting only the reviewed default must recover the complete prior
	// execution projection; no other setup mutation is silently accepted.
	input(t, docs.action, "go-version")["default"] = "1.26.6"
	const prior = "6a7fa725afa5cdf18cc459ea401a74bbd83cef757bf4e5d0dcd131a010df6ac0"
	if err := assertSemanticHash("setup without only custody Go alignment", projectAction(docs.action), prior); err != nil {
		t.Fatal(err)
	}
}

func TestMetadataToolchainModuleAndCompositeDefaultsAgree(t *testing.T) {
	root := filepath.Join("..", "..")
	body, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(body), "\ngo ") != 1 || !strings.Contains(string(body), "\ngo 1.26.7\n") {
		t.Fatal("module directive must select the reviewed Go 1.26.7 custody profile")
	}
	for _, platform := range []string{"ubuntu", "macos"} {
		action := readYAMLMap(t, filepath.Join(root, ".github", "actions", "setup-gascity-"+platform, "action.yml"))
		if got := input(t, action, "go-version")["default"]; got != "1.26.7" {
			t.Fatalf("%s Go default = %v, want 1.26.7", platform, got)
		}
	}
}

package docsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedUpgradeRunbookCoversOperationsAndRecoveryContract(t *testing.T) {
	root := repoRoot()
	rel := filepath.Join("docs", "runbooks", "versioned-platform-upgrades.md")
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("reading %s: %v", filepath.ToSlash(rel), err)
	}
	text := string(data)

	for _, section := range []string{
		"## Authority matrix",
		"## Authorization matrix",
		"## Cutover checklist",
		"## Postflight and two-tick reconciliation",
		"## Recovery decision tree",
		"## Routine operations",
		"## Evidence checklist",
		"## Worked 1.4.1-local evidence",
	} {
		if !strings.Contains(text, section) {
			t.Errorf("%s must contain section %q", filepath.ToSlash(rel), section)
		}
	}

	for _, fact := range []string{
		"source checkout",
		"/proc/<supervisor-pid>/exe",
		"gc.platform-install-manifest.v1",
		"configuration authority",
		"pack/lock/cache",
		"receipt",
		"rollback dry-run",
		"result=noop",
		"two stable reconciliation observations",
		"permission denied",
		"namespace",
		"wrong worktree",
		"service epoch",
		"provider binary missing",
		"stale or alias session",
		"validator or `check_path` missing",
		"reply unreadable",
		"approval wait",
		"transport error or `no_work`",
		"rules or grants missing",
		"canary failed or receipt drifted",
		"managed-product dispatch gate",
	} {
		if !strings.Contains(strings.ToLower(text), strings.ToLower(fact)) {
			t.Errorf("%s must document %q", filepath.ToSlash(rel), fact)
		}
	}
}

func TestManagedUpgradeRunbookIsLinkedFromTroubleshooting(t *testing.T) {
	root := repoRoot()
	data, err := os.ReadFile(filepath.Join(root, "docs", "troubleshooting", "index.md"))
	if err != nil {
		t.Fatalf("reading troubleshooting index: %v", err)
	}
	if !strings.Contains(string(data), "/runbooks/versioned-platform-upgrades") {
		t.Fatal("troubleshooting index must link the managed upgrade and recovery runbook")
	}
}

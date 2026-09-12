package docsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCacheReadonlyDropinHasOnlyOptionalLockSetting(t *testing.T) {
	path := filepath.Join(repoRoot(), "contrib", "systemd", "gascity-cache-readonly.conf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const want = "[Service]\nEnvironment=GIT_OPTIONAL_LOCKS=0\n"
	if string(data) != want {
		t.Fatalf("drop-in must set exactly one environment variable: got %q, want %q", data, want)
	}
}

func TestCacheReadonlyDropinDocumentsConfigurationAndBinaryRollbackSeparately(t *testing.T) {
	path := filepath.Join(repoRoot(), "docs", "runbooks", "versioned-platform-upgrades.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(strings.Fields(string(data)), " ")
	for _, required := range []string{
		"contrib/systemd/gascity-cache-readonly.conf",
		"$HOME/.config/systemd/user/<exact-supervisor-unit>.d/90-gas-city-cache-readonly.conf",
		"not a cache-write sandbox",
		"mandatory writes, access-time changes, or missing-cache materialization",
		"HEAD and dirty-tree checks remain enabled",
		"exact preimage and unrelated settings",
		"GIT_OPTIONAL_LOCKS=0",
		"already-planned broker restart",
		"binary rollback with adopted configuration retained",
		"removing the drop-in and reloading does not change an already-running process",
		"before the strict cache baseline",
		"no automatic city resume",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("runbook must document %q", required)
		}
	}
}

package dolt_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/orders"
)

func TestStaleDBScheduledOrderIsMechanical(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "orders", "mol-dog-stale-db.toml"))
	if err != nil {
		t.Fatal(err)
	}
	order, err := orders.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := orders.Validate(order); err != nil {
		t.Fatal(err)
	}
	if order.Exec != "$PACK_DIR/assets/scripts/mol-dog-stale-db.sh" {
		t.Fatalf("Exec = %q", order.Exec)
	}
	if order.Formula != "" || order.Pool != "" {
		t.Fatalf("scheduled stale-db maintenance must not create formula or pool work: %#v", order)
	}
}

func TestStaleDBExecScriptHasNoAgentOrBeadLifecycle(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "assets", "scripts", "mol-dog-stale-db.sh")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"gc dolt-cleanup --json --probe",
		"--force --max-orphan-dbs",
		"mol-dog-stale-db.scan",
		"mol-dog-stale-db.escalate",
		"mol-dog-stale-db.done",
		"latest.json",
		"set -euo pipefail",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("script missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"GC_BEAD_ID",
		"bd update",
		"bd close",
		"gc sling",
		"gc session new",
		"gc session wake",
		"gc session nudge",
		"gc runtime drain-ack",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("mechanical script contains model/bead lifecycle %q", forbidden)
		}
	}
}

func TestStaleDBExecNoWorkWritesDurableReportWithoutSession(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	runtimeDir := filepath.Join(dir, "runtime")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "gc.log")
	fakeGC := filepath.Join(binDir, "gc")
	writeTestFile(t, fakeGC, `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$GC_TEST_LOG"
case "$*" in
  *"dolt-cleanup --json --probe"*)
    printf '%s\n' '{"schema":"gc.dolt.cleanup.v1","dropped":{"count":0,"skipped":[],"failed":[]},"purge":{"bytes_reclaimed":0},"reaped":{"count":0,"targets":[]},"summary":{"bytes_freed_disk":0,"bytes_freed_rss":0,"errors_total":0},"force_blockers":[]}'
    ;;
  *"event emit mol-dog-stale-db.scan"*|*"event emit mol-dog-stale-db.done"*) exit 0 ;;
  *) echo "unexpected: $*" >&2; exit 64 ;;
esac
`, 0o755)
	out, err := runDogScriptCommand(t, "mol-dog-stale-db.sh", binDir, dir, dir,
		"GC_CITY_RUNTIME_DIR="+runtimeDir,
		"GC_TEST_LOG="+logPath,
		"TMPDIR="+dir,
	)
	if err != nil {
		t.Fatalf("script: %v\n%s", err, out)
	}
	reportPath := filepath.Join(runtimeDir, "maintenance", "mol-dog-stale-db", "latest.json")
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(reportBytes, &report); err != nil {
		t.Fatalf("report JSON: %v", err)
	}
	if report["schema"] != "gc.maintenance.stale-db.v1" || report["decision"] != "no-work" {
		t.Fatalf("report = %#v", report)
	}
	log := string(mustReadFile(t, logPath))
	if strings.Contains(log, "session") || strings.Contains(log, "sling") || strings.Contains(log, "bd ") {
		t.Fatalf("model/bead lifecycle invoked:\n%s", log)
	}
}

func TestStaleDBExecApplyFailureIsLoudAndPreservesReport(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	runtimeDir := filepath.Join(dir, "runtime")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "gc.log")
	fakeGC := filepath.Join(binDir, "gc")
	writeTestFile(t, fakeGC, `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$GC_TEST_LOG"
case "$*" in
  *"dolt-cleanup --json --probe --force"*)
    printf '%s\n' '{"schema":"gc.dolt.cleanup.v1","dropped":{"count":0,"failed":[{"name":"old","error":"drop failed"}]},"purge":{"bytes_reclaimed":0},"reaped":{"count":0,"targets":[]},"summary":{"bytes_freed_disk":0,"bytes_freed_rss":0,"errors_total":1},"errors":[{"stage":"drop","error":"drop failed"}]}'
    exit 42
    ;;
  *"dolt-cleanup --json --probe"*)
    printf '%s\n' '{"schema":"gc.dolt.cleanup.v1","dropped":{"count":1,"skipped":[],"failed":[]},"purge":{"bytes_reclaimed":0},"reaped":{"count":0,"targets":[]},"summary":{"bytes_freed_disk":0,"bytes_freed_rss":0,"errors_total":0},"force_blockers":[]}'
    ;;
  *"event emit mol-dog-stale-db.scan"*|*"event emit mol-dog-stale-db.escalate"*) exit 0 ;;
  *"mail send human"*) exit 0 ;;
  *) echo "unexpected: $*" >&2; exit 64 ;;
esac
`, 0o755)
	out, err := runDogScriptCommand(t, "mol-dog-stale-db.sh", binDir, dir, dir,
		"GC_CITY_RUNTIME_DIR="+runtimeDir,
		"GC_TEST_LOG="+logPath,
		"TMPDIR="+dir,
	)
	if err == nil {
		t.Fatalf("apply failure returned success:\n%s", out)
	}
	report := string(mustReadFile(t, filepath.Join(runtimeDir, "maintenance", "mol-dog-stale-db", "latest.json")))
	if !strings.Contains(report, `"decision": "apply-failed"`) || !strings.Contains(report, "drop failed") {
		t.Fatalf("failure report missing evidence:\n%s", report)
	}
	log := string(mustReadFile(t, logPath))
	if !strings.Contains(log, "event emit mol-dog-stale-db.escalate") || !strings.Contains(log, "mail send human") {
		t.Fatalf("failure was not escalated:\n%s", log)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

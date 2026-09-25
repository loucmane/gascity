package gastown_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// nudgeOnRouteGCStub answers the three gc calls nudge-on-route.sh makes:
// `gc events` replays $GC_EVENTS_FILE, `gc session list` reports no pool
// members (so targets are nudged directly), and `gc session nudge` is logged.
const nudgeOnRouteGCStub = `#!/bin/sh
case "$1 $2" in
  "events --type")
    cat "$GC_EVENTS_FILE"
    exit 0
    ;;
  "session list")
    printf '{"sessions":[]}\n'
    exit 0
    ;;
  "session nudge")
    printf '%s|%s\n' "$3" "$4" >> "$GC_NUDGE_LOG"
    exit 0
    ;;
esac
exit 1
`

func runNudgeOnRoute(t *testing.T, events string) (string, func() string) {
	t.Helper()
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("nudge-on-route.sh requires jq")
	}
	cityDir := t.TempDir()
	binDir := t.TempDir()
	eventsFile := filepath.Join(t.TempDir(), "events.jsonl")
	nudgeLog := filepath.Join(t.TempDir(), "nudges.log")
	if err := os.WriteFile(eventsFile, []byte(events), 0o644); err != nil {
		t.Fatalf("WriteFile(events): %v", err)
	}
	writeExecutable(t, filepath.Join(binDir, "gc"), nudgeOnRouteGCStub)
	env := mergeTestEnv(map[string]string{
		"GC_CITY":           cityDir,
		"GC_PACK_STATE_DIR": filepath.Join(cityDir, "state"),
		"GC_EVENTS_FILE":    eventsFile,
		"GC_NUDGE_LOG":      nudgeLog,
		"PATH":              binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	})
	run := func() string {
		cmd := exec.Command(coreScriptPath("nudge-on-route.sh"))
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("nudge-on-route.sh failed: %v\n%s", err, out)
		}
		return string(out)
	}
	out := run()
	readLog := func() string {
		data, err := os.ReadFile(nudgeLog)
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("ReadFile(nudge log): %v", err)
		}
		return string(data)
	}
	return out, func() string { run(); return readLog() }
}

// TestNudgeOnRouteAcceptsWrappedAndFlatPayloads guards ga-odny: the supervisor
// API serves bead.updated with the typed {"bead": {...}} payload, while the
// event log holds the flat bead snapshot. A filter that matches only one shape
// silently never nudges routed work in the other, so both must deliver.
func TestNudgeOnRouteAcceptsWrappedAndFlatPayloads(t *testing.T) {
	events := strings.Join([]string{
		// Shape served by `gc events` through the supervisor API (captured live).
		`{"seq":1,"type":"bead.updated","actor":"cache-reconcile","subject":"ga-wrapped","payload":{"bead":{"id":"ga-wrapped","status":"open","metadata":{"gc.routed_to":"gascity/gc.implementation-worker"}}}}`,
		// Flat snapshot recorded by CachingStore.
		`{"seq":2,"type":"bead.updated","actor":"bd","subject":"ga-flat","payload":{"id":"ga-flat","status":"open","metadata":{"gc.routed_to":"gascity/gc.review-worker"}}}`,
		// Unrouted, empty-target and malformed payloads are ignored.
		`{"seq":3,"type":"bead.updated","actor":"bd","subject":"ga-plain","payload":{"bead":{"id":"ga-plain","metadata":{}}}}`,
		`{"seq":4,"type":"bead.updated","actor":"bd","subject":"ga-empty","payload":{"id":"ga-empty","metadata":{"gc.routed_to":""}}}`,
		`{"seq":5,"type":"bead.updated","actor":"bd","subject":"ga-bad","payload":"not-an-object"}`,
		// A repeated routing event is nudged once.
		`{"seq":6,"type":"bead.updated","actor":"cache-reconcile","subject":"ga-wrapped","payload":{"bead":{"id":"ga-wrapped","status":"open","metadata":{"gc.routed_to":"gascity/gc.implementation-worker"}}}}`,
	}, "\n") + "\n"

	out, rerun := runNudgeOnRoute(t, events)
	if !strings.Contains(out, "nudged 2 newly-routed bead(s)") {
		t.Fatalf("expected two nudges, got output:\n%s", out)
	}
	log := rerun()
	// The script visits unique pairs sorted by bead id: ga-flat, then ga-wrapped.
	want := []string{
		"gascity/gc.review-worker|check for assigned work",
		"gascity/gc.implementation-worker|check for assigned work",
	}
	got := strings.Split(strings.TrimSpace(log), "\n")
	if len(got) != len(want) {
		t.Fatalf("nudges after a repeated run = %q, want exactly %q (dedup must hold)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("nudge %d = %q, want %q (all: %q)", i, got[i], want[i], got)
		}
	}
}

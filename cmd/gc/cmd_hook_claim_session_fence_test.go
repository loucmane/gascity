package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/session"
)

// writeFenceTestCity writes a minimal single-worker city and returns its dir.
func writeFenceTestCity(t *testing.T) string {
	t.Helper()
	cityDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityDir, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cityDir, "city.toml"), []byte(`[workspace]
name = "test-city"

[[agent]]
name = "worker"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	return cityDir
}

// newFenceSessionBead creates a session bead in the city store with the given
// runtime state and instance token, returning its id.
func newFenceSessionBead(t *testing.T, cityDir string, state session.State, instanceToken string) string {
	t.Helper()
	store, err := openCityStoreAt(cityDir)
	if err != nil {
		t.Fatalf("openCityStoreAt: %v", err)
	}
	bead, err := store.Create(beads.Bead{
		Title:  "worker-1",
		Type:   session.BeadType,
		Labels: []string{"gc:session", "agent:worker-1"},
		Metadata: map[string]string{
			"session_name":   "worker-1",
			"template":       "worker",
			"state":          string(state),
			"instance_token": instanceToken,
		},
	})
	if err != nil {
		t.Fatalf("create session bead: %v", err)
	}
	return bead.ID
}

// installFenceWorkQueryProbe puts a fake `bd` on PATH that records each
// invocation by touching the returned marker path and prints an empty
// work-query result, so a test can assert whether the claim fence reached the
// work query. Call it AFTER creating any session beads so bead setup uses the
// real store, not the probe.
func installFenceWorkQueryProbe(t *testing.T) string {
	t.Helper()
	fakeBin := t.TempDir()
	queryMarker := filepath.Join(t.TempDir(), "query-ran")
	fakeBD := filepath.Join(fakeBin, "bd")
	if err := os.WriteFile(fakeBD, []byte("#!/bin/sh\ntouch \"$QUERY_MARKER\"\nprintf '[]'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("QUERY_MARKER", queryMarker)
	return queryMarker
}

// setFenceClaimEnv points cmdHookWithOptions at the given session identity.
func setFenceClaimEnv(t *testing.T, cityDir, sessionID, instanceToken string) {
	t.Helper()
	t.Setenv("GC_CITY", cityDir)
	t.Setenv("GC_TEMPLATE", "worker")
	t.Setenv("GC_ALIAS", "worker-1")
	t.Setenv("GC_SESSION_ID", sessionID)
	t.Setenv("GC_SESSION_NAME", "worker-1")
	t.Setenv("GC_SESSION_ORIGIN", "ephemeral")
	t.Setenv("GC_INSTANCE_TOKEN", instanceToken)
}

const (
	claimIdentityRaceSessionID = "ci-a7u3"
	claimIdentityRaceWorkID    = "ga-routed-work"
	claimIdentityRaceRoute     = "hpfetcher/gc.implementation-worker"
	claimIdentityRaceToken     = "fresh-instance-token"
)

// newClaimIdentityRaceFixture reproduces the provider-before-projection order
// seen in ga-k4p. The runtime and all of its GC_* identity env exist, and the
// backing store contains one open routed task, but the session bead is not
// readable on the first `bd show`. In "appear" mode the cache-reconcile witness
// becomes readable on the second identity lookup; in "never" mode it remains
// unavailable; in "present" mode it exists from the first lookup.
//
// The fake bd intentionally returns a bare exit 1 for the missing session, the
// exact error shape from the live witnesses. While identity is unavailable its
// work reads fail too. The default work_query suppresses those errors and falls
// through to [], pinning the layer that currently launders the failure into
// benign no_work.
func newClaimIdentityRaceFixture(t *testing.T, mode string) (string, string) {
	t.Helper()
	clearGCEnv(t)
	disableManagedDoltRecoveryForTest(t)
	t.Setenv("GC_BEADS_FORCE_FALLBACK", "1")

	cityDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityDir, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	cityToml := `[workspace]
name = "test-city"

[beads]
provider = "bd"

[[rigs]]
name = "hpfetcher"
path = "."

[[agent]]
name = "gc.implementation-worker"
dir = "hpfetcher"
max_active_sessions = 2
`
	if err := os.WriteFile(filepath.Join(cityDir, "city.toml"), []byte(cityToml), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cityDir, ".beads", "config.yaml"), []byte("issue_prefix: ga\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stateDir := t.TempDir()
	fakeBin := t.TempDir()
	fakeBD := filepath.Join(fakeBin, "bd")
	script := `#!/bin/sh
set -eu
session_json='[{"id":"` + claimIdentityRaceSessionID + `","title":"worker","status":"open","issue_type":"session","labels":["gc:session"],"metadata":{"session_name":"worker-1","template":"` + claimIdentityRaceRoute + `","state":"active","instance_token":"` + claimIdentityRaceToken + `"}}]'
open_work='[{"id":"` + claimIdentityRaceWorkID + `","title":"routed work","status":"open","issue_type":"task","assignee":"","metadata":{"gc.routed_to":"` + claimIdentityRaceRoute + `","gc.session_id":"` + claimIdentityRaceSessionID + `","gc.session_name":"worker-1"}}]'
claimed_work='[{"id":"` + claimIdentityRaceWorkID + `","title":"routed work","status":"in_progress","issue_type":"task","assignee":"worker-1","metadata":{"gc.routed_to":"` + claimIdentityRaceRoute + `","gc.session_id":"` + claimIdentityRaceSessionID + `","gc.session_name":"worker-1"}}]'
identity_ready() {
  case "$CLAIM_IDENTITY_MODE" in
    present) return 0 ;;
    appear) [ -f "$CLAIM_IDENTITY_READS" ] && [ "$(wc -c < "$CLAIM_IDENTITY_READS")" -ge 2 ] ;;
    never) return 1 ;;
  esac
}
case "${1:-}" in
  show)
    id="${3:-}"
    if [ "$id" = "` + claimIdentityRaceSessionID + `" ]; then
      if [ "$CLAIM_IDENTITY_MODE" = "appear" ]; then
        printf x >> "$CLAIM_IDENTITY_READS"
      fi
      if identity_ready; then
        printf '%s\n' "$session_json"
        exit 0
      fi
      exit 1
    fi
    if [ "$id" = "` + claimIdentityRaceWorkID + `" ]; then
      if [ -d "$CLAIM_LOCK" ]; then printf '%s\n' "$claimed_work"; else printf '%s\n' "$open_work"; fi
      exit 0
    fi
    exit 1
    ;;
  list|query)
    if identity_ready; then printf '[]\n'; else exit 1; fi
    ;;
  ready)
    if ! identity_ready; then exit 1; fi
    if [ -d "$CLAIM_LOCK" ]; then printf '[]\n'; else printf '%s\n' "$open_work"; fi
    ;;
  update)
    if [ "${2:-}" = "` + claimIdentityRaceWorkID + `" ] && [ "${3:-}" = "--claim" ]; then
      if mkdir "$CLAIM_LOCK" 2>/dev/null; then
        printf '%s\n' "$claimed_work"
        exit 0
      fi
      printf '%s\n' '{"error":"issue is already assigned to worker-1"}'
      exit 1
    fi
    printf '%s\n' "$claimed_work"
    ;;
  *)
    printf '[]\n'
    ;;
esac
`
	if err := os.WriteFile(fakeBD, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	claimLock := filepath.Join(stateDir, "claim.lock")
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CLAIM_IDENTITY_MODE", mode)
	t.Setenv("CLAIM_IDENTITY_READS", filepath.Join(stateDir, "identity-reads"))
	t.Setenv("CLAIM_LOCK", claimLock)
	setFenceClaimEnv(t, cityDir, claimIdentityRaceSessionID, claimIdentityRaceToken)
	t.Setenv("GC_TEMPLATE", claimIdentityRaceRoute)
	t.Setenv("GC_ALIAS", claimIdentityRaceRoute+"-1")
	t.Setenv("GC_AGENT", claimIdentityRaceRoute+"-1")
	t.Setenv("GC_SESSION_NAME", "worker-1")
	t.Setenv("GC_SESSION_ORIGIN", "ephemeral")

	sp := runtime.NewFake()
	if err := sp.Start(context.Background(), "worker-1", runtime.Config{Env: map[string]string{
		"GC_SESSION_ID":     claimIdentityRaceSessionID,
		"GC_SESSION_NAME":   "worker-1",
		"GC_TEMPLATE":       claimIdentityRaceRoute,
		"GC_INSTANCE_TOKEN": claimIdentityRaceToken,
	}}); err != nil {
		t.Fatalf("register provider session: %v", err)
	}
	if !sp.IsRunning("worker-1") {
		t.Fatal("provider session is not running")
	}
	return cityDir, claimLock
}

func decodeClaimIdentityRaceResult(t *testing.T, stdout *bytes.Buffer) hookClaimJSONResult {
	t.Helper()
	var result hookClaimJSONResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		t.Fatalf("stdout is not a JSON claim result: %v\n%s", err, stdout.String())
	}
	return result
}

func TestHookCommandClaimWaitsForSessionIdentityProjection(t *testing.T) {
	_, claimLock := newClaimIdentityRaceFixture(t, "appear")

	var stdout, stderr bytes.Buffer
	code := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &stdout, &stderr)
	result := decodeClaimIdentityRaceResult(t, &stdout)
	if code != 0 || result.Action != "work" || result.Reason != "claimed" || result.BeadID != claimIdentityRaceWorkID {
		_, claimErr := os.Stat(claimLock)
		t.Fatalf("claim before session projection = code %d result %+v claim_lock=%v, want one claimed work result; stderr=%s",
			code, result, claimErr, stderr.String())
	}
}

func TestHookCommandClaimIdentityFailureIsNeverNoWork(t *testing.T) {
	newClaimIdentityRaceFixture(t, "never")

	var stdout, stderr bytes.Buffer
	code := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &stdout, &stderr)
	result := decodeClaimIdentityRaceResult(t, &stdout)
	if code != 1 {
		t.Fatalf("code = %d, want 1; result=%+v stderr=%s", code, result, stderr.String())
	}
	if result.Action != "drain" || result.Reason != hookClaimReasonClaimsErrored {
		t.Fatalf("identity-read exhaustion = %+v, want drain/claims_errored (never no_work); stderr=%s", result, stderr.String())
	}
}

func TestHookCommandConcurrentClaimsAcquireRoutedWorkOnce(t *testing.T) {
	_, claimLock := newClaimIdentityRaceFixture(t, "present")

	type outcome struct {
		code   int
		result hookClaimJSONResult
		stderr string
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			ready.Done()
			<-start
			var stdout, stderr bytes.Buffer
			code := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &stdout, &stderr)
			outcomes <- outcome{code: code, result: decodeClaimIdentityRaceResult(t, &stdout), stderr: stderr.String()}
		}()
	}
	ready.Wait()
	close(start)

	claimed := 0
	for range 2 {
		got := <-outcomes
		if got.result.Action == "work" && got.result.Reason == "claimed" {
			claimed++
			if got.code != 0 || got.result.BeadID != claimIdentityRaceWorkID {
				t.Fatalf("claimed outcome = %+v code=%d stderr=%s", got.result, got.code, got.stderr)
			}
		}
	}
	if claimed != 1 {
		t.Fatalf("claimed outcomes = %d, want exactly 1", claimed)
	}
	if _, err := os.Stat(claimLock); err != nil {
		t.Fatalf("claim mutation did not commit: %v", err)
	}
}

// TestHookCommandClaimStaleSessionDrainsBeforeWorkQuery proves a definitively
// stale session (here failed-create) is refused before the work query AND that
// the refusal now honors the gc hook --claim result contract: a --json caller
// gets a structured terminal drain record (action=drain, reason=stale_session)
// instead of empty stdout, so a startup wrapper can distinguish a definitive
// stale-session refusal from a transient command failure and stop retrying.
func TestHookCommandClaimStaleSessionDrainsBeforeWorkQuery(t *testing.T) {
	clearGCEnv(t)
	disableManagedDoltRecoveryForTest(t)
	t.Setenv("GC_BEADS", "file")
	cityDir := writeFenceTestCity(t)
	sessionID := newFenceSessionBead(t, cityDir, session.StateFailedCreate, "failed-token")
	queryMarker := installFenceWorkQueryProbe(t)
	setFenceClaimEnv(t, cityDir, sessionID, "failed-token")

	var stdout, stderr bytes.Buffer
	code := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &stdout, &stderr)

	// Without --drain-ack the refusal is still terminal (exit 1) but now carries a
	// schema-backed drain record instead of empty stdout.
	if code != 1 {
		t.Fatalf("code = %d, want 1; stdout=%q stderr=%s", code, stdout.String(), stderr.String())
	}
	var result hookClaimJSONResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		t.Fatalf("stdout is not a JSON drain result: %v\n%s", err, stdout.String())
	}
	if result.Action != "drain" || result.Reason != hookClaimReasonStaleSession {
		t.Fatalf("result = %+v, want action=drain reason=stale_session", result)
	}
	if result.DrainAcknowledged {
		t.Fatalf("result.DrainAcknowledged = true without --drain-ack")
	}
	if !strings.Contains(stderr.String(), "refusing stale session") ||
		!strings.Contains(stderr.String(), "failed-create") {
		t.Fatalf("stderr = %q, want failed-session refusal naming the state", stderr.String())
	}
	if _, err := os.Stat(queryMarker); !os.IsNotExist(err) {
		t.Fatalf("work query ran for stale session; stat error = %v", err)
	}
}

// TestHookCommandClaimEligibleStatesReachWorkQuery proves the fence lets the
// states a live worker legitimately claims in — active/awake plus the in-startup
// states creating/start-pending the deferred-start path passes through before
// its async active commit lands — through to the work query, rather than
// refusing a healthy first claim as stale.
func TestHookCommandClaimEligibleStatesReachWorkQuery(t *testing.T) {
	for _, state := range []session.State{
		session.StateActive,
		session.StateAwake,
		session.StateCreating,
		session.StateStartPending,
	} {
		t.Run(string(state), func(t *testing.T) {
			clearGCEnv(t)
			disableManagedDoltRecoveryForTest(t)
			t.Setenv("GC_BEADS", "file")
			cityDir := writeFenceTestCity(t)
			sessionID := newFenceSessionBead(t, cityDir, state, "current-token")
			queryMarker := installFenceWorkQueryProbe(t)
			setFenceClaimEnv(t, cityDir, sessionID, "current-token")

			var stdout, stderr bytes.Buffer
			code := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &stdout, &stderr)

			// The probe bd returns no work, so the claim drains with no_work; the
			// point is that the fence let an eligible session THROUGH to the work
			// query (marker created) instead of refusing it as stale.
			if _, err := os.Stat(queryMarker); err != nil {
				t.Fatalf("work query did not run for eligible %s session: %v; stderr=%s", state, err, stderr.String())
			}
			if strings.Contains(stderr.String(), "refusing stale session") {
				t.Fatalf("eligible %s session was refused as stale: %s", state, stderr.String())
			}
			if code != 1 {
				t.Fatalf("code = %d, want 1 (JSON no-work drain without --drain-ack); stderr=%s", code, stderr.String())
			}
			var result hookClaimJSONResult
			if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
				t.Fatalf("stdout is not a JSON result: %v\n%s", err, stdout.String())
			}
			if result.Action != "drain" || result.Reason != hookClaimReasonNoWork {
				t.Fatalf("result = %+v, want action=drain reason=no_work (probe returns no work)", result)
			}
		})
	}
}

// TestHookCommandClaimEmptyLegacyStateReachesWorkQuery proves a pre-metadata legacy
// session bead — one persisted with an empty state during upgrade — reaches the work
// query instead of being refused as stale. With Closed=false and a matching instance
// token the runtime is the live current incarnation, and the session lifecycle
// canonicalizes empty state to active, so draining it here would starve a healthy
// upgraded legacy worker of its routed work.
func TestHookCommandClaimEmptyLegacyStateReachesWorkQuery(t *testing.T) {
	clearGCEnv(t)
	disableManagedDoltRecoveryForTest(t)
	t.Setenv("GC_BEADS", "file")
	cityDir := writeFenceTestCity(t)
	sessionID := newFenceSessionBead(t, cityDir, session.StateNone, "current-token")
	queryMarker := installFenceWorkQueryProbe(t)
	setFenceClaimEnv(t, cityDir, sessionID, "current-token")

	var stdout, stderr bytes.Buffer
	code := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &stdout, &stderr)

	if _, err := os.Stat(queryMarker); err != nil {
		t.Fatalf("work query did not run for empty-legacy-state session: %v; stderr=%s", err, stderr.String())
	}
	if strings.Contains(stderr.String(), "refusing stale session") {
		t.Fatalf("empty-legacy-state session was refused as stale: %s", stderr.String())
	}
	if code != 1 {
		t.Fatalf("code = %d, want 1 (JSON no-work drain without --drain-ack); stderr=%s", code, stderr.String())
	}
	var result hookClaimJSONResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		t.Fatalf("stdout is not a JSON result: %v\n%s", err, stdout.String())
	}
	if result.Action != "drain" || result.Reason != hookClaimReasonNoWork {
		t.Fatalf("result = %+v, want action=drain reason=no_work (probe returns no work)", result)
	}
}

// TestHookCommandClaimTokenlessRuntimeSkipsFence proves the fence's empty-token
// guard keeps a token-less (legacy/unmanaged) runtime out of the identity check:
// with GC_INSTANCE_TOKEN unset the fence is skipped entirely and the work query
// runs, even when the session bead is in a state the fence would otherwise refuse
// as stale. This pins the deliberate compatibility escape hatch so a future
// refactor cannot silently start fencing — and refusing — healthy legacy workers.
func TestHookCommandClaimTokenlessRuntimeSkipsFence(t *testing.T) {
	clearGCEnv(t)
	disableManagedDoltRecoveryForTest(t)
	t.Setenv("GC_BEADS", "file")
	cityDir := writeFenceTestCity(t)
	// A failed-create bead would drain stale if the fence ran; the point is that a
	// token-less runtime never reaches that classification.
	sessionID := newFenceSessionBead(t, cityDir, session.StateFailedCreate, "legacy-token")
	queryMarker := installFenceWorkQueryProbe(t)
	setFenceClaimEnv(t, cityDir, sessionID, "")

	var stdout, stderr bytes.Buffer
	code := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &stdout, &stderr)

	if _, err := os.Stat(queryMarker); err != nil {
		t.Fatalf("work query did not run for token-less runtime (fence should be skipped): %v; stderr=%s", err, stderr.String())
	}
	if strings.Contains(stderr.String(), "refusing stale session") {
		t.Fatalf("token-less runtime was refused by the fence: %s", stderr.String())
	}
	if code != 1 {
		t.Fatalf("code = %d, want 1 (JSON no-work drain without --drain-ack); stderr=%s", code, stderr.String())
	}
	var result hookClaimJSONResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		t.Fatalf("stdout is not a JSON result: %v\n%s", err, stdout.String())
	}
	if result.Action != "drain" || result.Reason != hookClaimReasonNoWork {
		t.Fatalf("result = %+v, want action=drain reason=no_work (probe returns no work)", result)
	}
}

// TestHookCommandClaimAbsentSessionBeadDrainsStale proves a runtime whose session
// bead is confirmed absent — GC_SESSION_ID names no bead in the store — is refused
// as stale before the work query, not failed open into the claim path. A vanished
// session bead is a definitive identity failure: the incarnation can no longer
// prove it is the current one, so it must drain (action=drain,
// reason=stale_session) and stop rather than adopt routed work ahead of the
// reconciler terminating it.
func TestHookCommandClaimAbsentSessionBeadDrainsStale(t *testing.T) {
	clearGCEnv(t)
	disableManagedDoltRecoveryForTest(t)
	t.Setenv("GC_BEADS", "file")
	cityDir := writeFenceTestCity(t)
	queryMarker := installFenceWorkQueryProbe(t)
	setFenceClaimEnv(t, cityDir, "worker-1-vanished", "any-token")

	var stdout, stderr bytes.Buffer
	code := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("code = %d, want 1; stdout=%q stderr=%s", code, stdout.String(), stderr.String())
	}
	var result hookClaimJSONResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		t.Fatalf("stdout is not a JSON drain result: %v\n%s", err, stdout.String())
	}
	if result.Action != "drain" || result.Reason != hookClaimReasonStaleSession {
		t.Fatalf("result = %+v, want action=drain reason=stale_session", result)
	}
	if !strings.Contains(stderr.String(), "refusing stale session") ||
		!strings.Contains(stderr.String(), "not found") {
		t.Fatalf("stderr = %q, want stale refusal naming the missing bead", stderr.String())
	}
	if _, err := os.Stat(queryMarker); !os.IsNotExist(err) {
		t.Fatalf("work query ran for a session with no bead; stat error = %v", err)
	}
}

// TestHookCommandClaimFailsOpenOnSessionStoreError proves a GENUINE session-store
// fault — here a corrupt/unreadable store file, so the fence's store open itself
// fails — is NOT mislabeled as a stale session: the fence fails open and lets the
// normal claim path run, which surfaces and escalates its own store errors. This
// is the counterpart to the absent-bead case above: a confirmed-missing bead
// drains stale, but an infrastructure fault must never refuse a possibly-healthy
// worker.
func TestHookCommandClaimFailsOpenOnSessionStoreError(t *testing.T) {
	clearGCEnv(t)
	disableManagedDoltRecoveryForTest(t)
	t.Setenv("GC_BEADS", "file")
	cityDir := writeFenceTestCity(t)
	queryMarker := installFenceWorkQueryProbe(t)
	// Corrupt the file store so openCityStoreAt fails to parse it: a genuine
	// store-open fault, distinct from an absent bead (a confirmed identity
	// failure that drains stale).
	if err := os.WriteFile(filepath.Join(cityDir, ".gc", "beads.json"), []byte("{ this is not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	setFenceClaimEnv(t, cityDir, "worker-1", "any-token")

	var stdout, stderr bytes.Buffer
	code := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &stdout, &stderr)

	if _, err := os.Stat(queryMarker); err != nil {
		t.Fatalf("fail-open did not reach the work query: %v; stderr=%s", err, stderr.String())
	}
	if strings.Contains(stderr.String(), "refusing stale session") {
		t.Fatalf("store fault was mislabeled as a stale session: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "session fence unavailable") {
		t.Fatalf("stderr = %q, want fence-unavailable diagnostic", stderr.String())
	}
	if code != 1 {
		t.Fatalf("code = %d, want 1 (JSON no-work drain without --drain-ack)", code)
	}
}

// TestClassifyHookClaimSessionLookupError exercises the error taxonomy that
// decides whether a failed session lookup is a definitive identity failure
// (stale, drain) or a transient store fault (unavailable, fail open). The two
// confirmed-identity errors mirror the documented session.Store.Get contract: a
// confirmed-absent id wraps beads.ErrNotFound, a present-but-non-session id is
// session.ErrSessionNotFound.
func TestClassifyHookClaimSessionLookupError(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		want    hookClaimSessionVerdict
		wantMsg string
	}{
		{
			name:    "confirmed absent bead is stale",
			err:     fmt.Errorf("loading session %q: %w", "s", beads.ErrNotFound),
			want:    hookClaimSessionStale,
			wantMsg: "not found",
		},
		{
			name:    "present but non-session bead is stale",
			err:     fmt.Errorf("%w: %s", session.ErrSessionNotFound, "s"),
			want:    hookClaimSessionStale,
			wantMsg: "non-session",
		},
		{
			name:    "genuine store read fault fails open",
			err:     fmt.Errorf("loading session %q: %w", "s", errors.New("dial tcp: connection refused")),
			want:    hookClaimSessionStoreUnavailable,
			wantMsg: "loading session bead",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verdict, msg := classifyHookClaimSessionLookupError(tc.err)
			if verdict != tc.want {
				t.Fatalf("verdict = %d, want %d (msg=%q)", verdict, tc.want, msg)
			}
			if !strings.Contains(msg, tc.wantMsg) {
				t.Fatalf("msg = %q, want substring %q", msg, tc.wantMsg)
			}
		})
	}
}

// TestHookClaimSessionEligibility exercises the pure eligibility decision over a
// session Info snapshot for every branch of the fence.
func TestHookClaimSessionEligibility(t *testing.T) {
	const token = "current-token"
	cases := []struct {
		name    string
		info    session.Info
		token   string
		want    hookClaimSessionVerdict
		wantMsg string
	}{
		{
			name:    "closed",
			info:    session.Info{ID: "s", Closed: true, InstanceToken: token},
			token:   token,
			want:    hookClaimSessionStale,
			wantMsg: "closed",
		},
		{
			name:    "superseded token",
			info:    session.Info{ID: "s", MetadataState: string(session.StateActive), InstanceToken: "replacement-token"},
			token:   "stale-runtime-token",
			want:    hookClaimSessionStale,
			wantMsg: "token",
		},
		{
			name:    "empty stored token",
			info:    session.Info{ID: "s", MetadataState: string(session.StateActive), InstanceToken: ""},
			token:   token,
			want:    hookClaimSessionStale,
			wantMsg: "token",
		},
		{
			name:    "failed-create",
			info:    session.Info{ID: "s", MetadataState: string(session.StateFailedCreate), InstanceToken: token},
			token:   token,
			want:    hookClaimSessionStale,
			wantMsg: "failed-create",
		},
		{
			name:    "drained",
			info:    session.Info{ID: "s", MetadataState: string(session.StateDrained), InstanceToken: token},
			token:   token,
			want:    hookClaimSessionStale,
			wantMsg: "drained",
		},
		{
			name:  "active",
			info:  session.Info{ID: "s", MetadataState: string(session.StateActive), InstanceToken: token},
			token: token,
			want:  hookClaimSessionEligible,
		},
		{
			name:  "awake",
			info:  session.Info{ID: "s", MetadataState: string(session.StateAwake), InstanceToken: token},
			token: token,
			want:  hookClaimSessionEligible,
		},
		{
			name:  "creating",
			info:  session.Info{ID: "s", MetadataState: string(session.StateCreating), InstanceToken: token},
			token: token,
			want:  hookClaimSessionEligible,
		},
		{
			name:  "start-pending",
			info:  session.Info{ID: "s", MetadataState: string(session.StateStartPending), InstanceToken: token},
			token: token,
			want:  hookClaimSessionEligible,
		},
		{
			// A pre-metadata legacy bead mid-upgrade carries an empty MetadataState
			// (session.StateNone). With Closed=false and a matching instance token it
			// is the live current incarnation, and the session lifecycle canonicalizes
			// empty state to active, so the fence must admit it rather than drain a
			// healthy upgraded legacy worker before it claims its routed work.
			name:  "empty legacy state admitted as active",
			info:  session.Info{ID: "s", MetadataState: string(session.StateNone), InstanceToken: token},
			token: token,
			want:  hookClaimSessionEligible,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verdict, reason := hookClaimSessionEligibility(tc.info, tc.token)
			if verdict != tc.want {
				t.Fatalf("verdict = %d, want %d (reason=%q)", verdict, tc.want, reason)
			}
			if tc.want == hookClaimSessionEligible && reason != "" {
				t.Fatalf("eligible verdict carried reason %q, want empty", reason)
			}
			if tc.wantMsg != "" && !strings.Contains(reason, tc.wantMsg) {
				t.Fatalf("reason = %q, want substring %q", reason, tc.wantMsg)
			}
		})
	}
}

// TestWriteHookClaimDrainStaleSessionWithDrainAck proves the stale-session drain
// honors --drain-ack: it runs the drain-ack, marks the record acknowledged, and
// exits 0 so a startup wrapper treats the refusal as a completed drain.
func TestWriteHookClaimDrainStaleSessionWithDrainAck(t *testing.T) {
	acked := false
	fakeAck := func(io.Writer) error { acked = true; return nil }

	var stdout, stderr bytes.Buffer
	code := writeHookClaimDrain(hookClaimReasonStaleSession, true, true, fakeAck, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("code = %d, want 0 for an acknowledged drain; stderr=%s", code, stderr.String())
	}
	if !acked {
		t.Fatalf("drain-ack function was not called")
	}
	var result hookClaimJSONResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if result.Action != "drain" || result.Reason != hookClaimReasonStaleSession || !result.DrainAcknowledged {
		t.Fatalf("result = %+v, want drain/stale_session/acknowledged", result)
	}
}

// TestWriteHookClaimDrainDoesNotAckWhenNotRequested proves the drain path never
// runs drain-ack unless --drain-ack was requested, and returns the historical
// exit 1 for an unacknowledged drain.
func TestWriteHookClaimDrainDoesNotAckWhenNotRequested(t *testing.T) {
	fakeAck := func(io.Writer) error {
		t.Fatalf("drain-ack must not run without --drain-ack")
		return nil
	}
	var stdout, stderr bytes.Buffer
	code := writeHookClaimDrain(hookClaimReasonStaleSession, true, false, fakeAck, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1 when drain is not acknowledged", code)
	}
	var result hookClaimJSONResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if result.DrainAcknowledged {
		t.Fatalf("result.DrainAcknowledged = true without --drain-ack")
	}
}

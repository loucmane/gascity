package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/clock"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/events"
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
			"generation":     "1",
		},
	})
	if err != nil {
		t.Fatalf("create session bead: %v", err)
	}
	writeClaimIdentityRaceProjection(t, cityDir, bead.ID, instanceToken, 1, state)
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
	t.Setenv("GC_RUNTIME_EPOCH", "1")
}

const (
	claimIdentityRaceSessionID = "gc-1"
	claimIdentityRaceWorkID    = "ga-routed-work"
	claimIdentityRaceRoute     = "hpfetcher/gc.implementation-worker"
	claimIdentityRaceToken     = "fresh-instance-token"
)

const claimIdentityRaceProjectionSchemaVersion = 1

type claimIdentityRaceProjection struct {
	SchemaVersion       int    `json:"schema_version"`
	SessionID           string `json:"session_id"`
	InstanceTokenSHA256 string `json:"instance_token_sha256"`
	Generation          int    `json:"generation"`
	State               string `json:"state"`
	ProjectedAt         string `json:"projected_at"`
}

func claimIdentityRaceProjectionPath(cityDir, sessionID string) string {
	return filepath.Join(cityDir, ".gc", "runtime", "session-fence", sessionID+".json")
}

func writeClaimIdentityRaceProjection(t *testing.T, cityDir, sessionID, token string, generation int, state session.State) string {
	t.Helper()
	path := claimIdentityRaceProjectionPath(cityDir, sessionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(token))
	data, err := json.Marshal(claimIdentityRaceProjection{
		SchemaVersion:       claimIdentityRaceProjectionSchemaVersion,
		SessionID:           sessionID,
		InstanceTokenSHA256: fmt.Sprintf("%x", digest[:]),
		Generation:          generation,
		State:               string(state),
		ProjectedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// newClaimIdentityRaceFixture installs separate city and rig stores used by the
// claim lifecycle tests. The city store exclusively owns session identity; the
// rig store exclusively owns routed product work, and the worker's ambient
// store environment points at that rig. In "barrier" mode the city session
// becomes readable only when the lifecycle test releases the projection gate;
// in "never" mode it remains unavailable; in "present" mode it exists from the
// first city-scoped lookup.
//
// The fake bd intentionally returns a bare exit 1 for the missing session, the
// exact error shape from the live witnesses. Its scope checks make a city
// identity read from the rig store fail and make a work read outside the rig
// store return empty, pinning both halves of the production topology.
func newClaimIdentityRaceFixture(t *testing.T, mode string) (string, string, string, string) {
	t.Helper()
	clearGCEnv(t)
	disableManagedDoltRecoveryForTest(t)
	t.Setenv("GC_BEADS_FORCE_FALLBACK", "1")

	cityDir := t.TempDir()
	rigDir := filepath.Join(cityDir, "rigs", "hpfetcher")
	if err := os.MkdirAll(filepath.Join(cityDir, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(rigDir, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	cityToml := `[workspace]
name = "test-city"

[beads]
provider = "bd"

[[rigs]]
name = "hpfetcher"
path = "rigs/hpfetcher"

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
	if err := os.WriteFile(filepath.Join(rigDir, ".beads", "config.yaml"), []byte("issue_prefix: ga\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []struct {
		dir       string
		projectID string
	}{
		{dir: cityDir, projectID: "city-store"},
		{dir: rigDir, projectID: "rig-store"},
	} {
		metadata := fmt.Sprintf("{\"backend\":\"doltlite\",\"database\":\"doltlite\",\"dolt_database\":\"gascity\",\"project_id\":%q}\n", scope.projectID)
		if err := os.WriteFile(filepath.Join(scope.dir, ".beads", "metadata.json"), []byte(metadata), 0o644); err != nil {
			t.Fatal(err)
		}
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
    barrier) [ -f "$CLAIM_PROJECTION_READY" ] ;;
    never) return 1 ;;
  esac
}
store_scope() {
  case "$BEADS_DIR" in
    "$CLAIM_CITY_BEADS_DIR") printf city ;;
    "$CLAIM_RIG_BEADS_DIR") printf rig ;;
    *) printf unknown ;;
  esac
}
if [ "${1:-}" = "--dolt-auto-commit" ]; then
  shift 2
fi
case "${1:-}" in
  show)
    id="${3:-}"
    if [ "$id" = "` + claimIdentityRaceSessionID + `" ]; then
	  printf '%s\n' "$*" >> "$CLAIM_SOCKET_ATTEMPT_LOG"
      if [ "$(store_scope)" = city ] && identity_ready; then
        printf '%s\n' "$session_json"
        exit 0
      fi
      exit 1
    fi
    if [ "$id" = "` + claimIdentityRaceWorkID + `" ]; then
      [ "$(store_scope)" = rig ] || exit 1
      if [ -d "$CLAIM_LOCK" ]; then printf '%s\n' "$claimed_work"; else printf '%s\n' "$open_work"; fi
      exit 0
    fi
    exit 1
    ;;
  list|query)
    printf '[]\n'
    ;;
  ready)
    [ "$(store_scope)" = rig ] || { printf '[]\n'; exit 0; }
    if [ -d "$CLAIM_LOCK" ]; then printf '[]\n'; else printf '%s\n' "$open_work"; fi
    ;;
  update)
	printf 'scope=%s beads_dir=%s args=%s\n' "$(store_scope)" "$BEADS_DIR" "$*" >> "$CLAIM_CALL_LOG"
    if [ "$(store_scope)" = rig ] && [ "${2:-}" = "` + claimIdentityRaceWorkID + `" ] && [ "${3:-}" = "--claim" ]; then
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
	workerProvider := filepath.Join(fakeBin, "gc-beads-bd")
	if err := os.WriteFile(workerProvider, []byte(`#!/bin/sh
printf '%s\n' "$*" >> "$CLAIM_SOCKET_ATTEMPT_LOG"
case "${1:-}" in
  get)
    printf 'no issue found matching %s\n' "${2:-}" >&2
    exit 1
    ;;
  *) exit 2 ;;
esac
`), 0o755); err != nil {
		t.Fatal(err)
	}

	claimLock := filepath.Join(stateDir, "claim.lock")
	projectionReady := filepath.Join(stateDir, "projection.ready")
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CLAIM_IDENTITY_MODE", mode)
	t.Setenv("CLAIM_PROJECTION_READY", projectionReady)
	t.Setenv("CLAIM_LOCK", claimLock)
	t.Setenv("CLAIM_CALL_LOG", filepath.Join(stateDir, "claim-calls"))
	t.Setenv("CLAIM_SOCKET_ATTEMPT_LOG", filepath.Join(stateDir, "socket-attempts"))
	t.Setenv("CLAIM_CITY_BEADS_DIR", filepath.Join(cityDir, ".beads"))
	t.Setenv("CLAIM_RIG_BEADS_DIR", filepath.Join(rigDir, ".beads"))
	t.Setenv("BEADS_DIR", filepath.Join(rigDir, ".beads"))
	t.Setenv("GC_BEADS_SCOPE_ROOT", rigDir)
	t.Setenv("GC_RIG", "hpfetcher")
	t.Setenv("GC_RIG_ROOT", rigDir)
	setFenceClaimEnv(t, cityDir, claimIdentityRaceSessionID, claimIdentityRaceToken)
	t.Setenv("GC_RUNTIME_EPOCH", "1")
	t.Setenv("GC_TEMPLATE", claimIdentityRaceRoute)
	t.Setenv("GC_ALIAS", claimIdentityRaceRoute+"-1")
	t.Setenv("GC_AGENT", claimIdentityRaceRoute+"-1")
	t.Setenv("GC_SESSION_NAME", "worker-1")
	t.Setenv("GC_SESSION_ORIGIN", "ephemeral")
	if mode == "present" {
		writeClaimIdentityRaceProjection(t, cityDir, claimIdentityRaceSessionID, claimIdentityRaceToken, 1, session.StateActive)
	}

	return cityDir, claimLock, projectionReady, workerProvider
}

func decodeClaimIdentityRaceResult(t *testing.T, stdout *bytes.Buffer) hookClaimJSONResult {
	t.Helper()
	var result hookClaimJSONResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		t.Fatalf("stdout is not a JSON claim result: %v\n%s", err, stdout.String())
	}
	return result
}

type delayedSessionProjectionStore struct {
	*beads.MemStore
	projected      chan struct{}
	projectionRead chan struct{}
}

func newDelayedSessionProjectionStore() *delayedSessionProjectionStore {
	return &delayedSessionProjectionStore{
		MemStore:       beads.NewMemStore(),
		projected:      make(chan struct{}),
		projectionRead: make(chan struct{}, 8),
	}
}

func (s *delayedSessionProjectionStore) Get(id string) (beads.Bead, error) {
	if id == claimIdentityRaceSessionID {
		select {
		case s.projectionRead <- struct{}{}:
		default:
		}
		select {
		case <-s.projected:
		default:
			return beads.Bead{}, fmt.Errorf("projecting session %q: %w", id, beads.ErrNotFound)
		}
	}
	return s.MemStore.Get(id)
}

type claimLifecycleStartProvider struct {
	*runtime.Fake
	started        chan struct{}
	startCalls     atomic.Int32
	mu             sync.Mutex
	results        []hookClaimJSONResult
	resultErrors   []error
	identityErrors int
}

func newClaimLifecycleStartProvider() *claimLifecycleStartProvider {
	return &claimLifecycleStartProvider{
		Fake:    runtime.NewFake(),
		started: make(chan struct{}, 4),
	}
}

func (p *claimLifecycleStartProvider) Start(ctx context.Context, name string, cfg runtime.Config) error {
	p.startCalls.Add(1)
	if err := p.Fake.Start(ctx, name, cfg); err != nil {
		return err
	}
	p.started <- struct{}{}

	type hookOutcome struct {
		result        hookClaimJSONResult
		err           error
		identityError bool
	}
	outcomes := make(chan hookOutcome, 2)
	for range 2 {
		go func() {
			var stdout, stderr bytes.Buffer
			code := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &stdout, &stderr)
			var result hookClaimJSONResult
			err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result)
			if err == nil && code != 0 && result.Reason != hookClaimReasonNoWork && result.Reason != hookClaimReasonClaimsErrored {
				err = fmt.Errorf("claim hook code=%d result=%+v stderr=%s", code, result, stderr.String())
			}
			outcomes <- hookOutcome{
				result:        result,
				err:           err,
				identityError: strings.Contains(stderr.String(), "session identity unavailable"),
			}
		}()
	}

	identityErrors := 0
	p.mu.Lock()
	for range 2 {
		outcome := <-outcomes
		p.results = append(p.results, outcome.result)
		p.resultErrors = append(p.resultErrors, outcome.err)
		if outcome.identityError {
			identityErrors++
		}
	}
	p.identityErrors += identityErrors
	p.mu.Unlock()
	if identityErrors > 0 {
		_ = p.Stop(name)
	}
	return nil
}

func (p *claimLifecycleStartProvider) snapshot() (starts, claimed, identityErrors int, errs []error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, result := range p.results {
		if result.Action == "work" && result.Reason == "claimed" && result.BeadID == claimIdentityRaceWorkID {
			claimed++
		}
	}
	return int(p.startCalls.Load()), claimed, p.identityErrors, append([]error(nil), p.resultErrors...)
}

func TestPreparedSessionStartWaitsForClaimIdentityProjection(t *testing.T) {
	cityDir, claimLock, _, _ := newClaimIdentityRaceFixture(t, "never")
	backing := newDelayedSessionProjectionStore()
	store := beads.NewCachingStoreForTest(backing, nil)

	created, err := session.NewStore(beads.SessionStore{Store: store}).CreateSessionInfo(session.CreateSpec{
		Title:     "worker",
		AgentName: claimIdentityRaceRoute,
		Metadata: map[string]string{
			"session_name":         "worker-1",
			"template":             claimIdentityRaceRoute,
			"state":                string(session.StateStartPending),
			"provider":             "fake",
			"command":              "test-provider-command",
			"instance_token":       claimIdentityRaceToken,
			"generation":           "1",
			"continuation_epoch":   "1",
			"pending_create_claim": "true",
		},
	})
	if err != nil {
		t.Fatalf("create pending session: %v", err)
	}
	if created.ID != claimIdentityRaceSessionID {
		t.Fatalf("created session id = %q, want %q", created.ID, claimIdentityRaceSessionID)
	}
	// CachingStore.Create performs one authoritative refresh immediately after
	// the write. Consume that known-missing projection read so the next signal is
	// specifically the pre-provider readiness barrier.
	select {
	case <-backing.projectionRead:
	case <-time.After(3 * time.Second):
		t.Fatal("session create did not exercise the delayed authoritative projection")
	}

	provider := newClaimLifecycleStartProvider()
	projectionPath := writeClaimIdentityRaceProjection(t, cityDir, created.ID, "superseded-token", 0, session.StateSuspended)
	item := preparedStart{
		candidate: startCandidate{
			info: created,
			tp:   TemplateParams{TemplateName: claimIdentityRaceRoute},
		},
		cfg: runtime.Config{Command: "test-provider-command"},
	}
	startOnce := func() <-chan startResult {
		done := make(chan startResult, 1)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			done <- runPreparedStartCandidate(ctx, item, cityDir, provider, store, nil, 2*time.Second, nil, nil)
		}()
		return done
	}

	firstDone := startOnce()
	providerStartedBeforeProjection := false
	select {
	case <-backing.projectionRead:
		select {
		case <-provider.started:
			providerStartedBeforeProjection = true
		default:
		}
	case <-provider.started:
		providerStartedBeforeProjection = true
	}
	if providerStartedBeforeProjection {
		// Keep projection unavailable until the doomed hook finishes. This makes
		// the old ordering reproduce the real claims_errored drain instead of
		// allowing its bounded retry loop to hide the race.
		result := <-firstDone
		if result.err != nil {
			t.Fatalf("first provider start: %v", result.err)
		}
		firstDone = nil
	}
	close(backing.projected)
	if firstDone != nil {
		if result := <-firstDone; result.err != nil {
			t.Fatalf("first provider start: %v", result.err)
		}
	}
	// A drained identity-error worker is absent on the next reconciliation and
	// therefore gets respawned. A healthy first start remains live and never
	// enters the start path again.
	if !provider.IsRunning(created.SessionName) {
		if result := <-startOnce(); result.err != nil {
			t.Fatalf("converged provider start: %v", result.err)
		}
	}

	starts, claimed, identityErrors, resultErrs := provider.snapshot()
	for _, resultErr := range resultErrs {
		if resultErr != nil {
			t.Errorf("claim hook result: %v", resultErr)
		}
	}
	if providerStartedBeforeProjection {
		t.Error("provider started before its session identity was durably readable")
	}
	if claimed != 1 {
		t.Errorf("claimed routed bead %d times, want exactly once", claimed)
	}
	if identityErrors != 0 {
		t.Errorf("identity-error drains = %d, want zero", identityErrors)
	}
	if starts != 1 {
		t.Errorf("provider starts = %d, want one initial start and zero respawns", starts)
	}
	if _, err := os.Stat(claimLock); err != nil {
		t.Errorf("claim mutation did not commit: %v", err)
	}
	projectionBytes, err := os.ReadFile(projectionPath)
	if err != nil {
		t.Fatalf("read reconciled session-fence projection: %v", err)
	}
	if bytes.Contains(projectionBytes, []byte(claimIdentityRaceToken)) {
		t.Fatalf("session-fence projection leaked raw instance token: %s", projectionBytes)
	}
	if bytes.Contains(projectionBytes, []byte("superseded-token")) {
		t.Fatalf("boot reconciliation left superseded projection in place: %s", projectionBytes)
	}
}

// TestReconcileSessionBeads_AdoptedLiveUpgradePublishesClaimFence reproduces
// the supervisor-upgrade boundary: the provider already owns a live worker and
// the city store already owns its active session bead, but the old binary never
// wrote a worker-readable claim-fence projection. Reconciliation must publish
// that projection without restarting the adopted runtime, so its first hook
// under the new binary can claim rig-scoped work normally.
func TestReconcileSessionBeads_AdoptedLiveUpgradePublishesClaimFence(t *testing.T) {
	cityDir, claimLock, projectionReady, _ := newClaimIdentityRaceFixture(t, "barrier")
	if err := os.WriteFile(projectionReady, []byte("ready\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := beads.NewMemStore()
	bead, err := store.Create(beads.Bead{
		ID:     claimIdentityRaceSessionID,
		Title:  "worker-1",
		Type:   session.BeadType,
		Labels: []string{"gc:session", "agent:worker-1"},
		Metadata: map[string]string{
			"session_name":   "worker-1",
			"agent_name":     "worker-1",
			"template":       claimIdentityRaceRoute,
			"state":          string(session.StateActive),
			"instance_token": claimIdentityRaceToken,
			"generation":     "1",
		},
	})
	if err != nil {
		t.Fatalf("create adopted session bead: %v", err)
	}
	provider := runtime.NewFake()
	if err := provider.Start(context.Background(), "worker-1", runtime.Config{Command: "test-cmd"}); err != nil {
		t.Fatalf("seed already-live provider: %v", err)
	}
	startsBefore := provider.CountCalls("Start", "worker-1")
	desired := map[string]TemplateParams{
		"worker-1": {
			Command:      "test-cmd",
			SessionName:  "worker-1",
			TemplateName: claimIdentityRaceRoute,
		},
	}
	cfg := &config.City{Agents: []config.Agent{{Name: claimIdentityRaceRoute}}}
	var reconcileStdout, reconcileStderr bytes.Buffer
	reconcileSessionBeadsAtPath(
		context.Background(), cityDir, []beads.Bead{bead}, desired,
		configuredSessionNames(cfg, "", store), cfg, provider, store, nil, nil, nil,
		nil, newDrainTracker(), map[string]int{claimIdentityRaceRoute: 1}, false, nil,
		"", nil, &clock.Fake{Time: time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)},
		events.Discard, 0, 0, &reconcileStdout, &reconcileStderr,
		withStartStabilityWaiter(immediateStartStabilityWaiter),
		withSessionStaleKeyDetectionWaiter(immediateSessionStaleKeyDetectionWaiter),
	)
	if got := provider.CountCalls("Start", "worker-1"); got != startsBefore {
		t.Fatalf("provider Start calls = %d, want %d (live runtime must be adopted, not restarted); stderr=%s", got, startsBefore, reconcileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &stdout, &stderr)
	result := decodeClaimIdentityRaceResult(t, &stdout)
	if code != 0 || result.Action != "work" || result.Reason != "claimed" || result.BeadID != claimIdentityRaceWorkID {
		t.Fatalf("adopted worker first claim = code %d result %+v stderr=%s; want routed work claimed without claims_errored", code, result, stderr.String())
	}
	if _, err := os.Stat(claimLock); err != nil {
		t.Fatalf("adopted worker claim did not commit: %v", err)
	}
}

func TestReconcileSessionBeads_AdoptedLiveIneligibleFenceDoesNotStrandTeardown(t *testing.T) {
	tests := []struct {
		name           string
		persistedState session.State
		metadata       func(time.Time) map[string]string
	}{
		{
			name: "draining",
			// A live drain is acknowledged in provider state before the
			// reconciler commits drain-ack-stop-pending to the bead. The
			// preserved tombstone is what makes this active snapshot
			// lifecycle-ineligible for claims during that handoff.
			persistedState: session.StateActive,
			metadata: func(time.Time) map[string]string {
				return nil
			},
		},
		{
			name:           "suspended",
			persistedState: session.StateSuspended,
			metadata: func(now time.Time) map[string]string {
				return map[string]string{
					"held_until":   now.Add(2 * time.Hour).UTC().Format(time.RFC3339),
					"sleep_intent": "user-hold",
				}
			},
		},
		{
			name:           "quarantined",
			persistedState: session.StateQuarantined,
			metadata: func(now time.Time) map[string]string {
				return map[string]string{
					"quarantined_until": now.Add(2 * time.Hour).UTC().Format(time.RFC3339),
					"sleep_reason":      "quarantine",
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cityDir := t.TempDir()
			store := beads.NewMemStore()
			provider := runtime.NewFake()
			clk := &clock.Fake{Time: time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)}
			metadata := map[string]string{
				"session_name":   "worker-1",
				"agent_name":     "worker-1",
				"template":       "worker",
				"state":          string(tc.persistedState),
				"instance_token": claimIdentityRaceToken,
				"generation":     "1",
			}
			for key, value := range tc.metadata(clk.Now()) {
				metadata[key] = value
			}
			bead, err := store.Create(beads.Bead{
				ID:       claimIdentityRaceSessionID,
				Title:    "worker-1",
				Type:     session.BeadType,
				Labels:   []string{"gc:session", "agent:worker-1"},
				Metadata: metadata,
			})
			if err != nil {
				t.Fatalf("create %s session bead: %v", tc.name, err)
			}
			if err := session.TombstoneSessionFenceProjection(cityDir, bead.ID, claimIdentityRaceToken, 1); err != nil {
				t.Fatalf("tombstone %s claim fence: %v", tc.name, err)
			}
			if err := provider.Start(context.Background(), "worker-1", runtime.Config{Command: "test-cmd"}); err != nil {
				t.Fatalf("seed %s live provider: %v", tc.name, err)
			}
			if err := provider.SetMeta("worker-1", "GC_SESSION_ID", bead.ID); err != nil {
				t.Fatalf("set %s session id: %v", tc.name, err)
			}
			if err := provider.SetMeta("worker-1", "GC_INSTANCE_TOKEN", claimIdentityRaceToken); err != nil {
				t.Fatalf("set %s instance token: %v", tc.name, err)
			}

			desired := map[string]TemplateParams{
				"worker-1": {
					Command:      "test-cmd",
					SessionName:  "worker-1",
					TemplateName: "worker",
				},
			}
			cfg := &config.City{Agents: []config.Agent{{Name: "worker"}}}
			drainOps := newFakeDrainOps()
			if err := drainOps.setDrainAck("worker-1"); err != nil {
				t.Fatalf("ack %s drain: %v", tc.name, err)
			}
			drains := newDrainTracker()
			asyncStops := &asyncStartTracker{}
			var stdout, stderr bytes.Buffer
			reconcileSessionBeadsAtPath(
				context.Background(), cityDir, []beads.Bead{bead}, desired,
				configuredSessionNames(cfg, "", store), cfg, provider, store, drainOps, nil, nil,
				nil, drains, map[string]int{}, false, nil, "", nil, clk,
				events.Discard, 0, 0, &stdout, &stderr,
				withAsyncDrainAckStopTracker(asyncStops),
				withStartStabilityWaiter(immediateStartStabilityWaiter),
				withSessionStaleKeyDetectionWaiter(immediateSessionStaleKeyDetectionWaiter),
			)
			projection, err := session.LoadSessionFenceProjection(cityDir, bead.ID)
			if err != nil {
				t.Fatalf("load %s claim fence after drain tick: %v", tc.name, err)
			}
			if projection.State != "tombstoned" || projection.ClaimEligible() {
				t.Fatalf("%s claim fence after drain tick = state %q eligible=%t, want preserved tombstone", tc.name, projection.State, projection.ClaimEligible())
			}
			if strings.Contains(stderr.String(), "publishing adopted live session claim fence") {
				t.Fatalf("%s expected claim-fence refusal was treated as a publish failure: %s", tc.name, stderr.String())
			}
			if !asyncStops.wait(time.Second) {
				t.Fatalf("%s drain-ack stop did not complete; stderr=%s", tc.name, stderr.String())
			}
			if provider.IsRunning("worker-1") {
				t.Fatalf("%s live runtime remained stranded after drain-ack; stderr=%s", tc.name, stderr.String())
			}
			current, err := store.Get(bead.ID)
			if err != nil {
				t.Fatalf("reload %s session after stop: %v", tc.name, err)
			}
			if current.Metadata["state"] != string(session.StateDraining) {
				t.Fatalf("%s state after drain-ack = %q, want draining", tc.name, current.Metadata["state"])
			}
			if current.Metadata["state_reason"] != session.DrainAckStopPendingReason {
				t.Fatalf("%s state reason after drain-ack = %q, want %q", tc.name, current.Metadata["state_reason"], session.DrainAckStopPendingReason)
			}
			projection, err = session.LoadSessionFenceProjection(cityDir, bead.ID)
			if err != nil {
				t.Fatalf("load %s claim fence after stop tick: %v", tc.name, err)
			}
			if projection.State != "tombstoned" || projection.ClaimEligible() {
				t.Fatalf("%s claim fence after stop tick = state %q eligible=%t, want preserved tombstone", tc.name, projection.State, projection.ClaimEligible())
			}
		})
	}
}

func TestReconcileSessionBeads_AdoptedLiveEligiblePublishFailureRemainsFailClosed(t *testing.T) {
	cityDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityDir, ".gc", "runtime"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cityDir, ".gc", "runtime", "session-fence"), []byte("blocks projection directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := beads.NewMemStore()
	bead, err := store.Create(beads.Bead{
		ID:     claimIdentityRaceSessionID,
		Title:  "worker-1",
		Type:   session.BeadType,
		Labels: []string{"gc:session", "agent:worker-1"},
		Metadata: map[string]string{
			"session_name":      "worker-1",
			"agent_name":        "worker-1",
			"template":          "worker",
			"state":             string(session.StateActive),
			"instance_token":    claimIdentityRaceToken,
			"generation":        "1",
			"restart_requested": "true",
		},
	})
	if err != nil {
		t.Fatalf("create eligible session bead: %v", err)
	}
	provider := runtime.NewFake()
	if err := provider.Start(context.Background(), "worker-1", runtime.Config{Command: "test-cmd"}); err != nil {
		t.Fatalf("seed eligible live provider: %v", err)
	}
	startsBefore := provider.CountCalls("Start", "worker-1")
	desired := map[string]TemplateParams{
		"worker-1": {Command: "test-cmd", SessionName: "worker-1", TemplateName: "worker"},
	}
	cfg := &config.City{Agents: []config.Agent{{Name: "worker"}}}
	drains := newDrainTracker()
	var stdout, stderr bytes.Buffer
	reconcileSessionBeadsAtPath(
		context.Background(), cityDir, []beads.Bead{bead}, desired,
		configuredSessionNames(cfg, "", store), cfg, provider, store, nil, nil, nil,
		nil, drains, map[string]int{}, false, nil, "", nil,
		&clock.Fake{Time: time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)},
		events.Discard, 0, 0, &stdout, &stderr,
	)
	if drain := drains.get(bead.ID); drain != nil {
		t.Fatalf("eligible session entered teardown after an unexpected projection failure: %+v", drain)
	}
	if got := provider.CountCalls("Stop", "worker-1"); got != 0 {
		t.Fatalf("restart-requested eligible session was stopped after an unexpected projection failure: Stop calls = %d", got)
	}
	if !provider.IsRunning("worker-1") {
		t.Fatal("eligible session runtime stopped after an unexpected projection failure")
	}
	if got := provider.CountCalls("Start", "worker-1"); got != startsBefore {
		t.Fatalf("eligible session runtime was respawned without a claim-fence projection: Start calls = %d, want %d", got, startsBefore)
	}
	current, err := store.Get(bead.ID)
	if err != nil {
		t.Fatalf("reload eligible session bead: %v", err)
	}
	if got := current.Metadata["restart_requested"]; got != "true" {
		t.Fatalf("restart_requested = %q, want preserved while projection is unavailable", got)
	}
	if !strings.Contains(stderr.String(), "publishing adopted live session claim fence") {
		t.Fatalf("stderr = %q, want fail-closed projection diagnostic", stderr.String())
	}
}

func TestHookCommandClaimIdentityFailureIsNeverNoWork(t *testing.T) {
	_, claimLock, _, _ := newClaimIdentityRaceFixture(t, "never")

	var stdout, stderr bytes.Buffer
	code := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &stdout, &stderr)
	result := decodeClaimIdentityRaceResult(t, &stdout)
	if code != 1 {
		t.Fatalf("code = %d, want 1; result=%+v stderr=%s", code, result, stderr.String())
	}
	if result.Action != "drain" || result.Reason != hookClaimReasonClaimsErrored {
		t.Fatalf("identity-read exhaustion = %+v, want drain/claims_errored (never no_work); stderr=%s", result, stderr.String())
	}
	if _, err := os.Stat(claimLock); !os.IsNotExist(err) {
		t.Fatalf("claim mutation committed without city session identity; stat error = %v", err)
	}
}

func TestHookCommandClaimUsesCityIdentityStoreAndRigWorkStore(t *testing.T) {
	cityDir, claimLock, _, workerProvider := newClaimIdentityRaceFixture(t, "present")
	// Reproduce the launched worker's ambient provider/store selection: its
	// default handle is rig-scoped, while GC_CITY remains the explicit path to
	// the separate city store that owns session identity beads.
	t.Setenv("GC_BEADS", "exec:"+workerProvider)
	t.Setenv("GC_BEADS_SCOPE_ROOT", "")
	if err := os.Chmod(filepath.Join(cityDir, ".gc"), 0o555); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cityDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(cityDir, 0o755)
		_ = os.Chmod(filepath.Join(cityDir, ".gc"), 0o755)
	})

	var stdout, stderr bytes.Buffer
	code := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &stdout, &stderr)
	result := decodeClaimIdentityRaceResult(t, &stdout)
	if code != 0 || result.Action != "work" || result.Reason != "claimed" || result.BeadID != claimIdentityRaceWorkID {
		_, claimErr := os.Stat(claimLock)
		claimCalls, _ := os.ReadFile(os.Getenv("CLAIM_CALL_LOG"))
		t.Fatalf("split-store claim = code %d result %+v claim_lock=%v calls=%q, want one claimed rig work result; stderr=%s",
			code, result, claimErr, claimCalls, stderr.String())
	}
	if strings.Contains(stderr.String(), "session identity unavailable") {
		t.Fatalf("city session identity read drained under rig worker environment: %s", stderr.String())
	}
	if attempts, err := os.ReadFile(os.Getenv("CLAIM_SOCKET_ATTEMPT_LOG")); err == nil && len(bytes.TrimSpace(attempts)) > 0 {
		t.Fatalf("claim fence attempted city-store/socket access: %q", attempts)
	}
	claimCalls, err := os.ReadFile(os.Getenv("CLAIM_CALL_LOG"))
	if err != nil {
		t.Fatalf("read claim call log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(claimCalls)), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], "scope=rig ") ||
		!strings.Contains(lines[0], "beads_dir="+os.Getenv("CLAIM_RIG_BEADS_DIR")+" ") {
		t.Fatalf("claim mutations = %q, want exactly one against the rig store", claimCalls)
	}
}

func TestHookCommandClaimRejectsReplacedTokenAndAdmitsOnlyReplacement(t *testing.T) {
	cityDir, claimLock, _, _ := newClaimIdentityRaceFixture(t, "never")
	projectionPath := writeClaimIdentityRaceProjection(t, cityDir, claimIdentityRaceSessionID, claimIdentityRaceToken, 1, session.StateActive)
	projectionBytes, err := os.ReadFile(projectionPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(projectionBytes, []byte(claimIdentityRaceToken)) {
		t.Fatalf("projection contains raw token: %s", projectionBytes)
	}

	writeClaimIdentityRaceProjection(t, cityDir, claimIdentityRaceSessionID, claimIdentityRaceToken, 1, session.StateSuspended)
	replacementToken := "replacement-instance-token"
	writeClaimIdentityRaceProjection(t, cityDir, claimIdentityRaceSessionID, replacementToken, 2, session.StateActive)

	var staleStdout, staleStderr bytes.Buffer
	staleCode := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &staleStdout, &staleStderr)
	staleResult := decodeClaimIdentityRaceResult(t, &staleStdout)
	if staleCode != 1 || staleResult.Reason != hookClaimReasonStaleSession {
		t.Fatalf("superseded runtime = code %d result %+v stderr=%s, want stale refusal", staleCode, staleResult, staleStderr.String())
	}

	t.Setenv("GC_INSTANCE_TOKEN", replacementToken)
	t.Setenv("GC_RUNTIME_EPOCH", "2")
	var freshStdout, freshStderr bytes.Buffer
	freshCode := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &freshStdout, &freshStderr)
	freshResult := decodeClaimIdentityRaceResult(t, &freshStdout)
	if freshCode != 0 || freshResult.Action != "work" || freshResult.BeadID != claimIdentityRaceWorkID {
		t.Fatalf("replacement runtime = code %d result %+v stderr=%s, want one claim", freshCode, freshResult, freshStderr.String())
	}
	if _, err := os.Stat(claimLock); err != nil {
		t.Fatalf("replacement did not commit rig-scoped claim: %v", err)
	}
}

func TestHookCommandClaimRejectsIneligibleProjectedStates(t *testing.T) {
	for _, state := range []session.State{session.StateClosed, session.StateSuspended, session.StateDraining, session.StateDrained} {
		t.Run(string(state), func(t *testing.T) {
			cityDir, claimLock, _, _ := newClaimIdentityRaceFixture(t, "never")
			writeClaimIdentityRaceProjection(t, cityDir, claimIdentityRaceSessionID, claimIdentityRaceToken, 1, state)
			var stdout, stderr bytes.Buffer
			code := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &stdout, &stderr)
			result := decodeClaimIdentityRaceResult(t, &stdout)
			if code != 1 || result.Reason != hookClaimReasonStaleSession {
				t.Fatalf("state %q = code %d result %+v stderr=%s, want stale refusal", state, code, result, stderr.String())
			}
			if _, err := os.Stat(claimLock); !os.IsNotExist(err) {
				t.Fatalf("state %q reached claim mutation: %v", state, err)
			}
		})
	}
}

func TestHookCommandClaimMalformedProjectionReportsClaimsErrored(t *testing.T) {
	for _, tc := range []struct {
		name  string
		write func(*testing.T, string)
	}{
		{name: "corrupt", write: func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "oversized", write: func(t *testing.T, path string) {
			if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 65<<10), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "symlink", write: func(t *testing.T, path string) {
			target := filepath.Join(t.TempDir(), "projection.json")
			if err := os.WriteFile(target, []byte(`{"schema_version":1}`), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cityDir, claimLock, _, _ := newClaimIdentityRaceFixture(t, "never")
			path := claimIdentityRaceProjectionPath(cityDir, claimIdentityRaceSessionID)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			tc.write(t, path)
			var stdout, stderr bytes.Buffer
			code := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &stdout, &stderr)
			result := decodeClaimIdentityRaceResult(t, &stdout)
			if code != 1 || result.Reason != hookClaimReasonClaimsErrored {
				t.Fatalf("%s projection = code %d result %+v stderr=%s, want claims_errored", tc.name, code, result, stderr.String())
			}
			if _, err := os.Stat(claimLock); !os.IsNotExist(err) {
				t.Fatalf("%s projection reached claim mutation: %v", tc.name, err)
			}
		})
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

// TestHookCommandClaimTokenlessRuntimeFailsClosed proves environment identity is
// never trusted as a compatibility escape hatch. A managed runtime that names a
// session but carries no token cannot validate the controller projection.
func TestHookCommandClaimTokenlessRuntimeFailsClosed(t *testing.T) {
	clearGCEnv(t)
	disableManagedDoltRecoveryForTest(t)
	t.Setenv("GC_BEADS", "file")
	cityDir := writeFenceTestCity(t)
	sessionID := newFenceSessionBead(t, cityDir, session.StateActive, "legacy-token")
	queryMarker := installFenceWorkQueryProbe(t)
	setFenceClaimEnv(t, cityDir, sessionID, "")

	var stdout, stderr bytes.Buffer
	code := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &stdout, &stderr)

	if _, err := os.Stat(queryMarker); !os.IsNotExist(err) {
		t.Fatalf("work query ran for token-less runtime: %v; stderr=%s", err, stderr.String())
	}
	if code != 1 {
		t.Fatalf("code = %d, want 1; stderr=%s", code, stderr.String())
	}
	var result hookClaimJSONResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		t.Fatalf("stdout is not a JSON result: %v\n%s", err, stdout.String())
	}
	if result.Action != "drain" || result.Reason != hookClaimReasonClaimsErrored {
		t.Fatalf("result = %+v, want action=drain reason=claims_errored", result)
	}
}

// TestHookCommandClaimAbsentSessionBeadReportsClaimsErrored proves a runtime
// whose session bead stays absent through the bounded startup retry is refused
// before the work query with an honest operational result, never no_work.
func TestHookCommandClaimAbsentSessionBeadReportsClaimsErrored(t *testing.T) {
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
	if result.Action != "drain" || result.Reason != hookClaimReasonClaimsErrored {
		t.Fatalf("result = %+v, want action=drain reason=claims_errored", result)
	}
	if !strings.Contains(stderr.String(), "session identity unavailable") ||
		!strings.Contains(stderr.String(), "not found") {
		t.Fatalf("stderr = %q, want exhausted identity diagnostic naming the missing bead", stderr.String())
	}
	if _, err := os.Stat(queryMarker); !os.IsNotExist(err) {
		t.Fatalf("work query ran for a session with no bead; stat error = %v", err)
	}
}

// TestHookCommandClaimProjectionDoesNotOpenCityStore proves the readable
// controller projection is the entire worker-side identity authority: even a
// corrupt/unopenable city store cannot make the hook dial or query it.
func TestHookCommandClaimProjectionDoesNotOpenCityStore(t *testing.T) {
	clearGCEnv(t)
	disableManagedDoltRecoveryForTest(t)
	t.Setenv("GC_BEADS", "file")
	cityDir := writeFenceTestCity(t)
	sessionID := newFenceSessionBead(t, cityDir, session.StateActive, "current-token")
	queryMarker := installFenceWorkQueryProbe(t)
	if err := os.WriteFile(filepath.Join(cityDir, ".gc", "beads.json"), []byte("{ this is not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	setFenceClaimEnv(t, cityDir, sessionID, "current-token")

	var stdout, stderr bytes.Buffer
	code := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &stdout, &stderr)

	if _, err := os.Stat(queryMarker); err != nil {
		t.Fatalf("work query did not run with a valid projection: %v", err)
	}
	if strings.Contains(stderr.String(), "session identity unavailable") {
		t.Fatalf("projection reader attempted city store: %s", stderr.String())
	}
	if code != 1 {
		t.Fatalf("code = %d, want 1 (JSON claims-errored drain without --drain-ack)", code)
	}
	var result hookClaimJSONResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		t.Fatalf("stdout is not a JSON result: %v\n%s", err, stdout.String())
	}
	if result.Action != "drain" || result.Reason != hookClaimReasonNoWork {
		t.Fatalf("result = %+v, want action=drain reason=no_work", result)
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

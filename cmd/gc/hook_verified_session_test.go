package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/session"
)

// Embedding only Store makes every unimplemented operation panic. The allowlist
// is exact own-row reads and one-key conditional writes, never List/name lookup.
type ownSessionOnlyStore struct {
	beads.Store
	mem           *beads.MemStore
	t             *testing.T
	id            string
	reads, writes int
	readError     error
	beforeGet     func()
}

func (s *ownSessionOnlyStore) Get(id string) (beads.Bead, error) {
	s.t.Helper()
	if id != s.id {
		s.t.Fatalf("foreign session read %q", id)
	}
	s.reads++
	if s.beforeGet != nil {
		f := s.beforeGet
		s.beforeGet = nil
		f()
	}
	if s.readError != nil {
		return beads.Bead{}, s.readError
	}
	return s.mem.Get(id)
}

func (s *ownSessionOnlyStore) ConditionalWriterHandle() (beads.ConditionalWriter, bool) {
	return ownSessionWriter{s}, true
}

type ownSessionWriter struct{ s *ownSessionOnlyStore }

func (w ownSessionWriter) UpdateIfMatch(id string, revision int64, opts beads.UpdateOpts) error {
	w.s.t.Helper()
	if id != w.s.id || revision == 0 || len(opts.Metadata) != 1 || opts.Status != nil || opts.Assignee != nil || opts.Type != nil || opts.Title != nil || opts.Description != nil || opts.Priority != nil || opts.ParentID != nil || len(opts.Labels)+len(opts.RemoveLabels) != 0 {
		w.s.t.Fatal("session write escaped own-row claim contract")
	}
	if _, ok := opts.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey]; !ok {
		w.s.t.Fatal("foreign metadata write")
	}
	w.s.writes++
	return w.s.mem.UpdateIfMatch(id, revision, opts)
}
func (w ownSessionWriter) CloseIfMatch(string, int64) error { w.s.t.Fatal("session close"); return nil }
func (w ownSessionWriter) DeleteIfMatch(string, int64) error {
	w.s.t.Fatal("session delete")
	return nil
}

func (w ownSessionWriter) CompareAndSetMetadataKey(string, string, string, string) (bool, error) {
	w.s.t.Fatal("unguarded metadata CAS")
	return false, nil
}

func newVerifiedHookFixture(t *testing.T) (*VerifiedSession, *ownSessionOnlyStore) {
	t.Helper()
	mem := beads.NewMemStoreFrom(1, []beads.Bead{
		{ID: "s-own", Type: "session", Status: "open", Revision: 1, Metadata: map[string]string{
			"instance_token": "own-fixture", "generation": "1", "state": "active", "template": "rig-a/worker",
			"session_name": "reused-name", "alias": "rig-a/worker-1",
		}},
		{ID: "s-other", Type: "session", Status: "open", Revision: 1, Metadata: map[string]string{
			"instance_token": "other-fixture", "generation": "2", "state": "active", "template": "rig-b/worker",
			"session_name": "reused-name",
		}},
	}, nil)
	store := &ownSessionOnlyStore{mem: mem, t: t, id: "s-own"}
	front := session.NewStore(beads.SessionStore{Store: store})
	id, err := front.VerifySession("s-own", "own-fixture", 1)
	if err != nil {
		t.Fatal(err)
	}
	return &VerifiedSession{VerifiedSession: id, Rig: "rig-a", front: front}, store
}

func TestVerifiedHookEnvironmentRefusesForeignNamesAndTrailingAgent(t *testing.T) {
	v, store := newVerifiedHookFixture(t)
	for _, key := range []string{"GC_TMUX_SESSION", "GC_SESSION_NAME", "GC_TEMPLATE", "GC_ALIAS", "GC_AGENT"} {
		t.Run(key, func(t *testing.T) {
			if err := v.checkEnvironment([]string{key + "=rig-b/worker"}, nil); err == nil {
				t.Fatal("foreign identity accepted")
			}
		})
	}
	if err := v.checkEnvironment(nil, []string{"rig-b/worker"}); err == nil {
		t.Fatal("foreign positional agent accepted")
	}
	if err := v.checkEnvironment([]string{"GC_TEMPLATE=rig-a/worker", "GC_SESSION_NAME=reused-name"}, []string{"rig-a/worker"}); err != nil {
		t.Fatal(err)
	}
	if store.writes != 0 {
		t.Fatal("environment refusal wrote session")
	}
}

func TestVerifiedHookStampRaceUnwindsOnlyPendingClaim(t *testing.T) {
	v, store := newVerifiedHookFixture(t)
	bead := beads.Bead{ID: "work-own", Assignee: v.ID, Status: "in_progress", Revision: 1}
	mem := beads.NewMemStoreFrom(1, []beads.Bead{bead}, nil)
	work := &releaseWorkCASRace{MemStore: mem, beforeWrite: func() {
		if err := store.mem.SetMetadata(v.ID, "instance_token", "successor"); err != nil {
			t.Fatal(err)
		}
	}}
	installVerifiedWorkStore(t, v, work)
	releases, acknowledgements := 0, 0
	ops := hookClaimOps{
		EmitClaimReleased: func(record hookClaimReleaseRecord) {
			if record.BeadID != "work-own" || record.Assignee != v.ID {
				t.Fatal("foreign release")
			}
			releases++
		},
		DrainAck: func(io.Writer) error { acknowledgements++; return nil },
	}
	ops.applyDefaults()
	var out, diagnostic bytes.Buffer
	code := writeHookClaimWorkResultForBead(hookClaimJSONResult{Action: "work", BeadID: "work-own"},
		bead,
		hookClaimOptions{Verified: v, Assignee: v.ID, JSON: true, DrainAck: true}, ops, "", true, &out, &diagnostic)
	result := decodeClaimIdentityRaceResult(t, &out)
	if code != 1 || result.Reason != hookClaimReasonStaleSession || releases != 1 || acknowledgements != 0 || store.writes != 0 {
		t.Fatalf("code=%d result=%+v release=%d ack=%d writes=%d", code, result, releases, acknowledgements, store.writes)
	}
	live, _ := mem.Get(bead.ID)
	if live.Status != "open" || live.Assignee != "" {
		t.Fatal("pending work was not released")
	}
}

func TestVerifiedHookClearDoesNotOverwriteSuccessorOrDifferentClaim(t *testing.T) {
	for _, changed := range []string{"instance_token", "generation", beadmeta.CurrentClaimBeadIDMetadataKey} {
		t.Run(changed, func(t *testing.T) {
			v, store := newVerifiedHookFixture(t)
			if err := v.front.CompareAndSetCurrentClaim(v.VerifiedSession, "", "work-own"); err != nil {
				t.Fatal(err)
			}
			if err := store.mem.SetMetadata(v.ID, changed, "replacement"); err != nil {
				t.Fatal(err)
			}
			clearHookSessionCurrentClaim(hookClaimOptions{Verified: v, publishedClaim: "work-own"}, hookClaimOps{}, io.Discard)
			if store.writes != 1 {
				t.Fatal("clear overwrote successor")
			}
		})
	}
}

func TestVerifiedHookProbeErrorFailsClosed(t *testing.T) {
	v, store := newVerifiedHookFixture(t)
	store.readError = errors.New("read unavailable")
	queries := 0
	ops := hookClaimOps{Runner: func(string, string) (string, error) { queries++; return "[]", nil }}
	bindVerifiedHookOps(&ops, v)
	var out bytes.Buffer
	opts := hookClaimOptions{Verified: v, Assignee: v.ID, JSON: true, Env: []string{"GC_SESSION_ID=" + v.ID}}
	result := tryHookClaim("query", "", &opts, &ops, &out, io.Discard)
	if !result.terminal || queries != 0 || store.writes != 0 {
		t.Fatal("probe failure reached claim")
	}
}

func TestVerifiedHookReleaseRequiresBothOwnershipFieldsAndCurrentIncarnation(t *testing.T) {
	for _, stale := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "successor"}[stale], func(t *testing.T) {
			v, store := newVerifiedHookFixture(t)
			work := beads.NewMemStoreFrom(1, []beads.Bead{
				{ID: "work-own", Type: "task", Status: "in_progress", Assignee: v.ID, Revision: 1, Metadata: map[string]string{beadmeta.SessionIDMetadataKey: v.ID, verifiedHookIncarnationKey: v.workIncarnation(), verifiedHookAttemptKey: "fixture-attempt"}},
				{ID: "other-id", Type: "task", Status: "in_progress", Assignee: v.ID, Revision: 1, Metadata: map[string]string{beadmeta.SessionIDMetadataKey: "s-other"}},
				{ID: "other-assignee", Type: "task", Status: "in_progress", Assignee: "s-other", Revision: 1, Metadata: map[string]string{beadmeta.SessionIDMetadataKey: v.ID, verifiedHookIncarnationKey: v.workIncarnation(), verifiedHookAttemptKey: "fixture-attempt"}},
				{ID: "unstamped", Type: "task", Status: "in_progress", Assignee: v.ID, Revision: 1},
			}, nil)
			if stale {
				store.beforeGet = func() {
					if err := store.mem.SetMetadata(v.ID, "generation", "2"); err != nil {
						t.Fatal(err)
					}
				}
			}
			err := releaseVerifiedHookClaims(v, work)
			if stale && !errors.Is(err, session.ErrStaleSession) {
				t.Fatalf("stale release: %v", err)
			}
			if !stale && err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"work-own", "other-id", "other-assignee", "unstamped"} {
				b, _ := work.Get(id)
				want := "in_progress"
				if id == "work-own" && !stale {
					want = "open"
				}
				if b.Status != want {
					t.Fatalf("%s status %s, want %s", id, b.Status, want)
				}
			}
		})
	}
}

func TestVerifiedHookProviderAckRefusesReusedName(t *testing.T) {
	v, _ := newVerifiedHookFixture(t)
	sp := runtime.NewFake()
	if err := sp.Start(context.Background(), v.SessionName, runtime.Config{}); err != nil {
		t.Fatal(err)
	}
	if err := sp.SetMeta(v.SessionName, "GC_INSTANCE_TOKEN", "other-fixture"); err != nil {
		t.Fatal(err)
	}
	if err := acknowledgeVerifiedHookProvider(v, sp); !errors.Is(err, session.ErrStaleSession) {
		t.Fatalf("foreign provider occupant: %v", err)
	}
	if ack, _ := sp.GetMeta(v.SessionName, "GC_DRAIN_ACK"); ack != "" {
		t.Fatal("foreign acknowledgement")
	}
	mustSetMeta(t, sp, v.SessionName, "GC_RUNTIME_EPOCH", "1")
	if err := sp.SetMeta(v.SessionName, "GC_INSTANCE_TOKEN", "own-fixture"); err != nil {
		t.Fatal(err)
	}
	if err := acknowledgeVerifiedHookProvider(v, sp); err != nil {
		t.Fatal(err)
	}
	if ack, _ := sp.GetMeta(v.SessionName, "GC_DRAIN_ACK"); ack != verifiedDrainAckValue(v) {
		t.Fatal("own acknowledgement missing")
	}
}

func TestVerifiedHookCurrentRefusesStaleIncarnation(t *testing.T) {
	clearGCEnv(t)
	v, store := newVerifiedHookFixture(t)
	if err := v.front.CompareAndSetCurrentClaim(v.VerifiedSession, "", "work-own"); err != nil {
		t.Fatal(err)
	}
	old := hookCurrentSessionFrontDoor
	t.Cleanup(func() { hookCurrentSessionFrontDoor = old })
	hookCurrentSessionFrontDoor = func() (*session.Store, error) { return v.front, nil }
	t.Setenv("GC_SESSION_ID", v.ID)
	t.Setenv("GC_INSTANCE_TOKEN", "own-fixture")
	t.Setenv("GC_RUNTIME_EPOCH", "1")
	if err := store.mem.SetMetadata(v.ID, "generation", "2"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if cmdHookCurrent(true, &out, io.Discard) != 1 || out.Len() != 0 {
		t.Fatal("stale hook current printed a claim")
	}
}

func TestVerifiedHookControllerVetoRequiresAssigneeAndSessionID(t *testing.T) {
	info := session.Info{ID: "s-own", SessionName: "own-name", SessionNameMetadata: "own-name"}
	for _, row := range []struct {
		assignee, id string
		want         bool
	}{
		{"s-own", "s-own", true}, {"own-name", "s-own", true}, {"s-other", "s-own", false}, {"s-own", "s-other", false}, {"s-own", "", false}, {"", "s-own", false},
	} {
		work := beads.Bead{Status: "in_progress", Assignee: row.assignee, Metadata: map[string]string{beadmeta.SessionIDMetadataKey: row.id}}
		if got := claimedWorkStillHeldBy(work, info); got != row.want {
			t.Fatalf("assignee %q session %q = %v", row.assignee, row.id, got)
		}
	}
}

func installOwnSessionFrontDoor(t *testing.T, cityPath string, store beads.Store) {
	t.Helper()
	old := openHookSessionFrontDoor
	t.Cleanup(func() { openHookSessionFrontDoor = old })
	openHookSessionFrontDoor = func(path string, _ *config.City) (*session.Store, error) {
		if path != cityPath {
			t.Fatalf("session store opened for foreign city %q", path)
		}
		return session.NewStore(beads.SessionStore{Store: store}), nil
	}
}

func TestVerifiedHookRefusedDrainAckHasNoSideEffects(t *testing.T) {
	var out bytes.Buffer
	code := writeHookClaimStaleSessionDrain(hookCommandOptions{JSON: true, DrainAck: true}, &out, io.Discard)
	if code != 1 || !strings.Contains(out.String(), `"stale_session"`) || strings.Contains(out.String(), `"drain_acknowledged":true`) {
		t.Fatalf("refused result: %d %s", code, &out)
	}
}

func TestManagedHookRejectsForeignEnvironmentAndTrailingAgentBeforeQuery(t *testing.T) {
	for _, key := range []string{"GC_SESSION_NAME", "GC_TMUX_SESSION", "GC_ALIAS", "GC_TEMPLATE", "GC_AGENT", "positional"} {
		t.Run(key, func(t *testing.T) {
			city, claimLock, _, _ := newClaimIdentityRaceFixture(t, "present")
			configPath := filepath.Join(city, "city.toml")
			data, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			data = append(data, []byte("\n[[agent]]\nname = \"foreign-worker\"\n")...)
			if err := os.WriteFile(configPath, data, 0o644); err != nil {
				t.Fatal(err)
			}
			queryMarker := installFenceWorkQueryProbe(t)
			own := claimIdentityOwnSessionStore(t, claimIdentityRaceToken, "1")
			installOwnSessionFrontDoor(t, city, own)
			var args []string
			if key == "positional" {
				args = []string{"foreign-worker"}
			} else {
				t.Setenv(key, "foreign-worker")
			}
			var out bytes.Buffer
			code := cmdHookWithOptions(args, hookCommandOptions{Claim: true, JSON: true, DrainAck: true}, &out, io.Discard)
			result := decodeClaimIdentityRaceResult(t, &out)
			if code != 1 || result.Reason != hookClaimReasonStaleSession || result.DrainAcknowledged || own.writes != 0 {
				t.Fatalf("foreign identity = %d %+v, writes %d", code, result, own.writes)
			}
			if _, err := os.Stat(claimLock); !os.IsNotExist(err) {
				t.Fatal("foreign identity reached claim")
			}
			if _, err := os.Stat(queryMarker); !os.IsNotExist(err) {
				t.Fatal("foreign identity reached work query")
			}
			if data, _ := os.ReadFile(os.Getenv("CLAIM_CALL_LOG")); len(data) > 0 {
				t.Fatal("foreign identity reached mutation")
			}
		})
	}
}

func TestVerifiedHookProviderConstructionReadsOnlyOwnRow(t *testing.T) {
	v, store := newVerifiedHookFixture(t)
	v.cfg = &config.City{}
	oldOpen, oldBuild := openSessionProviderStore, buildSessionProviderByName
	t.Cleanup(func() { openSessionProviderStore = oldOpen; buildSessionProviderByName = oldBuild })
	openSessionProviderStore = func(string) (beads.Store, error) { t.Fatal("provider loaded session census"); return nil, nil }
	buildSessionProviderByName = func(*config.City, string, config.SessionConfig, string, string) (runtime.Provider, error) {
		return runtime.NewFake(), nil
	}
	info, err := v.front.ReadVerified(v.VerifiedSession)
	if err != nil {
		t.Fatal(err)
	}
	if info.InstanceToken != "" {
		t.Fatal("provider snapshot contains raw capability")
	}
	if _, err := newVerifiedHookProvider(v, info); err != nil {
		t.Fatal(err)
	}
	if store.reads != 2 || store.writes != 0 {
		t.Fatalf("provider accesses = %d reads, %d writes", store.reads, store.writes)
	}
}

type verifiedReleaseRaceStore struct {
	*beads.MemStore
	onGet func()
}

func (s *verifiedReleaseRaceStore) Get(id string) (beads.Bead, error) {
	if s.onGet != nil {
		f := s.onGet
		s.onGet = nil
		f()
	}
	return s.MemStore.Get(id)
}

func TestVerifiedHookReleaseRechecksIncarnationImmediatelyBeforeCAS(t *testing.T) {
	v, own := newVerifiedHookFixture(t)
	work := &verifiedReleaseRaceStore{MemStore: beads.NewMemStoreFrom(1, []beads.Bead{{ID: "work", Type: "task", Status: "in_progress", Assignee: v.ID, Revision: 1, Metadata: map[string]string{beadmeta.SessionIDMetadataKey: v.ID, verifiedHookIncarnationKey: v.workIncarnation(), verifiedHookAttemptKey: "fixture-attempt"}}}, nil)}
	work.onGet = func() {
		if err := own.mem.SetMetadata(v.ID, "generation", "2"); err != nil {
			t.Fatal(err)
		}
	}
	if err := releaseVerifiedHookClaims(v, work); !errors.Is(err, session.ErrStaleSession) {
		t.Fatalf("release race: %v", err)
	}
	b, _ := work.Get("work")
	if b.Status != "in_progress" || b.Revision != 1 || own.writes != 0 {
		t.Fatal("release race mutated work or successor")
	}
}

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/session"
)

func TestVerifiedHookCheckoutUsesOneExactIncarnationRead(t *testing.T) {
	v, own := newVerifiedHookFixture(t)
	if err := own.mem.SetMetadata(v.ID, "work_dir", "/worktrees/own"); err != nil {
		t.Fatal(err)
	}
	before := own.reads
	if got := v.workDir(); got != "/worktrees/own" || own.reads != before+1 {
		t.Fatalf("checkout %q, reads %d", got, own.reads-before)
	}
	if err := own.mem.SetMetadata(v.ID, "pool_managed", "true"); err != nil {
		t.Fatal(err)
	}
	if got := v.workDir(); got != "" {
		t.Fatal("shared pool checkout admitted")
	}
	if err := own.mem.SetMetadata(v.ID, "instance_token", "successor"); err != nil {
		t.Fatal(err)
	}
	if got := v.workDir(); got != "" {
		t.Fatal("successor checkout admitted")
	}
}

type verifiedBrokenOutput struct{}

func (verifiedBrokenOutput) Write([]byte) (int, error) { return 0, syscall.EPIPE }

func TestVerifiedHookBrokenOutputConditionallyClearsPublishedClaimAndUnwinds(t *testing.T) {
	v, own := newVerifiedHookFixture(t)
	released := 0
	ops := hookClaimOps{
		PublishRunMap: noopPublishRunMap,
		EmitClaimReleased: func(record hookClaimReleaseRecord) {
			if record.BeadID != "work-own" || record.Assignee != v.ID {
				t.Fatal("wrong pending release")
			}
			released++
		},
	}
	ops.applyDefaults()
	// This fixture needs no work identity patch; the test owns the session-side
	// stamp/clear and output rollback, independently of Git probes.
	bead := beads.Bead{ID: "work-own", Type: "task", Status: "in_progress", Assignee: v.ID, Metadata: map[string]string{beadmeta.ClaimedAtMetadataKey: "2026-10-13T00:00:00Z"}}
	mem := beads.NewMemStoreFrom(1, []beads.Bead{bead}, nil)
	installVerifiedWorkStore(t, v, mem)
	opts := hookClaimOptions{Verified: v, Assignee: v.ID, JSON: true}
	code := writeHookClaimWorkResultForBead(hookClaimJSONResult{Action: "work", BeadID: bead.ID}, bead, opts, ops, "", true, verifiedBrokenOutput{}, io.Discard)
	row, _ := own.mem.Get(v.ID)
	if code != 1 || released != 1 || own.writes != 2 || row.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey] != "" {
		t.Fatalf("code=%d releases=%d writes=%d", code, released, own.writes)
	}
	live, _ := mem.Get(bead.ID)
	if live.Status != "open" || live.Assignee != "" {
		t.Fatal("broken output did not unwind fenced work")
	}
}

type releaseWorkCASRace struct {
	*beads.MemStore
	beforeWrite func()
}

func (s *releaseWorkCASRace) UpdateIfMatch(id string, rev int64, opts beads.UpdateOpts) error {
	if s.beforeWrite != nil {
		f := s.beforeWrite
		s.beforeWrite = nil
		f()
	}
	return s.MemStore.UpdateIfMatch(id, rev, opts)
}

func TestVerifiedHookReleaseCannotOverwriteConcurrentWorkOwnership(t *testing.T) {
	v, _ := newVerifiedHookFixture(t)
	mem := beads.NewMemStoreFrom(1, []beads.Bead{{ID: "work", Type: "task", Status: "in_progress", Assignee: v.ID, Revision: 1, Metadata: map[string]string{beadmeta.SessionIDMetadataKey: v.ID, verifiedHookIncarnationKey: v.workIncarnation(), verifiedHookAttemptKey: "fixture-attempt"}}}, nil)
	store := &releaseWorkCASRace{MemStore: mem, beforeWrite: func() {
		if err := mem.SetMetadata("work", beadmeta.SessionIDMetadataKey, "s-other"); err != nil {
			t.Fatal(err)
		}
	}}
	if err := releaseVerifiedHookClaims(v, store); err != nil {
		t.Fatal(err)
	}
	b, _ := mem.Get("work")
	if b.Status != "in_progress" || b.Metadata[beadmeta.SessionIDMetadataKey] != "s-other" {
		t.Fatal("release overwrote concurrent work owner")
	}
}

type releaseNoCASStore struct{ beads.Store }

func TestVerifiedHookReleaseWithoutAtomicBackendRefuses(t *testing.T) {
	v, _ := newVerifiedHookFixture(t)
	mem := beads.NewMemStoreFrom(1, []beads.Bead{{ID: "work", Type: "task", Status: "in_progress", Assignee: v.ID, Revision: 1, Metadata: map[string]string{beadmeta.SessionIDMetadataKey: v.ID, verifiedHookIncarnationKey: v.workIncarnation(), verifiedHookAttemptKey: "fixture-attempt"}}}, nil)
	if err := releaseVerifiedHookClaims(v, releaseNoCASStore{mem}); !errors.Is(err, beads.ErrConditionalWriteUnsupported) {
		t.Fatalf("unfenced release: %v", err)
	}
	b, _ := mem.Get("work")
	if b.Revision != 1 {
		t.Fatal("unconditional release fallback")
	}
}

func TestManagedHookDrainingProjectionStopsClaimsWithVerifiedIdentity(t *testing.T) {
	city, claimLock, _, _ := newClaimIdentityRaceFixture(t, "present")
	writeClaimIdentityRaceProjection(t, city, claimIdentityRaceSessionID, claimIdentityRaceToken, 1, session.StateDraining)
	var out bytes.Buffer
	code := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true}, &out, io.Discard)
	result := decodeClaimIdentityRaceResult(t, &out)
	if code != 1 || result.Reason != hookClaimReasonDrainPending {
		t.Fatalf("draining result = %d %+v", code, result)
	}
	if _, err := os.Stat(claimLock); !os.IsNotExist(err) {
		t.Fatal("draining incarnation claimed work")
	}
}

func TestManagedHookStaleIncarnationWithDrainAckNeverOpensProviderOrReleases(t *testing.T) {
	city, claimLock, _, _ := newClaimIdentityRaceFixture(t, "present")
	own := claimIdentityOwnSessionStore(t, claimIdentityRaceToken, "1")
	installOwnSessionFrontDoor(t, city, own)
	writeClaimIdentityRaceProjection(t, city, claimIdentityRaceSessionID, "successor", 2, session.StateActive)
	var out bytes.Buffer
	code := cmdHookWithOptions(nil, hookCommandOptions{Claim: true, JSON: true, DrainAck: true}, &out, io.Discard)
	result := decodeClaimIdentityRaceResult(t, &out)
	if code != 1 || result.Reason != hookClaimReasonStaleSession || result.DrainAcknowledged || own.reads != 0 || own.writes != 0 {
		t.Fatalf("stale result=%d %+v accesses=%d/%d", code, result, own.reads, own.writes)
	}
	if _, err := os.Stat(claimLock); !os.IsNotExist(err) {
		t.Fatal("stale drain touched work")
	}
}

func TestVerifiedHookDrainAckReleasesOnlyConfiguredStoresAndClearsOwnStamp(t *testing.T) {
	clearGCEnv(t)
	t.Setenv("GC_BEADS", "file")
	v, own := newVerifiedHookFixture(t)
	v.cityPath = t.TempDir()
	if err := ensureScopedFileStoreLayout(v.cityPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v.cityPath, "city.toml"), []byte("[workspace]\nname = \"test-city\"\n[beads]\nprovider = \"file\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stores := make([]beads.Store, 2)
	work := make([]beads.Bead, 2)
	for i, rig := range []string{"own-rig", "other-rig"} {
		dir := filepath.Join(v.cityPath, rig)
		if err := ensurePersistedScopeLocalFileStore(dir); err != nil {
			t.Fatal(err)
		}
		store, err := openStoreAtForCity(dir, v.cityPath)
		if err != nil {
			t.Fatal(err)
		}
		stores[i] = store
		work[i], err = store.Create(beads.Bead{Title: "owned work", Type: "task", Status: "in_progress", Assignee: v.ID, Metadata: map[string]string{beadmeta.SessionIDMetadataKey: v.ID, verifiedHookIncarnationKey: v.workIncarnation(), verifiedHookAttemptKey: "fixture-attempt"}})
		if err != nil {
			t.Fatal(err)
		}
		status := "in_progress"
		if err := store.Update(work[i].ID, beads.UpdateOpts{Status: &status}); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			v.AllowedWorkStores = []hookStore{{dir: dir}, {dir: dir}}
		}
	}
	if err := v.front.CompareAndSetCurrentClaim(v.VerifiedSession, "", work[0].ID); err != nil {
		t.Fatal(err)
	}
	v.CurrentClaim = work[0].ID
	sp := runtime.NewFake()
	if err := sp.Start(context.Background(), v.SessionName, runtime.Config{}); err != nil {
		t.Fatal(err)
	}
	mustSetMeta(t, sp, v.SessionName, "GC_RUNTIME_EPOCH", "1")
	if err := sp.SetMeta(v.SessionName, "GC_INSTANCE_TOKEN", "own-fixture"); err != nil {
		t.Fatal(err)
	}
	old := newVerifiedHookProvider
	t.Cleanup(func() { newVerifiedHookProvider = old })
	newVerifiedHookProvider = func(got *VerifiedSession, info session.Info) (runtime.Provider, error) {
		if got != v || info.ID != v.ID || info.InstanceToken != "" {
			t.Fatal("provider received foreign identity or raw token")
		}
		return sp, nil
	}
	if err := v.drainAck(nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	for i, store := range stores {
		got, err := store.Get(work[i].ID)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && (got.Status != "open" || got.Assignee != "") {
			t.Fatalf("own work not released: %+v", got)
		}
		if i == 1 && (got.Status != "in_progress" || got.Assignee != v.ID) {
			t.Fatalf("work outside configured scope changed: %+v", got)
		}
	}
	row, err := own.mem.Get(v.ID)
	if err != nil || row.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey] != "" || own.writes != 2 {
		t.Fatalf("own stamp clear: writes=%d err=%v", own.writes, err)
	}
	if ack, _ := sp.GetMeta(v.SessionName, "GC_DRAIN_ACK"); ack != verifiedDrainAckValue(v) {
		t.Fatal("verified provider acknowledgement missing")
	}
}

func TestVerifiedHookOpsRefuseStaleIncarnationBeforeMutatingWork(t *testing.T) {
	v, own := newVerifiedHookFixture(t)
	ops := hookClaimOps{
		Claim: func(context.Context, string, []string, string, string) (beads.Bead, bool, error) {
			t.Fatal("stale claim reached work store")
			return beads.Bead{}, false, nil
		},
		RestampAdopted: func(context.Context, string, []string, string, string, string) (bool, error) {
			t.Fatal("stale legacy restamp reached work store")
			return false, nil
		},
		StampWorkMeta: func(context.Context, string, []string, string, string, map[string]string) error {
			t.Fatal("stale metadata stamp reached work store")
			return nil
		},
	}
	bindVerifiedHookOps(&ops, v)
	if err := own.mem.SetMetadata(v.ID, "generation", "2"); err != nil {
		t.Fatal(err)
	}
	_, _, claimErr := ops.Claim(context.Background(), "", nil, "work", v.ID)
	_, restampErr := ops.RestampAdopted(context.Background(), "", nil, "work", v.SessionName, v.ID)
	stampErr := ops.StampWorkMeta(context.Background(), "", nil, "work", v.ID, nil)
	for _, err := range []error{claimErr, restampErr, stampErr} {
		if !errors.Is(err, session.ErrStaleSession) {
			t.Fatalf("stale mutation = %v", err)
		}
	}
}

func TestVerifiedHookLateFailureUnwindsPendingClaimWithoutClearingSuccessor(t *testing.T) {
	for _, lateIdentityChange := range []bool{false, true} {
		t.Run(map[bool]string{false: "continuation_read_error", true: "incarnation_changed_before_delivery"}[lateIdentityChange], func(t *testing.T) {
			v, own := newVerifiedHookFixture(t)
			bead := beads.Bead{ID: "work", Type: "task", Status: "in_progress", Assignee: v.ID, Metadata: map[string]string{
				beadmeta.SessionIDMetadataKey: v.ID, beadmeta.ClaimedAtMetadataKey: "2026-10-13T00:00:00Z",
			}}
			if !lateIdentityChange {
				bead.Metadata[beadmeta.RootBeadIDMetadataKey] = "root"
				bead.Metadata[beadmeta.ContinuationGroupMetadataKey] = "group"
			}
			released := 0
			ops := hookClaimOps{
				ResolveSessionWorkDir: func(string) string { return "" },
				PublishRunMap: func(string, string, ...string) error {
					if lateIdentityChange {
						return own.mem.SetMetadata(v.ID, "generation", "2")
					}
					return nil
				},
				ListContinuation: func(context.Context, string, []string, string, string) ([]beads.Bead, error) {
					return nil, errors.New("fixture continuation read unavailable")
				},
				EmitClaimReleased: func(record hookClaimReleaseRecord) {
					if record.BeadID != bead.ID || record.Assignee != v.ID {
						t.Fatal("foreign pending release")
					}
					released++
				},
			}
			ops.applyDefaults()
			mem := beads.NewMemStoreFrom(1, []beads.Bead{bead}, nil)
			installVerifiedWorkStore(t, v, mem)
			var out bytes.Buffer
			opts := hookClaimOptions{Verified: v, Assignee: v.ID, Env: []string{"GC_SESSION_ID=" + v.ID}, JSON: true}
			code := writeHookClaimWorkResultForBead(hookClaimJSONResult{Action: "work", BeadID: bead.ID}, bead, opts, ops, "", true, &out, io.Discard)
			result := decodeClaimIdentityRaceResult(t, &out)
			row, _ := own.mem.Get(v.ID)
			wantReason, wantClaim, wantWrites := hookClaimReasonClaimsErrored, "", 2
			if lateIdentityChange {
				wantReason, wantClaim, wantWrites = hookClaimReasonStaleSession, bead.ID, 1
			}
			if code != 1 || result.Reason != wantReason || released != 1 || own.writes != wantWrites || row.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey] != wantClaim {
				t.Fatalf("late failure: code=%d result=%+v released=%d writes=%d row=%+v", code, result, released, own.writes, row)
			}
			live, _ := mem.Get(bead.ID)
			if live.Status != "open" || live.Assignee != "" {
				t.Fatal("late failure did not unwind fenced work")
			}
		})
	}
}

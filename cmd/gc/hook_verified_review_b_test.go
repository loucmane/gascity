package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/session"
)

func installVerifiedWorkStore(t *testing.T, v *VerifiedSession, store beads.Store) {
	t.Helper()
	old := openVerifiedHookWorkStore
	t.Cleanup(func() { openVerifiedHookWorkStore = old })
	openVerifiedHookWorkStore = func(_ context.Context, _ []string, got *VerifiedSession, _ *hookClaimClassRoute, _, _ string) (beads.Store, error) {
		if got.ID != v.ID {
			t.Fatal("foreign verified work store")
		}
		return store, nil
	}
}

// Regression: an old drain must not release a claim already adopted by the
// next generation of the same durable session. Session and work revisions are
// deliberately independent, matching relocated stores.
func TestReviewBDrainCannotReleaseSuccessorAdoptedWork(t *testing.T) {
	v, own := newVerifiedHookFixture(t)
	if err := v.front.CompareAndSetCurrentClaim(v.VerifiedSession, "", "work"); err != nil {
		t.Fatal(err)
	}
	v.CurrentClaim = "work"
	mem := beads.NewMemStoreFrom(1, []beads.Bead{{ID: "work", Type: "task", Status: "in_progress", Assignee: v.assignee(), Revision: 1, Metadata: map[string]string{verifiedHookIncarnationKey: v.workIncarnation(), verifiedHookAttemptKey: "original-attempt", beadmeta.SessionIDMetadataKey: v.ID, beadmeta.SessionNameMetadataKey: v.SessionName, beadmeta.ClaimedAtMetadataKey: "2026-10-13T00:00:00Z"}}}, nil)
	installVerifiedWorkStore(t, v, mem)
	store := &releaseWorkCASRace{MemStore: mem, beforeWrite: func() {
		// The controller rolls the session after the old drain's last own-row read.
		if err := own.mem.Update(v.ID, beads.UpdateOpts{Metadata: map[string]string{"generation": "2", "instance_token": "successor", "state": "active", beadmeta.CurrentClaimBeadIDMetadataKey: ""}}); err != nil {
			t.Fatal(err)
		}
		next, err := v.front.VerifySession(v.ID, "successor", 2)
		if err != nil {
			t.Fatal(err)
		}
		// Use the production adoption/delivery path, not a fabricated stamp.
		successor := &VerifiedSession{VerifiedSession: next, front: v.front}
		work, _ := mem.Get("work")
		ops := hookClaimOps{
			PublishRunMap:         noopPublishRunMap,
			ResolveSessionWorkDir: func(string) string { return "" },
			StampWorkMeta: func(_ context.Context, _ string, _ []string, id, _ string, patch map[string]string) error {
				return mem.Update(id, beads.UpdateOpts{Metadata: patch})
			},
			ReadWorkMeta: func(_ context.Context, _ string, _ []string, id, _ string) (beads.Bead, error) { return mem.Get(id) },
		}
		bindVerifiedHookOps(&ops, successor)
		opts := hookClaimOptions{Verified: successor, Assignee: successor.assignee(), Env: []string{"GC_SESSION_ID=" + v.ID, "GC_SESSION_NAME=" + v.SessionName}, JSON: true}
		var output bytes.Buffer
		if code := writeHookClaimWorkResultForBead(hookClaimJSONResult{Action: "work", BeadID: work.ID}, work, opts, ops, "", false, &output, io.Discard); code != 0 {
			t.Fatalf("successor adoption failed: code=%d output=%s", code, &output)
		}
		adopted, _ := mem.Get("work")
		if adopted.Revision <= 1 || adopted.Metadata[verifiedHookIncarnationKey] != successor.workIncarnation() {
			t.Fatalf("successor adoption did not advance the work fence: %+v", adopted)
		}
	}}
	err := releaseVerifiedHookClaims(v, store)
	work, _ := mem.Get("work")
	successor, _ := own.mem.Get(v.ID)
	if work.Status != "in_progress" || work.Assignee != v.assignee() {
		t.Fatalf("old drain changed successor's work: status=%q assignee=%q work_revision=%d successor_generation=%s successor_claim=%q returned=%v", work.Status, work.Assignee, work.Revision, successor.Metadata["generation"], successor.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey], err)
	}
}

type reviewBRecycleProvider struct {
	runtime.Provider
	reads   int
	recycle func()
}

func (p *reviewBRecycleProvider) GetMeta(name, key string) (string, error) {
	value, err := p.Provider.GetMeta(name, key)
	if key == "GC_INSTANCE_TOKEN" {
		p.reads++
		if p.reads == 5 {
			p.recycle()
		}
	}
	return value, err
}

func TestReviewBDrainAckCannotWriteRecycledRuntime(t *testing.T) {
	v, own := newVerifiedHookFixture(t)
	fake := runtime.NewFake()
	if err := fake.Start(context.Background(), v.SessionName, runtime.Config{}); err != nil {
		t.Fatal(err)
	}
	if err := fake.SetMeta(v.SessionName, "GC_INSTANCE_TOKEN", "own-fixture"); err != nil {
		t.Fatal(err)
	}
	mustSetMeta(t, fake, v.SessionName, "GC_RUNTIME_EPOCH", "1")
	p := &reviewBRecycleProvider{Provider: fake}
	p.recycle = func() {
		if err := own.mem.Update(v.ID, beads.UpdateOpts{Metadata: map[string]string{"generation": "2", "instance_token": "successor"}}); err != nil {
			t.Fatal(err)
		}
		if err := fake.Stop(v.SessionName); err != nil {
			t.Fatal(err)
		}
		if err := fake.Start(context.Background(), v.SessionName, runtime.Config{}); err != nil {
			t.Fatal(err)
		}
		if err := fake.SetMeta(v.SessionName, "GC_INSTANCE_TOKEN", "successor"); err != nil {
			t.Fatal(err)
		}
		mustSetMeta(t, fake, v.SessionName, "GC_RUNTIME_EPOCH", "2")
	}
	err := acknowledgeVerifiedHookProvider(v, p)
	ack, _ := fake.GetMeta(v.SessionName, "GC_DRAIN_ACK")
	token, _ := fake.GetMeta(v.SessionName, "GC_INSTANCE_TOKEN")
	if !errors.Is(err, session.ErrStaleSession) {
		t.Fatalf("stale acknowledgement reported success: %v", err)
	}
	if accepted, readErr := (&providerDrainOps{sp: fake}).isDrainAcked(v.SessionName); readErr != nil || accepted {
		t.Fatalf("old invocation acknowledged recycled runtime: token=%q ack=%q returned=%v accepted=%v read=%v", token, ack, err, accepted, readErr)
	}
	next, readErr := v.front.Get(v.ID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if controllerDrainAckMatches(next, fake, v.SessionName) {
		t.Fatal("controller accepted the recycled runtime acknowledgement")
	}
}

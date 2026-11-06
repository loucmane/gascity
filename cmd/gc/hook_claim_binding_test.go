package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/session"
)

type afterWorkFenceStore struct {
	*beads.MemStore
	after func()
}

func (s *afterWorkFenceStore) UpdateIfMatch(id string, rev int64, opts beads.UpdateOpts) error {
	if err := s.MemStore.UpdateIfMatch(id, rev, opts); err != nil {
		return err
	}
	if s.after != nil {
		f := s.after
		s.after = nil
		f()
	}
	return nil
}

func TestHookClaimBindingAfterWorkFenceUnwinds(t *testing.T) {
	v, own := newVerifiedHookFixture(t)
	work, bead := verifiedWorkFixture(t, v)
	if err := own.mem.SetMetadataBatch(v.ID, map[string]string{"pool_managed": "true", "work_dir": "/work/ga-gwav/a"}); err != nil {
		t.Fatal(err)
	}
	if err := work.SetMetadataBatch(bead.ID, hookClaimManagedWorkspaceBead(bead.ID, "/work/ga-gwav/a").Metadata); err != nil {
		t.Fatal(err)
	}
	info, err := v.front.ReadVerified(v.VerifiedSession)
	if err != nil {
		t.Fatal(err)
	}
	bp := &agentBuildParams{beadStore: own.mem, stderr: io.Discard}
	bound := false
	installVerifiedWorkStore(t, v, &afterWorkFenceStore{MemStore: work, after: func() {
		patch := computePoolTriggerBindingPatch(info, SessionRequest{WorkBeadID: "other"}, "/work/ga-gwav/b")
		if err := bp.poolClaims().write(info.ID, "other", patch); err != nil {
			t.Fatal(err)
		}
		bound = true
	}})
	releases := 0
	ops := hookClaimOps{PublishRunMap: noopPublishRunMap, EmitClaimReleased: func(r hookClaimReleaseRecord) {
		releases++
		if r.Reason != "session_binding_conflict" {
			t.Errorf("release reason = %q", r.Reason)
		}
	}}
	ops.applyDefaults()
	var stdout, stderr bytes.Buffer
	code := writeHookClaimWorkResultForBead(hookClaimJSONResult{Action: "work", BeadID: bead.ID}, bead,
		hookClaimOptions{Verified: v, Assignee: v.assignee(), JSON: true}, ops, "", true, &stdout, &stderr)
	var result hookClaimJSONResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !bound || code != 1 || result.Action != "drain" || result.Reason != "session_binding_conflict" || releases != 1 {
		t.Errorf("bound=%v code=%d result=%+v releases=%d stderr=%s", bound, code, result, releases, &stderr)
	}
	row := mustGetTestBead(t, work, bead.ID)
	if row.Status != "open" || row.Assignee != "" || row.Metadata[verifiedHookAttemptKey] != "" {
		t.Error("binding conflict did not unwind the fenced work")
	}
	if own.writes != 0 {
		t.Error("binding conflict published a session claim")
	}
}

func TestHookClaimBoundTriggerSelection(t *testing.T) {
	for _, trigger := range []string{"", "work-b", "absent"} {
		t.Run("trigger-"+trigger, func(t *testing.T) {
			v, own := newVerifiedHookFixture(t)
			if err := own.mem.SetMetadata(v.ID, beadmeta.TriggerBeadIDMetadataKey, trigger); err != nil {
				t.Fatal(err)
			}
			candidates := []beads.Bead{
				{ID: "work-a", Status: "open", Metadata: map[string]string{beadmeta.RoutedToMetadataKey: v.Template}},
				{ID: "work-b", Status: "open", Metadata: map[string]string{beadmeta.RoutedToMetadataKey: v.Template}},
			}
			work := beads.NewMemStoreFrom(1, candidates, nil)
			installVerifiedWorkStore(t, v, work)
			query, err := json.Marshal(candidates)
			if err != nil {
				t.Fatal(err)
			}
			var claimed []string
			ops := hookClaimOps{
				Runner: func(string, string) (string, error) { return string(query), nil },
				Claim: func(_ context.Context, _ string, _ []string, id, assignee string) (beads.Bead, bool, error) {
					claimed = append(claimed, id)
					status := "in_progress"
					if err := work.Update(id, beads.UpdateOpts{Status: &status, Assignee: &assignee}); err != nil {
						return beads.Bead{}, false, err
					}
					b, err := work.Get(id)
					return b, err == nil, err
				},
				PublishRunMap: noopPublishRunMap,
			}
			bindVerifiedHookOps(&ops, v)
			var out, stderr bytes.Buffer
			code := doHookClaim("query", "", hookClaimOptions{Verified: v, Assignee: v.assignee(), RouteTargets: []string{v.Template}, JSON: true}, ops, &out, &stderr)
			want := "work-a"
			if code != 0 || len(claimed) != 1 || claimed[0] != want {
				t.Fatalf("claimed=%v want=%s code=%d stderr=%s", claimed, want, code, &stderr)
			}
		})
	}
}

func TestHookClaimBindingWorkDirFence(t *testing.T) {
	v, own := newVerifiedHookFixture(t)
	work, bead := verifiedWorkFixture(t, v)
	if err := work.SetMetadataBatch(bead.ID, hookClaimManagedWorkspaceBead(bead.ID, "/work/ga-gwav/a").Metadata); err != nil {
		t.Fatal(err)
	}
	if err := own.mem.SetMetadataBatch(v.ID, map[string]string{"pool_managed": "true", "work_dir": "/work/ga-gwav/b"}); err != nil {
		t.Fatal(err)
	}
	ops := hookClaimOps{PublishRunMap: noopPublishRunMap, EmitClaimReleased: func(hookClaimReleaseRecord) {}}
	ops.applyDefaults()
	var out bytes.Buffer
	code := writeHookClaimWorkResultForBead(hookClaimJSONResult{Action: "work", BeadID: bead.ID}, bead,
		hookClaimOptions{Verified: v, Assignee: v.assignee(), JSON: true}, ops, "", true, &out, io.Discard)
	if code != 1 || own.writes != 0 || mustGetTestBead(t, work, bead.ID).Status != "open" {
		t.Fatalf("incompatible work dir accepted: code=%d writes=%d result=%s", code, own.writes, &out)
	}
}

// The guard must read a stale/released A before A is reclaimed and stamped.
// Reading only after reacquisition would exercise the ordinary held-claim path.
type reacquireBeforeBindStore struct {
	*beads.MemStore
	beforeBind func()
}

func (s *reacquireBeforeBindStore) UpdateIfMatch(id string, rev int64, opts beads.UpdateOpts) error {
	if s.beforeBind != nil {
		f := s.beforeBind
		s.beforeBind = nil
		f()
	}
	return s.MemStore.UpdateIfMatch(id, rev, opts)
}

func TestPoolClaimGuardSameIDReacquireFence(t *testing.T) {
	bp, owner, _ := claimGuardFixture(t, 5*time.Minute, 2)
	mem := bp.beadStore.(*beads.MemStore)
	if err := mem.Close("work-a"); err != nil {
		t.Fatal(err)
	}
	if err := mem.SetMetadataBatch(owner.ID, map[string]string{"instance_token": "fixture", "generation": "1"}); err != nil {
		t.Fatal(err)
	}
	front := session.NewStore(beads.SessionStore{Store: mem})
	v, err := front.VerifySession(owner.ID, "fixture", 1)
	if err != nil {
		t.Fatal(err)
	}
	bp.beadStore = &reacquireBeforeBindStore{MemStore: mem, beforeBind: func() {
		status := "in_progress"
		if err := mem.Update("work-a", beads.UpdateOpts{Status: &status}); err != nil {
			t.Fatal(err)
		}
		if err := front.CompareAndSetCurrentClaim(v, "work-a", "work-a"); err != nil {
			t.Fatal(err)
		}
	}}
	info := bp.sessionBeads.OpenInfos()[0]
	if _, err := bindPoolSessionTriggerBead(bp, &bp.city.Agents[0], "worker-1", info, SessionRequest{WorkBeadID: "work-b"}); err == nil {
		t.Error("bind accepted same-ID reacquisition after ownership read")
	}
	assertClaimGuardOwner(t, mem, owner)
}

func TestHookClaimStampThenBindingRefused(t *testing.T) {
	bp, owner, _ := claimGuardFixture(t, 5*time.Minute, 2)
	if err := bp.beadStore.SetMetadataBatch(owner.ID, map[string]string{
		"instance_token": "fixture", "generation": "1", beadmeta.CurrentClaimBeadIDMetadataKey: "",
	}); err != nil {
		t.Fatal(err)
	}
	front := session.NewStore(beads.SessionStore{Store: bp.beadStore})
	v, err := front.VerifySession(owner.ID, "fixture", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := front.CompareAndSetCurrentClaim(v, "", "work-a"); err != nil {
		t.Fatal(err)
	}
	info := bp.sessionBeads.OpenInfos()[0]
	if _, err := bindPoolSessionTriggerBead(bp, &bp.city.Agents[0], "worker-1", info, SessionRequest{WorkBeadID: "work-b"}); err == nil {
		t.Error("bind accepted an already stamped held claim")
	}
	assertClaimGuardOwner(t, bp.beadStore, owner)
}

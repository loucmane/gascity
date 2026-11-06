package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/session"
)

// Scheduling hints must not hide executable continuations, unmanaged recipe
// steps, or compatible work later in the query (ga-gwav round 4).
func TestHookClaimRound4Progress(t *testing.T) {
	for _, scenario := range []string{
		"closed-trigger-continuation", "blocked-priority-sibling", "named-cross-store-trigger",
		"bound-drain-work-dir-only", "unbound-drain-work-dir-only", "branch-only",
		"unbound-assigned-incompatible-head", "unbound-adopted-incompatible-head", "unbound-routed-incompatible-head",
		"unbound-assigned-missing-branch-head", "unbound-adopted-missing-branch-head", "unbound-routed-missing-branch-head",
	} {
		t.Run(scenario, func(t *testing.T) {
			v, own := newVerifiedHookFixture(t)
			meta := map[string]string{"pool_managed": "true", "work_dir": "/work/ga-gwav/a"}
			a := beads.Bead{
				ID: "ready-a", Status: "open", Assignee: v.assignee(), Revision: 1,
				Metadata: map[string]string{beadmeta.RoutedToMetadataKey: v.Template},
			}
			b := beads.Bead{ID: "trigger-b", Status: "closed", Revision: 1}
			meta[beadmeta.TriggerBeadIDMetadataKey] = b.ID
			switch scenario {
			case "closed-trigger-continuation":
				meta[beadmeta.CurrentClaimBeadIDMetadataKey] = b.ID
				a.Metadata[beadmeta.RootBeadIDMetadataKey] = "workflow"
				a.Metadata[beadmeta.ContinuationGroupMetadataKey] = "steps"
			case "blocked-priority-sibling":
				b.Status, b.Assignee, b.IsBlocked = "open", v.assignee(), boolPtr(true)
				b.Priority, a.Priority = intPtr(0), intPtr(2)
			case "named-cross-store-trigger":
				meta["pool_managed"] = "false"
				meta["session_origin"] = "named"
				meta["configured_named_identity"] = v.Alias
				meta["configured_named_session"] = "true"
				meta[beadmeta.TriggerBeadStoreRefMetadataKey] = "rig:elsewhere"
			case "bound-drain-work-dir-only", "unbound-drain-work-dir-only":
				a.Metadata[beadmeta.WorkDirMetadataKey] = "/work/ga-gwav/member"
				if strings.HasPrefix(scenario, "unbound") {
					meta[beadmeta.TriggerBeadIDMetadataKey] = ""
				}
			case "branch-only":
				a.Metadata[beadmeta.WorkBranchMetadataKey] = "work/member"
			default:
				meta[beadmeta.TriggerBeadIDMetadataKey] = ""
				a = hookClaimManagedWorkspaceBead(a.ID, "/work/ga-gwav/a")
				b = hookClaimManagedWorkspaceBead(b.ID, "/work/ga-gwav/b")
				a.Metadata[beadmeta.RoutedToMetadataKey], b.Metadata[beadmeta.RoutedToMetadataKey] = v.Template, v.Template
				if !strings.Contains(scenario, "routed") {
					a.Assignee, b.Assignee = v.assignee(), v.assignee()
				}
				if strings.Contains(scenario, "adopted") {
					b.Status = "in_progress"
				}
				if strings.Contains(scenario, "missing-branch") {
					delete(b.Metadata, beadmeta.WorkBranchMetadataKey)
				}
			}
			if err := own.mem.SetMetadataBatch(v.ID, meta); err != nil {
				t.Fatal(err)
			}
			fresh, err := v.front.VerifySession(v.ID, "own-fixture", 1)
			if err != nil {
				t.Fatal(err)
			}
			v.VerifiedSession = fresh
			candidates := []beads.Bead{b, a}
			triggerStore := beads.NewMemStoreFrom(1, []beads.Bead{b}, nil)
			if scenario == "named-cross-store-trigger" {
				// The authoritative closed trigger is in another ledger, outside
				// this work query. No hook-side cleanup or lookup is needed.
				candidates = []beads.Bead{a}
			}
			work := beads.NewMemStoreFrom(1, candidates, nil)
			installVerifiedWorkStore(t, v, work)
			query, err := json.Marshal(candidates)
			if err != nil {
				t.Fatal(err)
			}
			var claims []string
			ops := hookClaimOps{
				Runner:           func(string, string) (string, error) { return string(query), nil },
				ReadWorkMeta:     func(_ context.Context, _ string, _ []string, id, _ string) (beads.Bead, error) { return work.Get(id) },
				ListContinuation: func(context.Context, string, []string, string, string) ([]beads.Bead, error) { return nil, nil },
				Claim: func(_ context.Context, _ string, _ []string, id, assignee string) (beads.Bead, bool, error) {
					claims = append(claims, id)
					status := "in_progress"
					if err := work.Update(id, beads.UpdateOpts{Status: &status, Assignee: &assignee}); err != nil {
						return beads.Bead{}, false, err
					}
					row, err := work.Get(id)
					return row, err == nil, err
				},
				PublishRunMap: noopPublishRunMap,
			}
			if strings.Contains(scenario, "missing-branch") {
				ops.ResolveWorkBranch = func(hookClaimWorkTree) string { return "resolved-branch" }
			}
			bindVerifiedHookOps(&ops, v)
			var out, diagnostic bytes.Buffer
			code := doHookClaim("query", "", hookClaimOptions{
				Verified: v, Assignee: v.assignee(),
				IdentityCandidates: v.identifiers(), RouteTargets: []string{v.Template}, JSON: true,
			}, ops, &out, &diagnostic)
			result := decodeClaimIdentityRaceResult(t, &out)
			if code != 0 || result.Action != "work" || result.BeadID != a.ID || strings.Join(claims, ",") != a.ID {
				t.Fatalf("code=%d result=%+v claims=%v stderr=%s", code, result, claims, &diagnostic)
			}
			if scenario != "named-cross-store-trigger" {
				triggerStore = work
			}
			if got := mustGetTestBead(t, triggerStore, b.ID); got.Revision != b.Revision {
				t.Fatal("skipped candidate was mutated")
			}
		})
	}
}

func TestHookClaimRound4ControlOwnershipBlocksBind(t *testing.T) {
	bp, owner, _ := claimGuardFixture(t, 5*time.Minute, 2)
	mem := bp.beadStore.(*beads.MemStore)
	if err := mem.SetMetadataBatch(owner.ID, map[string]string{"instance_token": "fixture", "generation": "1"}); err != nil {
		t.Fatal(err)
	}
	front := session.NewStore(beads.SessionStore{Store: mem})
	identity, err := front.VerifySession(owner.ID, "fixture", 1)
	if err != nil {
		t.Fatal(err)
	}
	v := &VerifiedSession{VerifiedSession: identity, front: front}
	if err := mem.SetMetadataBatch("work-a", map[string]string{
		beadmeta.KindMetadataKey: "check", beadmeta.SessionIDMetadataKey: "", beadmeta.SessionNameMetadataKey: "",
	}); err != nil {
		t.Fatal(err)
	}
	installVerifiedWorkStore(t, v, mem)
	fenced, err := fenceVerifiedHookWork(mustGetTestBead(t, mem, "work-a"), hookClaimOptions{Verified: v}, hookClaimOps{}, "")
	if err != nil || !verifiedHookOwnsWork(v, fenced) {
		t.Fatalf("control claim was not fenced: %v", err)
	}
	info, err := front.ReadVerified(identity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bindPoolSessionTriggerBead(bp, &bp.city.Agents[0], "worker-1", info, SessionRequest{WorkBeadID: "work-b"}); err == nil {
		t.Fatal("bound B while verified control A is held")
	}
	assertClaimGuardOwner(t, mem, owner)
	if fenced.Metadata[beadmeta.SessionIDMetadataKey] != "" || fenced.Metadata[beadmeta.SessionNameMetadataKey] != "" {
		t.Fatal("control work gained execution back-references")
	}
}

type failAfterNewPublication struct{ publish func() }

func (w failAfterNewPublication) Write([]byte) (int, error) {
	w.publish()
	return 0, io.ErrClosedPipe
}

func TestHookClaimRound4OldDeliveryFailureKeepsNewPublication(t *testing.T) {
	v, own := newVerifiedHookFixture(t)
	work, bead := verifiedWorkFixture(t, v)
	ops := hookClaimOps{PublishRunMap: noopPublishRunMap, EmitClaimReleased: func(hookClaimReleaseRecord) {}}
	ops.applyDefaults()
	var newStamp, newAttempt string
	writer := failAfterNewPublication{publish: func() {
		identity, err := v.front.VerifySession(v.ID, "own-fixture", 1)
		if err != nil {
			t.Fatal(err)
		}
		next := *v
		next.VerifiedSession = identity
		if code := writeHookClaimWorkResultForBead(hookClaimJSONResult{Action: "work", BeadID: bead.ID}, bead,
			hookClaimOptions{Verified: &next, Assignee: v.assignee(), JSON: true}, ops, "", false, io.Discard, io.Discard); code != 0 {
			t.Fatal("new publication failed")
		}
		newStamp = mustGetTestBead(t, own.mem, v.ID).Metadata[session.CurrentClaimStampMetadataKey]
		newAttempt = mustGetTestBead(t, work, bead.ID).Metadata[verifiedHookAttemptKey]
	}}
	code := writeHookClaimWorkResultForBead(hookClaimJSONResult{Action: "work", BeadID: bead.ID}, bead,
		hookClaimOptions{Verified: v, Assignee: v.assignee(), JSON: true}, ops, "", true, writer, io.Discard)
	row, held := mustGetTestBead(t, own.mem, v.ID), mustGetTestBead(t, work, bead.ID)
	if code != 1 || row.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey] != bead.ID || row.Metadata[session.CurrentClaimStampMetadataKey] != newStamp ||
		held.Status != "in_progress" || held.Metadata[verifiedHookAttemptKey] != newAttempt {
		t.Fatal("old delivery failure cleared or released newer publication")
	}
}

func TestHookClaimRound4RefusedAdoptionDoesNotWrite(t *testing.T) {
	v, own := newVerifiedHookFixture(t)
	work, bead := verifiedWorkFixture(t, v)
	bead.Metadata = hookClaimManagedWorkspaceBead(bead.ID, "/work/ga-gwav/b").Metadata
	if err := work.SetMetadataBatch(bead.ID, bead.Metadata); err != nil {
		t.Fatal(err)
	}
	if err := own.mem.SetMetadataBatch(v.ID, map[string]string{"pool_managed": "true", "work_dir": "/work/ga-gwav/a", beadmeta.CurrentClaimBeadIDMetadataKey: bead.ID}); err != nil {
		t.Fatal(err)
	}
	before := mustGetTestBead(t, work, bead.ID)
	for range 2 {
		if code := writeHookClaimWorkResultForBead(hookClaimJSONResult{Action: "work"}, bead,
			hookClaimOptions{Verified: v, JSON: true}, hookClaimOps{}, "", false, io.Discard, io.Discard); code != 1 {
			t.Fatal("incompatible adoption accepted")
		}
	}
	if own.writes != 0 || mustGetTestBead(t, work, bead.ID).Revision != before.Revision {
		t.Fatal("refused adoption rewrote a claim fence")
	}
}

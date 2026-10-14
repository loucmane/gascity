package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/session"
)

func TestRound4DControlWorkFenceAndRelease(t *testing.T) {
	for _, kind := range beadmeta.ControlKinds {
		for _, release := range []string{"drain", "pending"} {
			t.Run(kind+"/"+release, func(t *testing.T) {
				v, _ := newVerifiedHookFixture(t)
				b := beads.Bead{
					ID: "control", Type: "task", Status: "in_progress", Assignee: v.assignee(), Revision: 1,
					Metadata: map[string]string{beadmeta.KindMetadataKey: " " + kind + " "},
				}
				mem := beads.NewMemStoreFrom(1, []beads.Bead{b}, nil)
				installVerifiedWorkStore(t, v, mem)
				opts := hookClaimOptions{Verified: v, Assignee: v.assignee()}
				first, err := fenceVerifiedHookWork(b, opts, hookClaimOps{}, "")
				if err != nil {
					t.Fatal(err)
				}
				second, err := fenceVerifiedHookWork(b, opts, hookClaimOps{}, "")
				if err != nil {
					t.Fatal(err)
				}
				live, _ := mem.Get(b.ID)
				if live.Revision != 3 || live.Metadata[verifiedHookIncarnationKey] != v.workIncarnation() ||
					live.Metadata[verifiedHookAttemptKey] == "" || first.Metadata[verifiedHookAttemptKey] == second.Metadata[verifiedHookAttemptKey] {
					t.Fatal("control acquisition/adoption failed to advance the work fence")
				}
				if live.Metadata[beadmeta.SessionIDMetadataKey] != "" || live.Metadata[beadmeta.SessionNameMetadataKey] != "" {
					t.Fatal("control work acquired forbidden session back-references")
				}
				if released, err := releaseVerifiedHookPending(first, opts, hookClaimOps{}, ""); err != nil || released {
					t.Fatalf("old control attempt released later adoption: %v %v", released, err)
				}
				if release == "drain" {
					if err := releaseVerifiedHookClaims(v, mem); err != nil {
						t.Fatal(err)
					}
				} else if released, err := releaseVerifiedHookPending(second, opts, hookClaimOps{}, ""); err != nil || !released {
					t.Fatalf("control pending release: %v %v", released, err)
				}
				live, _ = mem.Get(b.ID)
				if live.Status != "open" || live.Assignee != "" || live.Metadata[verifiedHookIncarnationKey] != "" || live.Metadata[verifiedHookAttemptKey] != "" {
					t.Fatal("control work was not released")
				}
			})
		}
	}
}

func TestRound4DControlOwnershipRequiresAssigneeAndMarkers(t *testing.T) {
	v, _ := newVerifiedHookFixture(t)
	for _, missing := range []string{"assignee", "incarnation", "attempt"} {
		t.Run(missing, func(t *testing.T) {
			b := beads.Bead{ID: "control", Status: "in_progress", Assignee: v.assignee(), Metadata: map[string]string{
				beadmeta.KindMetadataKey: "check", verifiedHookIncarnationKey: v.workIncarnation(), verifiedHookAttemptKey: "own-attempt",
			}}
			switch missing {
			case "assignee":
				b.Assignee = "other"
			case "incarnation":
				b.Metadata[verifiedHookIncarnationKey] = "2:other"
			case "attempt":
				delete(b.Metadata, verifiedHookAttemptKey)
			}
			if verifiedHookOwnsWork(v, b) {
				t.Fatal("control work without ownership proof accepted")
			}
		})
	}
}

func TestRound4DClaimOwnershipMatchesMarkedIncarnation(t *testing.T) {
	info := session.Info{ID: "s-own", SessionName: "worker", Generation: "2", InstanceToken: "current"}
	for _, tc := range []struct {
		name, marker, generation, token string
		want                            bool
	}{
		{"legacy", "", "2", "current", true},
		{"current", "2:" + drainAckInstanceTokenDigest("current"), "2", "current", true},
		{"old-token", "2:" + drainAckInstanceTokenDigest("old"), "2", "current", false},
		{"old-generation", "1:" + drainAckInstanceTokenDigest("current"), "2", "current", false},
		{"missing-token", "2:" + drainAckInstanceTokenDigest(""), "2", "", false},
		{"invalid-generation", "0:" + drainAckInstanceTokenDigest("current"), "0", "current", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info.Generation, info.InstanceToken = tc.generation, tc.token
			b := beads.Bead{Status: "in_progress", Assignee: info.ID, Metadata: map[string]string{
				beadmeta.SessionIDMetadataKey: info.ID, verifiedHookIncarnationKey: tc.marker,
			}}
			if got := claimedWorkStillHeldBy(b, info); got != tc.want {
				t.Fatalf("held=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestRound4DAdoptionNoopStampSurvivesLaterFailure(t *testing.T) {
	for _, alreadyStamped := range []bool{false, true} {
		name := "new-stamp"
		if alreadyStamped {
			name = "existing-stamp"
		}
		t.Run(name, func(t *testing.T) {
			v, own := newVerifiedHookFixture(t)
			mem, bead := verifiedWorkFixture(t, v)
			if err := mem.Update(bead.ID, beads.UpdateOpts{Metadata: map[string]string{
				beadmeta.RootBeadIDMetadataKey: "root", beadmeta.ContinuationGroupMetadataKey: "group",
			}}); err != nil {
				t.Fatal(err)
			}
			if alreadyStamped {
				if err := own.mem.SetMetadata(v.ID, beadmeta.CurrentClaimBeadIDMetadataKey, bead.ID); err != nil {
					t.Fatal(err)
				}
				v.CurrentClaim = bead.ID
			}
			ops := hookClaimOps{
				PublishRunMap: noopPublishRunMap, ResolveSessionWorkDir: func(string) string { return "" },
				ListContinuation: func(context.Context, string, []string, string, string) ([]beads.Bead, error) {
					return nil, errors.New("fixture continuation unavailable")
				},
			}
			ops.applyDefaults()
			opts := hookClaimOptions{Verified: v, Assignee: v.assignee(), JSON: true}
			if code := writeHookClaimWorkResultForBead(hookClaimJSONResult{Action: "work", BeadID: bead.ID}, bead, opts, ops, "", false, io.Discard, io.Discard); code != 1 {
				t.Fatal("continuation failure did not fail delivery")
			}
			row, _ := own.mem.Get(v.ID)
			wantClaim, wantWrites := "", 2
			if alreadyStamped {
				wantClaim, wantWrites = bead.ID, 0
			}
			if row.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey] != wantClaim || own.writes != wantWrites {
				t.Fatalf("claim=%q writes=%d, want %q/%d", row.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey], own.writes, wantClaim, wantWrites)
			}
			live, _ := mem.Get(bead.ID)
			if live.Status != "in_progress" {
				t.Fatal("adopted work was released")
			}
		})
	}
}

func TestRound4DWakeAckReadersMatchRuntime(t *testing.T) {
	for _, tc := range []struct {
		name, value, token, epoch string
		want                      bool
	}{
		{"legacy", "1", "", "", true},
		{"current", "incarnation:2:" + drainAckInstanceTokenDigest("current"), "current", "2", true},
		{"wrong-token", "incarnation:2:" + drainAckInstanceTokenDigest("old"), "current", "2", false},
		{"wrong-generation", "incarnation:1:" + drainAckInstanceTokenDigest("current"), "current", "2", false},
		{"missing-epoch", "incarnation:2:" + drainAckInstanceTokenDigest("current"), "current", "", false},
		{"malformed", "incarnation:bad", "current", "2", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newReconcilerTestEnv()
			e.addDesired("worker", "worker", true)
			row := e.createSessionBead("worker", "worker")
			mustSetMeta(t, e.sp, "worker", "GC_DRAIN_ACK", tc.value)
			mustSetMeta(t, e.sp, "worker", "GC_INSTANCE_TOKEN", tc.token)
			mustSetMeta(t, e.sp, "worker", "GC_RUNTIME_EPOCH", tc.epoch)
			if got := staleOrLegacyDrainAckBeforeStart(row, e.sp, "worker"); got != tc.want {
				t.Fatalf("raw ack=%v, want %v", got, tc.want)
			}
			if got := staleOrLegacyDrainAckBeforeStartInfo(e.sessionInfo(row.ID), e.sp, "worker"); got != tc.want {
				t.Fatalf("info ack=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestRound4DHookCurrentRejectsInvalidEpochBeforeStore(t *testing.T) {
	for _, epoch := range []string{"", "invalid", "0", "-1", "99999999999999999999999999999999"} {
		t.Run(epoch, func(t *testing.T) {
			clearGCEnv(t)
			t.Setenv("GC_SESSION_ID", "s-own")
			t.Setenv("GC_INSTANCE_TOKEN", "fixture")
			t.Setenv("GC_RUNTIME_EPOCH", epoch)
			saved := hookCurrentSessionFrontDoor
			t.Cleanup(func() { hookCurrentSessionFrontDoor = saved })
			opened := false
			hookCurrentSessionFrontDoor = func() (*session.Store, error) {
				opened = true
				return nil, errors.New("fixture store should not open")
			}
			var out, stderr bytes.Buffer
			code := cmdHookCurrent(true, &out, &stderr)
			if code != 1 || opened || out.Len() != 0 || !strings.Contains(stderr.String(), "GC_RUNTIME_EPOCH") {
				t.Fatalf("invalid epoch: code=%d opened=%v out=%s error=%s", code, opened, &out, &stderr)
			}
		})
	}
}

func TestRound4DHookCurrentHelpNamesIncarnation(t *testing.T) {
	long := newHookCurrentCmd(io.Discard, io.Discard).Long
	for _, name := range []string{"GC_SESSION_ID", "GC_INSTANCE_TOKEN", "GC_RUNTIME_EPOCH"} {
		if !strings.Contains(long, name) {
			t.Fatalf("help omits %s", name)
		}
	}
}

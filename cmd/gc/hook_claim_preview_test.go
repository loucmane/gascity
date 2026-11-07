package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
)

func TestHookClaimPreviewDoesNotSpendMutationBudget(t *testing.T) {
	previous := hookClaimMutationTimeout
	hookClaimMutationTimeout = 200 * time.Millisecond
	t.Cleanup(func() { hookClaimMutationTimeout = previous })
	for _, scenario := range []struct {
		tier       string
		rejectHead bool
	}{
		{tier: "adopted"},
		{tier: "assigned"},
		{tier: "routed"},
		{tier: "reclaimed"},
		{tier: "assigned", rejectHead: true},
		{tier: "routed", rejectHead: true},
	} {
		tier := scenario.tier
		for _, previewFails := range []bool{false, true} {
			name := tier
			if scenario.rejectHead {
				name += "_after_rejected_head"
			}
			if previewFails {
				name += "/branch_unresolved"
			} else {
				name += "/resolved"
			}
			t.Run(name, func(t *testing.T) {
				v, own := newVerifiedHookFixture(t)
				const workDir = "/work/ga-gwav/preview-budget"
				if err := own.mem.SetMetadataBatch(v.ID, map[string]string{"pool_managed": "true", "work_dir": workDir}); err != nil {
					t.Fatal(err)
				}
				fresh, err := v.front.VerifySession(v.ID, "own-fixture", 1)
				if err != nil {
					t.Fatal(err)
				}
				v.VerifiedSession = fresh
				candidate := hookClaimManagedWorkspaceBead("head", workDir)
				delete(candidate.Metadata, beadmeta.WorkBranchMetadataKey)
				candidate.Metadata[beadmeta.RoutedToMetadataKey] = v.Template
				switch tier {
				case "adopted":
					candidate.Status, candidate.Assignee = "in_progress", v.assignee()
				case "assigned":
					candidate.Assignee = v.assignee()
				case "reclaimed":
					candidate.Status, candidate.Assignee = "in_progress", "stale-owner"
				}
				candidates := []beads.Bead{candidate}
				wantClaims := 1
				if tier == "adopted" {
					wantClaims = 0
				}
				if scenario.rejectHead {
					head := hookClaimManagedWorkspaceBead("rejected-head", workDir)
					head.Assignee = candidate.Assignee
					head.Metadata[beadmeta.RoutedToMetadataKey] = v.Template
					candidates = append([]beads.Bead{head}, candidates...)
					wantClaims++
				}
				work := beads.NewMemStoreFrom(1, candidates, nil)
				installVerifiedWorkStore(t, v, work)
				query, err := json.Marshal(candidates)
				if err != nil {
					t.Fatal(err)
				}
				claims, reads, reclaims, probes := 0, 0, 0, 0
				ops := hookClaimOps{
					Runner: func(string, string) (string, error) { return string(query), nil },
					ResolveWorkBranch: func(hookClaimWorkTree) string {
						probes++
						if probes == 1 {
							if scenario.rejectHead && claims != 1 {
								t.Fatalf("slow preview preceded head rejection: claims=%d", claims)
							}
							// Real elapsed time is the subject: exceed the mutation
							// budget while staying within the independent Git budget.
							ctx, cancel := context.WithTimeout(context.Background(), 2*hookClaimMutationTimeout)
							defer cancel()
							<-ctx.Done()
							if previewFails {
								return ""
							}
						}
						return "work/head"
					},
					ReadWorkMeta: func(ctx context.Context, _ string, _ []string, id, _ string) (beads.Bead, error) {
						reads++
						if err := ctx.Err(); err != nil {
							t.Fatalf("certification started with expired budget: %v", err)
						}
						return work.Get(id)
					},
					ReclaimStale: func(ctx context.Context, _ string, _ []string, id string) (bool, string, error) {
						reclaims++
						if err := ctx.Err(); err != nil {
							t.Fatalf("reclaim started with expired budget: %v", err)
						}
						status, assignee := "open", ""
						err := work.Update(id, beads.UpdateOpts{Status: &status, Assignee: &assignee})
						return err == nil, "stale-owner", err
					},
					Claim: func(ctx context.Context, _ string, _ []string, id, assignee string) (beads.Bead, bool, error) {
						claims++
						if err := ctx.Err(); err != nil {
							t.Fatalf("claim started with expired budget: %v", err)
						}
						if id == "rejected-head" {
							return beads.Bead{}, false, nil
						}
						status := "in_progress"
						if err := work.Update(id, beads.UpdateOpts{Status: &status, Assignee: &assignee}); err != nil {
							return beads.Bead{}, false, err
						}
						row, err := work.Get(id)
						return row, err == nil, err
					},
					ListContinuation:            func(context.Context, string, []string, string, string) ([]beads.Bead, error) { return nil, nil },
					EmitHookClaimReclaimedStale: func(string, string, string) {},
					PublishRunMap:               noopPublishRunMap,
				}
				bindVerifiedHookOps(&ops, v)
				var out, diagnostic bytes.Buffer
				code := doHookClaim("query", "", hookClaimOptions{
					Verified: v, Assignee: v.assignee(), IdentityCandidates: v.identifiers(),
					RouteTargets: []string{v.Template}, AutoReclaimStaleClaims: tier == "reclaimed", JSON: true,
				}, ops, &out, &diagnostic)
				if code != 0 {
					t.Fatalf("code=%d stdout=%s stderr=%s", code, &out, &diagnostic)
				}
				result := decodeClaimIdentityRaceResult(t, &out)
				if result.Action != "work" || result.BeadID != candidate.ID || probes != 2 {
					t.Fatalf("result=%+v probes=%d stderr=%s", result, probes, &diagnostic)
				}
				if claims != wantClaims || (tier == "adopted" && reads != 1) || (tier == "reclaimed" && reclaims != 1) {
					t.Fatalf("claims=%d reads=%d reclaims=%d", claims, reads, reclaims)
				}
				row := mustGetTestBead(t, work, candidate.ID)
				if row.Status != "in_progress" || row.Assignee != v.assignee() || row.Metadata[beadmeta.WorkBranchMetadataKey] != "work/head" {
					t.Fatalf("head was not claimed and fenced: %+v", row)
				}
			})
		}
	}
}

func TestHookClaimPreviewMemoUsesRevision(t *testing.T) {
	v, own := newVerifiedHookFixture(t)
	if err := own.mem.SetMetadataBatch(v.ID, map[string]string{"pool_managed": "true", "work_dir": "/work/ga-gwav/a"}); err != nil {
		t.Fatal(err)
	}
	info, err := v.front.ReadVerified(v.VerifiedSession)
	if err != nil {
		t.Fatal(err)
	}
	opts := hookClaimOptions{preview: &hookClaimPreview{candidates: make(map[hookClaimCandidateKey]beads.Bead)}}
	probes := 0
	ops := hookClaimOps{ResolveWorkBranch: func(hookClaimWorkTree) string { probes++; return "work/head" }}
	for _, dir := range []string{"b", "a"} {
		candidate := hookClaimManagedWorkspaceBead("same-id", "/work/ga-gwav/"+dir)
		candidate.Revision = int64(probes + 1)
		delete(candidate.Metadata, beadmeta.WorkBranchMetadataKey)
		query, err := json.Marshal([]beads.Bead{candidate})
		if err != nil {
			t.Fatal(err)
		}
		rows, skipped, err := decodeHookClaimBeads(string(query))
		if err != nil || len(skipped) != 0 || len(rows) != 1 {
			t.Fatalf("decode: rows=%v skipped=%v err=%v", rows, skipped, err)
		}
		candidate = rows[0]
		if candidate.Revision != 0 {
			t.Fatalf("query revision=%d, want zero (Revision is not on the wire)", candidate.Revision)
		}
		for range 2 {
			if matches := hookClaimWorktreeCompatible(info, candidate, opts, ops, ""); matches != (dir == "a") {
				t.Fatalf("revision=%d tree=%s matches=%v", candidate.Revision, dir, matches)
			}
		}
		if candidate.Metadata[beadmeta.WorkBranchMetadataKey] != "" {
			t.Fatal("preview mutated query metadata")
		}
	}
	if probes != 2 {
		t.Fatalf("probes=%d, want one per distinct query workspace", probes)
	}
}

func TestHookClaimPreviewMemoUsesStoreDir(t *testing.T) {
	preview := &hookClaimPreview{}
	for _, dir := range []string{"/work/ga-gwav/store-a", "/work/ga-gwav/store-b", "/work/ga-gwav/store-a"} {
		if head := preview.head(dir); strings.TrimSpace(head.dir) != dir {
			t.Fatalf("head dir=%q, want %q", head.dir, dir)
		}
	}
}

func TestHookClaimPreviewRetriesUnavailableStoreHeadAtFence(t *testing.T) {
	v, _ := newVerifiedHookFixture(t)
	work, bead := verifiedWorkFixture(t, v)
	bead.Status = "open"
	if err := work.Update(bead.ID, beads.UpdateOpts{Status: &bead.Status}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	preview := &hookClaimPreview{storeHead: &hookClaimStoreHead{dir: dir, probe: hookClaimProbeUnavailable}}
	opts := hookClaimOptions{Verified: v, Assignee: v.assignee(), IdentityCandidates: v.identifiers(), JSON: true, preview: preview}
	ops := hookClaimOps{
		Claim: func(_ context.Context, _ string, _ []string, id, assignee string) (beads.Bead, bool, error) {
			status := "in_progress"
			if err := work.Update(id, beads.UpdateOpts{Status: &status, Assignee: &assignee}); err != nil {
				return beads.Bead{}, false, err
			}
			row, err := work.Get(id)
			return row, err == nil, err
		},
		PublishRunMap: noopPublishRunMap,
	}
	bindVerifiedHookOps(&ops, v)
	var out, diagnostic bytes.Buffer
	result := claimFirstReadyHookAssignment([]beads.Bead{bead}, opts, ops, dir, &out, &diagnostic)
	if !result.terminal || result.code != 0 {
		t.Fatalf("result=%+v stderr=%s", result, &diagnostic)
	}
	if receipt := decodeClaimIdentityRaceResult(t, &out); receipt.Action != "work" || receipt.BeadID != bead.ID {
		t.Fatalf("receipt=%+v", receipt)
	}
	if preview.storeHead == nil || preview.storeHead.probe == hookClaimProbeUnavailable {
		t.Fatalf("fence reused unavailable store head: %+v", preview.storeHead)
	}
	if row := mustGetTestBead(t, work, bead.ID); row.Metadata[verifiedHookAttemptKey] == "" {
		t.Fatal("work was not fenced")
	}
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/session"
)

// ga-g189: pool workers sharing a route must only claim their own managed tree,
// regardless of which bead sorts first or which worker polls first.
func TestHookClaimManagedWorktreeSessionOrder(t *testing.T) {
	for _, candidateOrder := range [][]string{{"A", "B"}, {"B", "A"}} {
		for _, sessionOrder := range [][]string{{"A", "B"}, {"B", "A"}} {
			t.Run(strings.Join(candidateOrder, "")+"/"+strings.Join(sessionOrder, ""), func(t *testing.T) {
				root := t.TempDir()
				work := beads.NewMemStoreFrom(1, []beads.Bead{
					hookClaimManagedWorkspaceBead("A", filepath.Join(root, "A")),
					hookClaimManagedWorkspaceBead("B", filepath.Join(root, "B")),
				}, nil)
				for _, id := range sessionOrder {
					v := hookClaimWorkspaceSession(t, id, filepath.Join(root, id))
					var candidates []beads.Bead
					for _, candidateID := range candidateOrder {
						b, err := work.Get(candidateID)
						if err != nil {
							t.Fatal(err)
						}
						candidates = append(candidates, b)
					}
					result, attempts, reclaimed := runHookClaimWorkspaceCandidates(t, v, work, candidates, false)
					if result.BeadID != id || strings.Join(attempts, ",") != id || len(reclaimed) != 0 {
						t.Fatalf("session %s claimed %q; attempts=%v reclaimed=%v, want only %s", id, result.BeadID, attempts, reclaimed, id)
					}
				}
				for _, id := range sessionOrder {
					b, err := work.Get(id)
					if err != nil || b.Assignee != "session-"+id || b.Status != "in_progress" {
						t.Fatalf("bead %s = %+v, %v; want its own session", id, b, err)
					}
				}
			})
		}
	}
}

func TestHookClaimManagedWorktreeReclaim(t *testing.T) {
	for _, name := range []string{"foreign_only", "own_after_foreign"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			v := hookClaimWorkspaceSession(t, "A", filepath.Join(root, "A"))
			other := hookClaimManagedWorkspaceBead("B", filepath.Join(root, "B"))
			other.Assignee, other.Status = "dead-B", "in_progress"
			candidates := []beads.Bead{other}
			want := ""
			if name == "own_after_foreign" {
				own := hookClaimManagedWorkspaceBead("A", filepath.Join(root, "A"))
				own.Assignee, own.Status = "dead-A", "in_progress"
				candidates = append(candidates, own)
				want = "A"
			}
			work := beads.NewMemStoreFrom(1, candidates, nil)
			result, attempts, reclaimed := runHookClaimWorkspaceCandidates(t, v, work, candidates, true)
			if result.BeadID != want || strings.Join(attempts, ",") != want || strings.Join(reclaimed, ",") != want {
				t.Fatalf("result=%+v claims=%v reclaims=%v; want only %q", result, attempts, reclaimed, want)
			}
			got, err := work.Get("B")
			if err != nil || got.Assignee != other.Assignee || got.Revision != other.Revision {
				t.Fatalf("foreign candidate changed: %+v, %v", got, err)
			}
		})
	}
}

func TestHookClaimManagedWorktreeCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*beads.Bead)
		want   string
	}{
		{name: "different managed tree"},
		{name: "no work dir", change: func(b *beads.Bead) { delete(b.Metadata, beadmeta.WorkDirMetadataKey) }, want: "B"},
		{name: "work dir only", change: func(b *beads.Bead) {
			b.Metadata = map[string]string{beadmeta.RoutedToMetadataKey: "rig/worker", beadmeta.WorkDirMetadataKey: b.Metadata[beadmeta.WorkDirMetadataKey]}
		}, want: "B"},
		{name: "branch without ownership", change: func(b *beads.Bead) {
			for _, key := range worktreeOwnershipEvidenceKeys {
				delete(b.Metadata, key)
			}
		}, want: "B"},
		{name: "partial ownership", change: func(b *beads.Bead) { delete(b.Metadata, beadmeta.WorktreeOwnerMetadataKey) }, want: "B"},
		{name: "missing branch", change: func(b *beads.Bead) { delete(b.Metadata, beadmeta.WorkBranchMetadataKey) }, want: "B"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			v := hookClaimWorkspaceSession(t, "A", filepath.Join(root, "A"))
			b := hookClaimManagedWorkspaceBead("B", filepath.Join(root, "B"))
			if tc.change != nil {
				tc.change(&b)
			}
			candidates := []beads.Bead{b}
			if tc.want != "" {
				candidates = append(candidates, hookClaimManagedWorkspaceBead("A", filepath.Join(root, "A")))
			}
			work := beads.NewMemStoreFrom(1, candidates, nil)
			result, attempts, _ := runHookClaimWorkspaceCandidates(t, v, work, candidates, false)
			if result.BeadID != tc.want || strings.Join(attempts, ",") != tc.want {
				t.Fatalf("result=%+v claims=%v; want %q", result, attempts, tc.want)
			}
		})
	}
}

func TestHookClaimManagedWorktreeUnknownSessionDirectory(t *testing.T) {
	for _, dir := range []string{"", "relative/tree"} {
		t.Run(dir, func(t *testing.T) {
			v := hookClaimWorkspaceSession(t, "A", dir)
			b := hookClaimManagedWorkspaceBead("A", filepath.Join(t.TempDir(), "A"))
			work := beads.NewMemStoreFrom(1, []beads.Bead{b}, nil)
			result, attempts, _ := runHookClaimWorkspaceCandidates(t, v, work, []beads.Bead{b}, false)
			if result.BeadID != "" || len(attempts) != 0 {
				t.Fatalf("unknown session directory claimed work: result=%+v claims=%v", result, attempts)
			}
		})
	}
}

func TestHookClaimManagedWorktreeKeepsNonPoolBehavior(t *testing.T) {
	v, _ := newVerifiedHookFixture(t)
	b := hookClaimManagedWorkspaceBead("B", filepath.Join(t.TempDir(), "B"))
	work := beads.NewMemStoreFrom(1, []beads.Bead{b}, nil)
	result, attempts, _ := runHookClaimWorkspaceCandidates(t, v, work, []beads.Bead{b}, false)
	if result.BeadID != "B" || strings.Join(attempts, ",") != "B" {
		t.Fatalf("non-pool session changed behavior: result=%+v claims=%v", result, attempts)
	}
}

func TestHookClaimManagedWorktreeSessionReadFailure(t *testing.T) {
	v, own := newVerifiedHookFixture(t)
	own.readError = errors.New("session store unavailable")
	b := hookClaimManagedWorkspaceBead("B", filepath.Join(t.TempDir(), "B"))
	ops := hookClaimOps{
		Claim: func(context.Context, string, []string, string, string) (beads.Bead, bool, error) {
			t.Fatal("claimed without verifying the session directory")
			return beads.Bead{}, false, nil
		},
	}
	var stdout, stderr bytes.Buffer
	outcome := claimFirstEligibleHookCandidate([]beads.Bead{b}, hookClaimOptions{
		Verified: v, RouteTargets: []string{"rig/worker"}, JSON: true,
	}, ops, "", &stdout, &stderr)
	var result hookClaimJSONResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !outcome.terminal || outcome.code != 1 || result.Reason != hookClaimReasonClaimsErrored ||
		!strings.Contains(stderr.String(), own.readError.Error()) {
		t.Fatalf("session read failure: outcome=%+v result=%+v stderr=%q", outcome, result, stderr.String())
	}
}

func TestHookClaimManagedWorktreeSymlink(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "tree")
	alias := filepath.Join(root, "alias")
	if err := os.Mkdir(tree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(tree, alias); err != nil {
		t.Fatal(err)
	}
	v := hookClaimWorkspaceSession(t, "A", alias)
	b := hookClaimManagedWorkspaceBead("A", tree)
	work := beads.NewMemStoreFrom(1, []beads.Bead{b}, nil)
	result, attempts, _ := runHookClaimWorkspaceCandidates(t, v, work, []beads.Bead{b}, false)
	if result.BeadID != "A" || strings.Join(attempts, ",") != "A" {
		t.Fatalf("same tree through alias: result=%+v claims=%v", result, attempts)
	}
}

func hookClaimManagedWorkspaceBead(id, dir string) beads.Bead {
	return beads.Bead{
		ID: id, Type: "task", Status: "open", Revision: 1,
		Metadata: map[string]string{
			beadmeta.RoutedToMetadataKey:           "rig/worker",
			beadmeta.WorkDirMetadataKey:            dir,
			beadmeta.WorkBranchMetadataKey:         "work/" + id,
			beadmeta.WorktreeRepoMetadataKey:       filepath.Join(filepath.Dir(dir), "repo"),
			beadmeta.WorktreeRootMetadataKey:       filepath.Dir(dir),
			beadmeta.WorktreeBaseRefMetadataKey:    "main",
			beadmeta.WorktreeBaseSHAMetadataKey:    strings.Repeat("a", 40),
			beadmeta.WorktreeCreatorMetadataKey:    "gc-sling",
			beadmeta.WorktreeOwnerMetadataKey:      "gc-sling",
			beadmeta.WorktreeGenerationMetadataKey: "1",
			beadmeta.WorktreeLifecycleMetadataKey:  "active",
		},
	}
}

func hookClaimWorkspaceSession(t *testing.T, id, dir string) *VerifiedSession {
	t.Helper()
	id = "session-" + id
	mem := beads.NewMemStoreFrom(1, []beads.Bead{{
		ID: id, Type: session.BeadType, Status: "open", Revision: 1,
		Metadata: map[string]string{
			"instance_token": "fixture", "generation": "1", "state": "active",
			"template": "rig/worker", "session_name": id, "pool_managed": "true",
			"work_dir": dir,
		},
	}}, nil)
	front := session.NewStore(beads.SessionStore{Store: mem})
	verified, err := front.VerifySession(id, "fixture", 1)
	if err != nil {
		t.Fatal(err)
	}
	return &VerifiedSession{VerifiedSession: verified, front: front}
}

func runHookClaimWorkspaceCandidates(t *testing.T, v *VerifiedSession, work *beads.MemStore, candidates []beads.Bead, reclaim bool) (hookClaimJSONResult, []string, []string) {
	t.Helper()
	installVerifiedWorkStore(t, v, work)
	var attempts, reclaimed []string
	ops := hookClaimOps{
		Claim: func(_ context.Context, _ string, _ []string, id, assignee string) (beads.Bead, bool, error) {
			attempts = append(attempts, id)
			status := "in_progress"
			if err := work.Update(id, beads.UpdateOpts{Assignee: &assignee, Status: &status}); err != nil {
				return beads.Bead{}, false, err
			}
			b, err := work.Get(id)
			return b, true, err
		},
		ReclaimStale: func(_ context.Context, _ string, _ []string, id string) (bool, string, error) {
			reclaimed = append(reclaimed, id)
			b, err := work.Get(id)
			return err == nil, b.Assignee, err
		},
		EmitHookClaimReclaimedStale: func(string, string, string) {},
	}
	opts := hookClaimOptions{
		Verified: v, SessionID: v.ID, Assignee: v.assignee(), IdentityCandidates: v.identifiers(),
		RouteTargets: []string{"rig/worker"}, JSON: true, AutoReclaimStaleClaims: reclaim,
	}
	var stdout, stderr bytes.Buffer
	outcome := claimFirstEligibleHookCandidate(candidates, opts, ops, "", &stdout, &stderr)
	if outcome.code != 0 || outcome.claimsErrored {
		t.Fatalf("claim failed: outcome=%+v stdout=%q stderr=%q", outcome, stdout.String(), stderr.String())
	}
	var result hookClaimJSONResult
	if outcome.terminal {
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatalf("result %q, stderr %q: %v", stdout.String(), stderr.String(), err)
		}
	}
	return result, attempts, reclaimed
}

func TestHookClaimWithBdStoreReloadsCanonicalBeadAfterPartialMutation(t *testing.T) {
	originalRunner := hookClaimCommandRunnerWithEnvContext
	t.Cleanup(func() { hookClaimCommandRunnerWithEnvContext = originalRunner })

	var calls [][]string
	hookClaimCommandRunnerWithEnvContext = func(_ context.Context, _ map[string]string) beads.CommandRunner {
		return func(_ string, name string, args ...string) ([]byte, error) {
			if name != "bd" {
				t.Fatalf("command name = %q, want bd", name)
			}
			calls = append(calls, append([]string(nil), args...))
			switch {
			case reflect.DeepEqual(args, []string{"update", "work-1", "--claim", "--json"}):
				return []byte(`[{"id":"work-1","status":"in_progress","assignee":"worker-1","metadata":{"gc.routed_to":"rig/worker"}}]`), nil
			case reflect.DeepEqual(args, []string{"show", "--json", "work-1"}):
				return []byte(`[{"id":"work-1","status":"in_progress","assignee":"worker-1","metadata":{"gc.routed_to":"rig/worker","gc.root_bead_id":"root-1","gc.continuation_group":"review"}}]`), nil
			default:
				t.Fatalf("unexpected bd args: %#v", args)
				return nil, nil
			}
		}
	}

	claimed, ok, err := hookClaimWithBdStore(context.Background(), "/rig", nil, "work-1", "worker-1")
	if err != nil {
		t.Fatalf("hookClaimWithBdStore: %v", err)
	}
	if !ok {
		t.Fatal("hookClaimWithBdStore ok = false, want true")
	}
	if claimed.Metadata["gc.root_bead_id"] != "root-1" || claimed.Metadata["gc.continuation_group"] != "review" {
		t.Fatalf("claimed metadata = %#v, want canonical root and continuation group", claimed.Metadata)
	}
	if len(calls) != 2 {
		t.Fatalf("bd calls = %#v, want claim update followed by canonical show", calls)
	}
}

func TestDoHookClaimStopsAfterCommittedClaimReadbackFailure(t *testing.T) {
	runner := func(string, string) (string, error) {
		return `[
			{"id":"work-1","status":"open","metadata":{"gc.routed_to":"worker"}},
			{"id":"work-2","status":"open","metadata":{"gc.routed_to":"worker"}}
		]`, nil
	}
	var attempts []string
	drained := false
	ops := hookClaimOps{
		Runner: runner,
		Claim: func(_ context.Context, _ string, _ []string, beadID, assignee string) (beads.Bead, bool, error) {
			attempts = append(attempts, beadID)
			return beads.Bead{ID: beadID, Assignee: assignee}, true, errors.New("canonical read failed")
		},
		DrainAck: func(io.Writer) error {
			drained = true
			return nil
		},
	}

	var stdout, stderr bytes.Buffer
	code := doHookClaim("query", "/rig", hookClaimOptions{
		Assignee:     "worker-1",
		RouteTargets: []string{"worker"},
		DrainAck:     true,
		JSON:         true,
	}, ops, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("doHookClaim = %d, want 1", code)
	}
	if got := strings.Join(attempts, ","); got != "work-1" {
		t.Fatalf("claim attempts = %q, want only committed work-1", got)
	}
	if drained {
		t.Fatal("drain acknowledged after committed claim readback failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "claimed work-1 but loading canonical bead failed") {
		t.Fatalf("stderr = %q, want committed-claim diagnostic", stderr.String())
	}
}

func TestDoHookClaimUsesSelectedStoreContextForMutationAndContinuation(t *testing.T) {
	var claimedDir string
	var claimedEnv []string
	var listedDir string
	var listedEnv []string
	var assignedDir string
	var assignedEnv []string
	var assignedBead string

	storeDir := "rig-store"
	storeEnv := []string{"BEADS_DIR=rig-store", "GC_RIG_ROOT=rig-root"}
	candidates := []beads.Bead{{
		ID:       "bead-1",
		Status:   "open",
		Metadata: map[string]string{"gc.kind": "workflow", "gc.run_target": "route-1", "gc.root_bead_id": "root-1", "gc.continuation_group": "group-a"},
	}}
	output, err := json.Marshal(candidates)
	if err != nil {
		t.Fatalf("marshal candidates: %v", err)
	}

	ops := hookClaimOps{
		Runner: func(string, string) (string, error) { return string(output), nil },
		Claim: func(_ context.Context, dir string, env []string, beadID, assignee string) (beads.Bead, bool, error) {
			claimedDir = dir
			claimedEnv = append([]string(nil), env...)
			return beads.Bead{ID: beadID, Assignee: assignee, Status: "in_progress", Metadata: candidates[0].Metadata}, true, nil
		},
		ListContinuation: func(_ context.Context, dir string, env []string, rootID, group string) ([]beads.Bead, error) {
			listedDir = dir
			listedEnv = append([]string(nil), env...)
			if rootID != "root-1" || group != "group-a" {
				t.Fatalf("continuation lookup = (%q, %q), want (root-1, group-a)", rootID, group)
			}
			return []beads.Bead{{ID: "sib-1", Status: "open", Metadata: candidates[0].Metadata}}, nil
		},
		AssignContinuation: func(_ context.Context, dir string, env []string, beadID, assignee string) error {
			assignedDir = dir
			assignedEnv = append([]string(nil), env...)
			assignedBead = beadID
			if assignee != "worker-1" {
				t.Fatalf("assignee = %q, want worker-1", assignee)
			}
			return nil
		},
		DrainAck: func(io.Writer) error { return nil },
	}

	var stdout, stderr bytes.Buffer
	code := doHookClaim("query", storeDir, hookClaimOptions{
		Assignee:           "worker-1",
		IdentityCandidates: []string{"worker-1"},
		RouteTargets:       []string{"route-1"},
		Env:                storeEnv,
		JSON:               true,
	}, ops, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doHookClaim() = %d, want 0; stderr=%s", code, stderr.String())
	}
	if claimedDir != storeDir {
		t.Fatalf("claimedDir = %q, want %q", claimedDir, storeDir)
	}
	if listedDir != storeDir {
		t.Fatalf("listedDir = %q, want %q", listedDir, storeDir)
	}
	if assignedDir != storeDir {
		t.Fatalf("assignedDir = %q, want %q", assignedDir, storeDir)
	}
	if !reflect.DeepEqual(claimedEnv, storeEnv) {
		t.Fatalf("claimedEnv = %#v, want %#v", claimedEnv, storeEnv)
	}
	if !reflect.DeepEqual(listedEnv, storeEnv) {
		t.Fatalf("listedEnv = %#v, want %#v", listedEnv, storeEnv)
	}
	if !reflect.DeepEqual(assignedEnv, storeEnv) {
		t.Fatalf("assignedEnv = %#v, want %#v", assignedEnv, storeEnv)
	}
	if assignedBead != "sib-1" {
		t.Fatalf("assignedBead = %q, want sib-1", assignedBead)
	}
}

func TestHookClaimEnvMapUsesOnlyTheQueryEnvironment(t *testing.T) {
	t.Setenv("BEADS_DOLT_SERVER_HOST", "ambient-dolt.example.com")
	t.Setenv("BEADS_DOLT_SERVER_DATABASE", "ambient_database")
	t.Setenv("BEADS_DOLT_SERVER_TLS", "1")
	t.Setenv("BEADS_POSTGRES_HOST", "ambient-postgres.example.com")

	got := hookClaimEnvMap([]string{
		"BEADS_DOLT_SERVER_PORT=30778",
		"BEADS_POSTGRES_PORT=5432",
	}, "/rig", "worker")

	if value, ok := got["BEADS_DOLT_SERVER_HOST"]; ok {
		t.Fatalf("BEADS_DOLT_SERVER_HOST = %q, want absent ambient value", value)
	}
	if value, ok := got["BEADS_POSTGRES_HOST"]; ok {
		t.Fatalf("BEADS_POSTGRES_HOST = %q, want absent ambient value", value)
	}
	if value, ok := got["BEADS_DOLT_SERVER_DATABASE"]; ok {
		t.Fatalf("BEADS_DOLT_SERVER_DATABASE = %q, want absent ambient value", value)
	}
	if value, ok := got["BEADS_DOLT_SERVER_TLS"]; ok {
		t.Fatalf("BEADS_DOLT_SERVER_TLS = %q, want absent ambient value", value)
	}
	if got["BEADS_DOLT_SERVER_PORT"] != "30778" {
		t.Fatalf("BEADS_DOLT_SERVER_PORT = %q, want 30778", got["BEADS_DOLT_SERVER_PORT"])
	}
	if got["BEADS_POSTGRES_PORT"] != "5432" {
		t.Fatalf("BEADS_POSTGRES_PORT = %q, want 5432", got["BEADS_POSTGRES_PORT"])
	}
}

// TestHookClaimRunnerIsTheExactEnvRunner pins the wiring at the top of this
// file. Every other test in the package replaces
// hookClaimCommandRunnerWithEnvContext with a stub, so reverting it to the
// layered ExecCommandRunnerWithEnvContext would silently restore #5142
// (ambient BEADS_DOLT_* / BEADS_POSTGRES_* selectors reappearing in the claim
// mutation) with the suite still green.
func TestHookClaimRunnerIsTheExactEnvRunner(t *testing.T) {
	got := reflect.ValueOf(hookClaimCommandRunnerWithEnvContext).Pointer()
	want := reflect.ValueOf(beads.ExecCommandRunnerWithExactEnvContext).Pointer()
	if got != want {
		t.Fatal("hookClaimCommandRunnerWithEnvContext must be beads.ExecCommandRunnerWithExactEnvContext; " +
			"the layered runner re-admits ambient store selectors into the claim mutation (#5142)")
	}
}

// TestDoHookClaimSkipsBlockedRoutedHeadAndClaimsReadyBehindIt guards the
// widened-routed-tier fix: a routed tier's oldest candidate can be
// is_blocked (e.g. gated on a PR), and the hook must fall through to a
// Ready routed bead behind it rather than idle-exiting on the blocked head.
func TestDoHookClaimSkipsBlockedRoutedHeadAndClaimsReadyBehindIt(t *testing.T) {
	candidates := []beads.Bead{
		{ID: "blocked-head", Status: "open", IsBlocked: boolPtr(true), Metadata: map[string]string{"gc.routed_to": "route-1"}},
		{ID: "ready-behind", Status: "open", Metadata: map[string]string{"gc.routed_to": "route-1"}},
	}
	output, err := json.Marshal(candidates)
	if err != nil {
		t.Fatalf("marshal candidates: %v", err)
	}

	var claimedBead string
	ops := hookClaimOps{
		Runner: func(string, string) (string, error) { return string(output), nil },
		Claim: func(_ context.Context, _ string, _ []string, beadID, assignee string) (beads.Bead, bool, error) {
			claimedBead = beadID
			return beads.Bead{ID: beadID, Assignee: assignee, Status: "in_progress"}, true, nil
		},
		DrainAck: func(io.Writer) error { return nil },
	}

	var stdout, stderr bytes.Buffer
	code := doHookClaim("query", ".", hookClaimOptions{
		Assignee:           "worker-1",
		IdentityCandidates: []string{"worker-1"},
		RouteTargets:       []string{"route-1"},
		JSON:               true,
	}, ops, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doHookClaim() = %d, want 0; stderr=%s", code, stderr.String())
	}
	if claimedBead != "ready-behind" {
		t.Fatalf("claimedBead = %q, want ready-behind (blocked-head must be skipped)", claimedBead)
	}
}

func TestPreassignHookContinuationGroupPinsSiblingsToSessionID(t *testing.T) {
	tests := []struct {
		name         string
		opts         hookClaimOptions
		wantAssignee string
	}{{
		// The pin means "run this on THIS session". Only the session bead ID is an
		// identity the consumers agree on: ComputeAwakeSet matches it, and the
		// session's own re-poll queries $GC_SESSION_ID. The runtime slot label in
		// Assignee matches neither.
		name:         "session id wins over the runtime slot label",
		opts:         hookClaimOptions{Assignee: "gascity--gc__implementation-worker-5-pool", SessionID: "gcs-session-74f608f2"},
		wantAssignee: "gcs-session-74f608f2",
	}, {
		name:         "blank session id falls back to the claim assignee",
		opts:         hookClaimOptions{Assignee: "worker-1", SessionID: "   "},
		wantAssignee: "worker-1",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			claimed := beads.Bead{
				ID:       "work-1",
				Status:   "in_progress",
				Metadata: map[string]string{"gc.kind": "workflow", "gc.root_bead_id": "root-1", "gc.continuation_group": "group-a", "gc.run_target": "route-1"},
			}
			var gotAssignees []string
			opts := tc.opts
			opts.RouteTargets = []string{"route-1"}
			ops := hookClaimOps{
				ListContinuation: func(_ context.Context, _ string, _ []string, rootID, group string) ([]beads.Bead, error) {
					if rootID != "root-1" || group != "group-a" {
						t.Fatalf("continuation lookup = (%q, %q), want (root-1, group-a)", rootID, group)
					}
					return []beads.Bead{
						{ID: "sib-1", Status: "open", Metadata: claimed.Metadata},
						{ID: "sib-2", Status: "open", Metadata: claimed.Metadata},
					}, nil
				},
				AssignContinuation: func(_ context.Context, _ string, _ []string, _, assignee string) error {
					gotAssignees = append(gotAssignees, assignee)
					return nil
				},
			}

			assigned, err := preassignHookContinuationGroup(claimed, opts, ops, ".")
			if err != nil {
				t.Fatalf("preassignHookContinuationGroup() error = %v", err)
			}
			if want := []string{"sib-1", "sib-2"}; !reflect.DeepEqual(assigned, want) {
				t.Fatalf("assigned = %#v, want %#v", assigned, want)
			}
			want := []string{tc.wantAssignee, tc.wantAssignee}
			if !reflect.DeepEqual(gotAssignees, want) {
				t.Fatalf("pinned assignees = %#v, want %#v", gotAssignees, want)
			}
		})
	}
}

// TestDoHookClaimReclaimsStaleAssigneeAndClaimsSameCycle covers ga-7rj87d
// FR1/FR2/FR3: a route-matched candidate whose only claim-eligibility
// failure is an existing Assignee gets a scoped reclaim attempt, and a
// successful reclaim is retried as a normal claim in the SAME hook cycle.
func TestDoHookClaimReclaimsStaleAssigneeAndClaimsSameCycle(t *testing.T) {
	runner := func(string, string) (string, error) {
		return `[{"id":"work-1","assignee":"dead-worker","metadata":{"gc.routed_to":"worker"}}]`, nil
	}
	var reclaimCalls []string
	var claimCalls []string
	ops := hookClaimOps{
		Runner: runner,
		ReclaimStale: func(_ context.Context, _ string, _ []string, beadID string) (bool, string, error) {
			reclaimCalls = append(reclaimCalls, beadID)
			return true, "dead-worker", nil
		},
		Claim: func(_ context.Context, _ string, _ []string, beadID, assignee string) (beads.Bead, bool, error) {
			claimCalls = append(claimCalls, beadID)
			return beads.Bead{ID: beadID, Assignee: assignee, Metadata: map[string]string{"gc.routed_to": "worker"}}, true, nil
		},
		DrainAck: func(io.Writer) error { return nil },
	}

	var stdout, stderr bytes.Buffer
	code := doHookClaim("query", ".", hookClaimOptions{
		Assignee:               "worker-1",
		RouteTargets:           []string{"worker"},
		AutoReclaimStaleClaims: true,
		JSON:                   true,
	}, ops, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("doHookClaim() = %d, want 0; stderr=%s", code, stderr.String())
	}
	if got := strings.Join(reclaimCalls, ","); got != "work-1" {
		t.Fatalf("reclaim calls = %q, want scoped to work-1 only (FR2)", got)
	}
	if got := strings.Join(claimCalls, ","); got != "work-1" {
		t.Fatalf("claim calls = %q, want claim retried on work-1 in the same cycle (FR3)", got)
	}
}

// TestDoHookClaimLeavesCandidateUntouchedWhenNothingReclaimed covers
// ga-7rj87d FR4: when the scoped reclaim reports nothing reclaimed, the
// candidate must be left untouched (no claim attempt) and the hook
// continues — current behavior for an unreclaimable stale candidate.
func TestDoHookClaimLeavesCandidateUntouchedWhenNothingReclaimed(t *testing.T) {
	runner := func(string, string) (string, error) {
		return `[{"id":"work-1","assignee":"live-worker","metadata":{"gc.routed_to":"worker"}}]`, nil
	}
	var reclaimCalls []string
	claimCalled := false
	ops := hookClaimOps{
		Runner: runner,
		ReclaimStale: func(_ context.Context, _ string, _ []string, beadID string) (bool, string, error) {
			reclaimCalls = append(reclaimCalls, beadID)
			return false, "", nil
		},
		Claim: func(_ context.Context, _ string, _ []string, _, _ string) (beads.Bead, bool, error) {
			claimCalled = true
			return beads.Bead{}, false, nil
		},
		DrainAck: func(io.Writer) error { return nil },
	}

	var stdout, stderr bytes.Buffer
	doHookClaim("query", ".", hookClaimOptions{
		Assignee:               "worker-1",
		RouteTargets:           []string{"worker"},
		AutoReclaimStaleClaims: true,
		JSON:                   true,
	}, ops, &stdout, &stderr)

	if len(reclaimCalls) != 1 || reclaimCalls[0] != "work-1" {
		t.Fatalf("reclaim calls = %v, want exactly one scoped attempt on work-1", reclaimCalls)
	}
	if claimCalled {
		t.Fatal("Claim should not be attempted when nothing was reclaimed (FR4)")
	}
}

// TestDoHookClaimFlagOffNeverAttemptsReclaim covers ga-7rj87d NFR4/NFR5: the
// feature is off by default, and the flag-off path must never call
// ReclaimStale at all (byte-for-byte unchanged behavior/latency).
func TestDoHookClaimFlagOffNeverAttemptsReclaim(t *testing.T) {
	runner := func(string, string) (string, error) {
		return `[{"id":"work-1","assignee":"dead-worker","metadata":{"gc.routed_to":"worker"}}]`, nil
	}
	reclaimCalled := false
	ops := hookClaimOps{
		Runner: runner,
		ReclaimStale: func(_ context.Context, _ string, _ []string, _ string) (bool, string, error) {
			reclaimCalled = true
			return true, "dead-worker", nil
		},
		DrainAck: func(io.Writer) error { return nil },
	}

	var stdout, stderr bytes.Buffer
	doHookClaim("query", ".", hookClaimOptions{
		Assignee:     "worker-1",
		RouteTargets: []string{"worker"},
		JSON:         true,
	}, ops, &stdout, &stderr)

	if reclaimCalled {
		t.Fatal("ReclaimStale must not be called when AutoReclaimStaleClaims is off (NFR4/NFR5)")
	}
}

// TestDoHookClaimEmitsReclaimedStaleEventOnSuccess covers ga-7rj87d FR5: a
// successful reclaim-then-claim must fire the typed
// hook.claim.reclaimed_stale event exactly once, naming the bead, the
// previous (stale) owner, and the new assignee.
func TestDoHookClaimEmitsReclaimedStaleEventOnSuccess(t *testing.T) {
	runner := func(string, string) (string, error) {
		return `[{"id":"work-1","assignee":"dead-worker","metadata":{"gc.routed_to":"worker"}}]`, nil
	}
	var emitted []string
	ops := hookClaimOps{
		Runner: runner,
		ReclaimStale: func(_ context.Context, _ string, _ []string, _ string) (bool, string, error) {
			return true, "dead-worker", nil
		},
		Claim: func(_ context.Context, _ string, _ []string, beadID, assignee string) (beads.Bead, bool, error) {
			return beads.Bead{ID: beadID, Assignee: assignee, Metadata: map[string]string{"gc.routed_to": "worker"}}, true, nil
		},
		EmitHookClaimReclaimedStale: func(beadID, previousOwner, newAssignee string) {
			emitted = append(emitted, beadID+"|"+previousOwner+"|"+newAssignee)
		},
		DrainAck: func(io.Writer) error { return nil },
	}

	var stdout, stderr bytes.Buffer
	doHookClaim("query", ".", hookClaimOptions{
		Assignee:               "worker-1",
		RouteTargets:           []string{"worker"},
		AutoReclaimStaleClaims: true,
		JSON:                   true,
	}, ops, &stdout, &stderr)

	want := "work-1|dead-worker|worker-1"
	if len(emitted) != 1 || emitted[0] != want {
		t.Fatalf("emitted = %v, want exactly [%q] (FR5)", emitted, want)
	}
}

// TestDoHookClaimSkipsBudgetDeferredStaleAssigneeCandidate covers the
// ga-7rj87d round-1 review gap (FR1/NFR5): hookCandidateReclaimEligible
// checked only ID/assignee/route and omitted the budget-deferred check that
// hookCandidateClaimable already applies on the fresh-claim path. That let a
// route-matched candidate with a stale assignee bypass the daily
// build-budget gate via the opt-in reclaim path merely by having an
// assignee. A candidate still inside its gc.budget_deferred_until window
// must not be reclaimed even with AutoReclaimStaleClaims on.
func TestDoHookClaimSkipsBudgetDeferredStaleAssigneeCandidate(t *testing.T) {
	runner := func(string, string) (string, error) {
		return `[{"id":"work-1","assignee":"dead-worker","metadata":{"gc.routed_to":"worker","gc.budget_deferred_until":"2099-01-01T00:00:00Z"}}]`, nil
	}
	reclaimCalled := false
	ops := hookClaimOps{
		Runner: runner,
		ReclaimStale: func(_ context.Context, _ string, _ []string, _ string) (bool, string, error) {
			reclaimCalled = true
			return true, "dead-worker", nil
		},
		DrainAck: func(io.Writer) error { return nil },
	}

	var stdout, stderr bytes.Buffer
	doHookClaim("query", ".", hookClaimOptions{
		Assignee:               "worker-1",
		RouteTargets:           []string{"worker"},
		AutoReclaimStaleClaims: true,
		JSON:                   true,
	}, ops, &stdout, &stderr)

	if reclaimCalled {
		t.Fatal("ReclaimStale must not be called for a candidate still inside its budget-deferred window, even with AutoReclaimStaleClaims on")
	}
}

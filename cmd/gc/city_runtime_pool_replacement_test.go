package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/events"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/testutil"
	"github.com/gastownhall/gascity/internal/worktree"
)

// replacementStart records persisted metadata at the runtime boundary, before
// later ticks could repair the session's directory without moving its process.
type replacementStart struct {
	name    string
	config  runtime.Config
	session beads.Bead
}

type poolReplacementRuntime struct {
	*runtime.Fake
	store          beads.Store
	mu             sync.Mutex
	dead           map[string]bool
	starts         []replacementStart
	failFirstStart bool
}

// Inject one failed session listing at the boundary selected by the test.
type poolReplacementStore struct {
	*beads.MemStore
	failAfterCloseID    string
	failNextSessionList bool
	failed              bool
}

func (s *poolReplacementStore) List(q beads.ListQuery) ([]beads.Bead, error) {
	if q.Type == sessionBeadType && s.failNextSessionList {
		s.failNextSessionList = false
		return nil, errors.New("post-build snapshot unavailable")
	}
	if q.Type == sessionBeadType && s.failAfterCloseID != "" && !s.failed {
		b, err := s.Get(s.failAfterCloseID)
		if err == nil && b.Status == "closed" {
			s.failed = true
			return nil, errors.New("replacement snapshot unavailable")
		}
	}
	return s.MemStore.List(q)
}

func (p *poolReplacementRuntime) IsRunning(name string) bool {
	p.mu.Lock()
	dead := p.dead[name]
	p.mu.Unlock()
	return !dead && p.Fake.IsRunning(name)
}

func (p *poolReplacementRuntime) IsDeadRuntimeSession(name string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.dead[name], nil
}

func (p *poolReplacementRuntime) Start(ctx context.Context, name string, cfg runtime.Config) error {
	b, err := p.store.Get(cfg.Env["GC_SESSION_ID"])
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.starts = append(p.starts, replacementStart{name: name, config: cfg, session: b})
	fail := p.failFirstStart && len(p.starts) == 1
	delete(p.dead, name)
	p.mu.Unlock()
	if fail {
		return errors.New("replacement fixture: provider start failed")
	}
	return p.Fake.Start(ctx, name, cfg)
}

func TestCityRuntimeDemandSnapshotTracksMaterializedSessions(t *testing.T) {
	for _, stage := range []string{"build", "sync"} {
		t.Run(stage, func(t *testing.T) {
			store := beads.NewMemStore()
			cr := &CityRuntime{
				cityPath: t.TempDir(), cityName: "test-city",
				cfg: &config.City{Workspace: config.Workspace{Name: "test-city"}},
				cs:  &controllerState{cityBeadStore: store, eventProv: events.NewFake()},
				sp:  runtime.NewFake(), stderr: io.Discard,
			}
			builds := 0
			cr.buildFnWithSessionBeads = func(*config.City, runtime.Provider, beads.Store, map[string]beads.Store, *sessionBeadSnapshot, *sessionReconcilerTraceCycle) DesiredStateResult {
				builds++
				if stage == "build" {
					if _, err := store.Create(beads.Bead{
						Type: sessionBeadType, Status: "open", Labels: []string{sessionBeadLabel},
						Metadata: map[string]string{"session_name": "worker", "template": "worker", "state": "active"},
					}); err != nil {
						t.Fatal(err)
					}
				}
				return DesiredStateResult{State: map[string]TemplateParams{
					"worker": {SessionName: "worker", TemplateName: "worker", Command: "true"},
				}}
			}
			input := cr.loadSessionBeadSnapshot()
			demand := cr.loadDemandSnapshot(input, nil, "patrol", false)
			if stage == "sync" {
				cr.syncBeadsAndUpdateIndex(demand.result.State, input, nil)
			} else {
				cr.loadDemandSnapshot(cr.loadSessionBeadSnapshot(), nil, "patrol", false)
				if builds != 1 {
					t.Fatalf("unchanged post-build sessions rebuilt demand %d times, want 1", builds)
				}
			}
			infos := cr.loadSessionBeadSnapshot().OpenInfos()
			if len(infos) != 1 {
				t.Fatalf("%s created %d sessions, want 1", stage, len(infos))
			}
			if err := store.Close(infos[0].ID); err != nil {
				t.Fatal(err)
			}
			cr.loadDemandSnapshot(cr.loadSessionBeadSnapshot(), nil, "patrol", false)
			if builds != 2 {
				t.Fatalf("create -> close -> patrol built demand %d times, want 2", builds)
			}
		})
	}
}

func TestCityRuntimeDemandSnapshotDoesNotCachePostBuildReadFailure(t *testing.T) {
	store := &poolReplacementStore{MemStore: beads.NewMemStore()}
	var stderr bytes.Buffer
	cr := &CityRuntime{
		cityName: "test-city",
		cfg:      &config.City{Workspace: config.Workspace{Name: "test-city"}},
		cs:       &controllerState{cityBeadStore: store, eventProv: events.NewFake()},
		stderr:   &stderr,
	}
	builds := 0
	cr.buildFnWithSessionBeads = func(*config.City, runtime.Provider, beads.Store, map[string]beads.Store, *sessionBeadSnapshot, *sessionReconcilerTraceCycle) DesiredStateResult {
		builds++
		store.failNextSessionList = builds == 1
		return DesiredStateResult{State: map[string]TemplateParams{}}
	}
	first := cr.loadDemandSnapshot(cr.loadSessionBeadSnapshot(), nil, "patrol", false)
	if !first.result.SessionQueryPartial || !bytes.Contains(stderr.Bytes(), []byte("post-build snapshot unavailable")) {
		t.Fatalf("post-build read failure must be reported as partial demand; partial=%v stderr=%q", first.result.SessionQueryPartial, stderr.String())
	}
	second := cr.loadDemandSnapshot(cr.loadSessionBeadSnapshot(), nil, "patrol", false)
	if builds != 2 || second.result.SessionQueryPartial {
		t.Fatalf("next patrol must rebuild complete demand: builds=%d partial=%v", builds, second.result.SessionQueryPartial)
	}
	cr.loadDemandSnapshot(cr.loadSessionBeadSnapshot(), nil, "patrol", false)
	if builds != 2 {
		t.Fatalf("stable complete demand must be reused: builds=%d", builds)
	}
}

func TestCityRuntimeTick_PoolReplacementUsesVerifiedTriggerWorktree(t *testing.T) {
	testCityRuntimePoolReplacementUsesVerifiedTriggerWorktree(t, false)
}

func TestCityRuntimeRun_PoolReplacementUsesVerifiedTriggerWorktree(t *testing.T) {
	testCityRuntimePoolReplacementUsesVerifiedTriggerWorktree(t, true)
}

func testCityRuntimePoolReplacementUsesVerifiedTriggerWorktree(t *testing.T, startup bool) {
	t.Helper()
	for _, tc := range []struct {
		name           string
		branch         string
		fresh          bool
		partial        bool
		failFirstStart bool
	}{
		{name: "raw_sha_review", branch: "review/fx-work"},
		{name: "origin_main_codex", branch: "codex/fx-work"},
		{name: "fresh_creation", branch: "codex/fx-work", fresh: true},
		{name: "async_start_rollback", branch: "codex/fx-work", fresh: true, failFirstStart: true},
		{name: "snapshot_read_failure", branch: "review/fx-work", partial: true},
	} {
		// Patrol owns the evidence/error matrix; startup proves the separate
		// run() cleanup-to-planning path with a recent, bound corpse.
		if startup && tc.name != "raw_sha_review" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			repo, _ := worktreeTestRepo(t)
			baseSHA := testutil.RunGit(t, repo, "rev-parse", "HEAD")
			base := baseSHA
			if tc.name != "raw_sha_review" {
				testutil.RunGit(t, repo, "update-ref", "refs/remotes/origin/main", baseSHA)
				base = "origin/main"
			}
			root := t.TempDir()
			tree := filepath.Join(root, "fx-work")
			lobby := filepath.Join(root, "_lobby")
			if err := os.MkdirAll(lobby, 0o755); err != nil {
				t.Fatal(err)
			}
			spec := worktree.Spec{
				RepoDir: repo, Root: root, Path: tree, Branch: tc.branch, Base: base,
				BeadID: "fx-work", StoreRef: "rig:fixture", Creator: "gc-sling",
				Owner: "gc-sling", Generation: "1", Lifecycle: worktree.LifecycleActive,
			}
			cityPath := t.TempDir()
			workStore := newConfiguredAttemptTestStore(t, cityPath, repo)
			work, err := workStore.Create(beads.Bead{
				Type: "task", Status: "open", Title: "routed work",
				Metadata: map[string]string{
					beadmeta.RoutedToMetadataKey:           "fixture/worker",
					beadmeta.WorkDirMetadataKey:            tree,
					beadmeta.WorkBranchMetadataKey:         spec.Branch,
					beadmeta.WorktreeRootMetadataKey:       root,
					beadmeta.WorktreeRepoMetadataKey:       repo,
					beadmeta.WorktreeBaseRefMetadataKey:    base,
					beadmeta.WorktreeBaseSHAMetadataKey:    baseSHA,
					beadmeta.WorktreeCreatorMetadataKey:    spec.Creator,
					beadmeta.WorktreeOwnerMetadataKey:      spec.Owner,
					beadmeta.WorktreeGenerationMetadataKey: spec.Generation,
					beadmeta.WorktreeLifecycleMetadataKey:  spec.Lifecycle,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			spec.BeadID = work.ID
			if _, err := worktree.Ensure(spec); err != nil {
				t.Fatalf("Ensure: %v", err)
			}
			cfg := &config.City{
				Workspace: config.Workspace{Name: "test-city"},
				Rigs:      []config.Rig{{Name: "fixture", Path: repo}},
				Daemon:    config.DaemonConfig{ShutdownTimeout: "30s"},
				Agents: []config.Agent{{
					Name: "worker", Dir: "fixture", StartCommand: "true",
					WorkDir: lobby, WorkDirRoots: []string{root},
					MinActiveSessions: intPtr(0), MaxActiveSessions: intPtr(2),
				}},
			}
			store := &poolReplacementStore{MemStore: beads.NewMemStoreFrom(100, nil, nil)}
			sp := &poolReplacementRuntime{
				Fake: runtime.NewFake(), store: store, dead: make(map[string]bool),
				failFirstStart: tc.failFirstStart,
			}
			oldID := ""
			if !tc.fresh {
				name := "fixture__worker-1"
				old, err := store.Create(beads.Bead{
					Type: sessionBeadType, Status: "open", Title: "fixture/worker-1",
					Labels: []string{sessionBeadLabel, "agent:fixture/worker-1"},
					Metadata: map[string]string{
						"session_name": name, "template": "fixture/worker",
						"agent_name": "fixture/worker-1", "pool_slot": "1",
						"state": "active", "generation": "1",
						poolManagedMetadataKey:                  "true",
						beadmeta.TriggerBeadIDMetadataKey:       work.ID,
						beadmeta.TriggerBeadStoreRefMetadataKey: "rig:fixture",
						beadmeta.WorkDirMetadataKey:             tree,
						beadmeta.LegacyWorkDirMetadataKey:       tree,
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				oldID = old.ID
				if err := sp.Fake.Start(context.Background(), name, runtime.Config{}); err != nil {
					t.Fatal(err)
				}
				sp.dead[name] = true
			}
			var stderr bytes.Buffer
			cr := &CityRuntime{
				cityPath: cityPath, cityName: "test-city", cfg: cfg, sp: sp,
				cs: &controllerState{
					cityPath: cityPath, cityName: "test-city", cfg: cfg,
					cityBeadStore: store, beadStores: map[string]beads.Store{"fixture": workStore},
					eventProv: events.NewFake(),
				},
				sessionDrains: newDrainTracker(), wg: fixedWispGC{},
				rec: events.Discard, stdout: io.Discard, stderr: &stderr,
				managedDoltOwned: func(string) (bool, error) { return false, nil },
			}
			cr.buildFnWithSessionBeads = func(c *config.City, currentSP runtime.Provider, s beads.Store, rigs map[string]beads.Store, snapshot *sessionBeadSnapshot, trace *sessionReconcilerTraceCycle) DesiredStateResult {
				return buildDesiredStateWithSessionBeads("test-city", cityPath, time.Now(), c, currentSP, s, rigs, snapshot, trace, &stderr)
			}
			if !tc.fresh && !startup {
				// Cache the plan containing the protected, recently created corpse.
				cr.loadDemandSnapshot(cr.loadSessionBeadSnapshot(), nil, "patrol", false)
				snapshot := cr.loadSessionBeadSnapshot()
				cr.loadDemandSnapshot(snapshot, nil, "patrol", false)
				if cr.shouldRefreshDemandSnapshot("patrol", false, sessionBeadSnapshotFingerprint(snapshot)) {
					t.Fatal("fixture must have reusable cached demand before corpse cleanup")
				}
			}
			var dirty atomic.Bool
			var provider string
			var prevRunning map[string]bool
			settleStarts := func() {
				t.Helper()
				done := make(chan struct{})
				go func() {
					cr.asyncStarts.wg.Wait()
					close(done)
				}()
				select {
				case <-done:
				case <-time.After(30 * time.Second):
					t.Fatal("async starts did not settle")
				}
			}
			if tc.partial {
				store.failAfterCloseID = oldID
				cr.tick(context.Background(), &dirty, &provider, cityPath, &prevRunning, "patrol")
				settleStarts()
				if !store.failed || !cr.demandSnapshot.result.SessionQueryPartial {
					t.Errorf("failed post-cleanup snapshot was not carried as partial demand")
				}
				if len(sp.starts) != 0 {
					t.Fatalf("incomplete snapshot launched %d replacements; stderr:\n%s", len(sp.starts), stderr.String())
				}
			}
			passes := 2
			if tc.name == "fresh_creation" {
				// First drain-ack finalization, then corpse cleanup: keep the
				// original repeated-replacement proof as well.
				passes = 3
			}
			if startup {
				passes = 1
			}
			for tick := 0; tick < passes; tick++ {
				if startup {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					// Wait for the first startup launch before cancellation,
					// and stop before any patrol can repair its binding.
					startsDrained := false
					cr.onStarted = func() {
						startsDrained = cr.waitForAsyncStarts()
						cancel()
					}
					cr.preserveSessionsOnShutdown()
					done := make(chan struct{})
					go func() {
						cr.run(ctx)
						close(done)
					}()
					awaitClose(t, done, "startup reconciliation and first runtime Start")
					if !startsDrained {
						t.Fatalf("startup did not finish its runtime starts; stderr:\n%s", stderr.String())
					}
				} else {
					cr.tick(context.Background(), &dirty, &provider, cityPath, &prevRunning, "patrol")
				}
				settleStarts()
				sp.mu.Lock()
				starts := append([]replacementStart(nil), sp.starts...)
				sp.mu.Unlock()
				if len(starts) != tick+1 {
					t.Fatalf("tick %d: starts = %d, want %d; stderr:\n%s", tick, len(starts), tick+1, stderr.String())
				}
				start := starts[tick]
				if start.session.ID == oldID {
					t.Errorf("tick %d: reused closed session %s", tick, oldID)
				}
				for key, want := range map[string]string{
					beadmeta.TriggerBeadIDMetadataKey: work.ID,
					beadmeta.WorkDirMetadataKey:       tree,
					beadmeta.LegacyWorkDirMetadataKey: tree,
				} {
					if got := start.session.Metadata[key]; got != want {
						t.Errorf("tick %d: at Start, %s = %q, want %q", tick, key, got, want)
					}
				}
				if start.config.WorkDir != tree || start.config.Env["GC_DIR"] != tree {
					t.Errorf("tick %d: runtime WorkDir = %q, GC_DIR = %q, want %q", tick, start.config.WorkDir, start.config.Env["GC_DIR"], tree)
				}
				if oldID != "" {
					old, err := store.Get(oldID)
					if err != nil || old.Status != "closed" {
						t.Errorf("tick %d: corpse status = %q, err = %v", tick, old.Status, err)
					}
				}
				current, err := store.Get(start.session.ID)
				if err != nil {
					t.Fatal(err)
				}
				for _, key := range []string{beadmeta.WorkDirMetadataKey, beadmeta.LegacyWorkDirMetadataKey} {
					if current.Metadata[key] != start.session.Metadata[key] {
						t.Errorf("tick %d: %s corrected after launch", tick, key)
					}
				}
				oldID = start.session.ID
				if tick == passes-1 {
					continue
				}
				if tc.fresh && tick == 0 {
					task, err := workStore.Get(work.ID)
					if err != nil || task.Status != "open" || task.Assignee != "" {
						t.Fatalf("work must remain open and unclaimed: status=%q assignee=%q err=%v", task.Status, task.Assignee, err)
					}
					if tc.failFirstStart {
						if current.Status != "closed" || current.Metadata["state"] != "failed-create" || sp.IsRunning(start.name) {
							t.Fatalf("async start must roll back before patrol: status=%q state=%q running=%v; stderr:\n%s",
								current.Status, current.Metadata["state"], sp.IsRunning(start.name), stderr.String())
						}
					} else {
						if !sp.IsRunning(start.name) {
							t.Fatal("first start must succeed before drain-ack finalization")
						}
						if err := store.SetMetadataBatch(oldID, map[string]string{
							"state": "draining", "state_reason": "drain-ack-stop-pending",
						}); err != nil {
							t.Fatal(err)
						}
						if err := sp.Stop(start.name); err != nil {
							t.Fatal(err)
						}
					}
					// No dead runtime artifact for corpse cleanup, no work
					// changes, and no poke before the replacement patrol.
					continue
				}
				sp.mu.Lock()
				sp.dead[start.name] = true
				sp.mu.Unlock()
			}
		})
	}
}

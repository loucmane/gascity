package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/taskattempt"
	"github.com/gastownhall/gascity/internal/worker"
)

func TestTaskAttemptRejectsUnregisteredStoreRefs(t *testing.T) {
	cityPath := t.TempDir()
	cfg := &config.City{Workspace: config.Workspace{Name: "fixture"}, Rigs: []config.Rig{
		{Name: "frontend", Path: "frontend"}, {Name: "empty"},
	}}
	for _, ref := range []string{"city:other", "city:", "city:fixture ", "rig:", "rig:missing", "rig:frontend ", "rig:../frontend", "rig:empty", "frontend"} {
		t.Run(ref, func(t *testing.T) {
			err := attemptWorkStoreAccess(cityPath, cfg)(ref, "", func(beads.Store) error {
				t.Fatal("invalid store reference reached callback")
				return nil
			})
			if !errors.Is(err, taskattempt.ErrRefused) {
				t.Fatalf("%q: %v", ref, err)
			}
		})
	}
	if err := attemptWorkStoreAccess(cityPath, nil)("rig:frontend", "", func(beads.Store) error {
		t.Fatal("rig without configuration reached callback")
		return nil
	}); !errors.Is(err, taskattempt.ErrRefused) {
		t.Fatalf("missing configuration: %v", err)
	}
}

func TestTaskAttemptResolvesRegisteredRigPaths(t *testing.T) {
	for _, absolute := range []bool{false, true} {
		t.Run(map[bool]string{false: "relative", true: "absolute"}[absolute], func(t *testing.T) {
			t.Setenv("GC_HOME", t.TempDir())
			cityPath := t.TempDir()
			rigPath := filepath.Join(cityPath, "frontend")
			if err := os.MkdirAll(rigPath, 0o700); err != nil {
				t.Fatal(err)
			}
			s := newConfiguredAttemptTestStore(t, cityPath, rigPath)
			b, err := s.Create(beads.Bead{Title: "registered work authority"})
			if err != nil {
				t.Fatal(err)
			}
			registeredPath := "frontend"
			if absolute {
				registeredPath = rigPath
			}
			cfg := &config.City{Workspace: config.Workspace{Name: "fixture"}, Rigs: []config.Rig{
				{Name: "frontend", Path: registeredPath},
			}}
			if err := attemptWorkStoreAccess(cityPath, cfg)("rig:frontend", "", func(s beads.Store) error {
				got, err := s.Get(b.ID)
				if err == nil && got.Title != b.Title {
					return errors.New("opened wrong work authority")
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Routed-demand fixtures must publish their task authority, rather than only
// supplying a session-store fake: admission now re-opens that authority.
func newConfiguredAttemptTestStore(t *testing.T, cityPath, scopeRoot string) *beads.FileStore {
	t.Helper()
	t.Setenv("GC_BEADS", "file")
	if err := ensureScopedFileStoreLayout(cityPath); err != nil {
		t.Fatal(err)
	}
	s, err := openScopeLocalFileStore(scopeRoot)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPoolTaskAttemptReservationSurvivesNewBuild(t *testing.T) {
	s := beads.NewMemStore()
	b, err := s.Create(beads.Bead{Title: "bounded synthetic demand"})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := taskattempt.Request("rig:fixture", b.ID, "fixture/worker")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMetadata(b.ID, beadmeta.NativeAttemptMetadataKey, policy); err != nil {
		t.Fatal(err)
	}
	meta := map[string]string{beadmeta.TriggerBeadIDMetadataKey: b.ID, beadmeta.TriggerBeadStoreRefMetadataKey: "rig:fixture"}
	makeBP := func() *agentBuildParams {
		return &agentBuildParams{attemptWorkStore: func(ref, id string, use func(beads.Store) error) error {
			if ref != "rig:fixture" {
				t.Fatalf("unexpected store %q", ref)
			}
			return use(s)
		}}
	}
	if token, err := reservePoolTaskAttempt(makeBP(), "fixture/worker", meta); err != nil || token == "" {
		t.Fatalf("initial: %q %v", token, err)
	}
	for range 10 {
		if _, err := reservePoolTaskAttempt(makeBP(), "fixture/worker", meta); !errors.Is(err, taskattempt.ErrRefused) {
			t.Fatalf("new scheduler build: %v", err)
		}
	}
}

func TestPoolTaskAttemptUsesClassBindingBeforeRetainedWorkCopy(t *testing.T) {
	cityPath := t.TempDir()
	work, binding := beads.NewMemStore(), beads.NewMemStore()
	work.HonorExplicitIDs, binding.HonorExplicitIDs = true, true
	routes := splitRoutes(binding)
	registerResidencyRoutes(cityPath, routes, func() beads.Store { return work })
	t.Cleanup(func() { unregisterResidencyRoutes(cityPath, routes) })
	const id = "gc-bounded-control"
	const template = "core.control-dispatcher"
	policy, err := taskattempt.Request("city:test", id, template)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := binding.Create(beads.Bead{ID: id, Title: "bounded class task", Metadata: map[string]string{
		beadmeta.NativeAttemptMetadataKey: policy,
		beadmeta.KindMetadataKey:          beadmeta.KindWorkflowFinalize,
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := work.Create(beads.Bead{ID: id, Title: "unbounded retained copy"}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.City{Workspace: config.Workspace{Name: "test"}}
	meta := map[string]string{beadmeta.TriggerBeadIDMetadataKey: id, beadmeta.TriggerBeadStoreRefMetadataKey: "city:test"}
	bp := &agentBuildParams{attemptWorkStore: attemptWorkStoreAccess(cityPath, cfg)}
	token, err := reservePoolTaskAttempt(bp, template, meta)
	if err != nil || token == "" {
		t.Fatalf("class reservation: %q %v", token, err)
	}
	bp = &agentBuildParams{attemptWorkStore: attemptWorkStoreAccess(cityPath, cfg)}
	if _, err := reservePoolTaskAttempt(bp, template, meta); !errors.Is(err, taskattempt.ErrRefused) {
		t.Fatalf("second build must retain the bounded reservation: %v", err)
	}
	start := func(s beads.Store) error { return taskattempt.Start(s, id, "city:test", template, token, "session-1") }
	if err := bp.attemptWorkStore("city:test", id, start); err != nil {
		t.Fatal(err)
	}
	if err := bp.attemptWorkStore("city:test", id, start); !errors.Is(err, taskattempt.ErrRefused) {
		t.Fatalf("class attempt started twice: %v", err)
	}
	decoy, err := work.Get(id)
	if err != nil || len(decoy.Metadata) != 0 {
		t.Fatalf("retained work copy was changed: %+v %v", decoy, err)
	}
}

func TestPoolTaskAttemptUsesConfiguredCityAuthorityNotSessionDecoy(t *testing.T) {
	t.Setenv("GC_BEADS", "file")
	cityPath := t.TempDir()
	tasks, err := openScopeLocalFileStore(cityPath)
	if err != nil {
		t.Fatal(err)
	}
	task, err := tasks.Create(beads.Bead{Title: "bounded city work"})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := taskattempt.Request("city:test", task.ID, "worker")
	if err != nil {
		t.Fatal(err)
	}
	if err := tasks.SetMetadata(task.ID, beadmeta.NativeAttemptMetadataKey, policy); err != nil {
		t.Fatal(err)
	}
	sessions := beads.NewMemStore()
	decoy, err := sessions.Create(beads.Bead{ID: task.ID, Title: "unbounded same-ID decoy"})
	if err != nil || decoy.ID != task.ID {
		t.Fatalf("decoy: %s %v", decoy.ID, err)
	}
	cfg := &config.City{Workspace: config.Workspace{Name: "test"}}
	sp := runtime.NewFake()
	bp := newAgentBuildParams("test", cityPath, cfg, sp, time.Now().UTC(), sessions, io.Discard)
	meta := map[string]string{beadmeta.TriggerBeadIDMetadataKey: task.ID, beadmeta.TriggerBeadStoreRefMetadataKey: "city:test"}
	token, err := reservePoolTaskAttempt(bp, "worker", meta)
	if err != nil || token == "" {
		t.Fatalf("reserve authoritative task: %q %v", token, err)
	}
	// A newly constructed scheduler cannot refund the on-disk reservation.
	bp = newAgentBuildParams("test", cityPath, cfg, sp, time.Now().UTC(), sessions, io.Discard)
	if _, err := reservePoolTaskAttempt(bp, "worker", meta); !errors.Is(err, taskattempt.ErrRefused) {
		t.Fatalf("reopened reservation: %v", err)
	}
	meta["template"], meta["session_name"], meta["state"] = "worker", "bounded-cli", "creating"
	meta["instance_token"], meta["generation"], meta["command"] = "fixture", "1", "true"
	// Deliberately omit the reserved token: the same-ID unbounded decoy must
	// not cause the launch boundary to take its legacy/unbounded path.
	b, err := sessions.Create(beads.Bead{Type: "session", Title: "bounded", Metadata: meta})
	if err != nil {
		t.Fatal(err)
	}
	factory, err := workerFactoryWithConfig(cityPath, sessions, sp, cfg)
	if err != nil {
		t.Fatal(err)
	}
	h, err := factory.Session(worker.SessionSpec{ID: b.ID, Template: "worker", Command: "true"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.StartResolved(context.Background(), "true", runtime.Config{Command: "true"}); !errors.Is(err, taskattempt.ErrRefused) {
		t.Fatalf("missing token: %v", err)
	}
	if sp.CountCalls("Start", "bounded-cli") != 0 {
		t.Fatal("session decoy bypassed authority")
	}
	if err := sessions.SetMetadata(b.ID, beadmeta.NativeAttemptTokenMetadataKey, token); err != nil {
		t.Fatal(err)
	}
	sp.StartErrors["bounded-cli"] = errors.New("fixture failed startup")
	if err := h.StartResolved(context.Background(), "true", runtime.Config{Command: "true"}); err == nil {
		t.Fatal("missing fixture failure")
	}
	if got := sp.CountCalls("Start", "bounded-cli"); got != 1 {
		t.Fatalf("first calls: %d", got)
	}
	if err := h.StartResolved(context.Background(), "true", runtime.Config{Command: "true"}); !errors.Is(err, taskattempt.ErrRefused) {
		t.Fatalf("second start: %v", err)
	}
	if sp.CountCalls("Start", "bounded-cli") != 1 {
		t.Fatal("duplicate provider start")
	}
}

func TestPoolTaskAttemptConsumedBeforeCreateAndCopiedToSession(t *testing.T) {
	s := beads.NewMemStore()
	b, err := s.Create(beads.Bead{Title: "bounded demand"})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := taskattempt.Request("city:test", b.ID, "worker")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMetadata(b.ID, beadmeta.NativeAttemptMetadataKey, policy); err != nil {
		t.Fatal(err)
	}
	agent := config.Agent{Name: "worker", Provider: "test", MaxActiveSessions: intPtr(1)}
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test"}, Agents: []config.Agent{agent},
		Providers: map[string]config.ProviderSpec{"test": {Command: "true"}},
	}
	bp := newAgentBuildParams("test", t.TempDir(), cfg, runtime.NewFake(), time.Now().UTC(), s, io.Discard)
	bp.attemptWorkStore = func(ref, id string, use func(beads.Store) error) error {
		if ref != "city:test" {
			t.Fatalf("unexpected store %q", ref)
		}
		return use(s)
	}
	plan := poolSessionCreatePlan{qualifiedInstance: "worker-1", slot: 1, metadata: map[string]string{
		beadmeta.TriggerBeadIDMetadataKey: b.ID, beadmeta.TriggerBeadStoreRefMetadataKey: "city:test",
	}}
	info, err := executePlannedPoolSessionBeadCreate(bp, &agent, "worker", plan)
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.Get(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if session.Metadata[beadmeta.NativeAttemptTokenMetadataKey] == "" {
		t.Fatal("reservation token not persisted on session")
	}
	if _, err := executePlannedPoolSessionBeadCreate(bp, &agent, "worker", plan); !errors.Is(err, taskattempt.ErrRefused) {
		t.Fatalf("second create: %v", err)
	}
}

type attemptCreateFailureStore struct {
	beads.Store
	calls int
}

func (s *attemptCreateFailureStore) Create(beads.Bead) (beads.Bead, error) {
	s.calls++
	return beads.Bead{}, errors.New("fixture session creation failed")
}

func TestPoolTaskAttemptFailedCreateNeverRefunds(t *testing.T) {
	tasks := beads.NewMemStore()
	task, err := tasks.Create(beads.Bead{Title: "bounded work"})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := taskattempt.Request("city:test", task.ID, "worker")
	if err != nil {
		t.Fatal(err)
	}
	if err := tasks.SetMetadata(task.ID, beadmeta.NativeAttemptMetadataKey, policy); err != nil {
		t.Fatal(err)
	}
	sessions := &attemptCreateFailureStore{Store: beads.NewMemStore()}
	agent := config.Agent{Name: "worker", Provider: "test", MaxActiveSessions: intPtr(1)}
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test"}, Agents: []config.Agent{agent},
		Providers: map[string]config.ProviderSpec{"test": {Command: "true"}},
	}
	bp := newAgentBuildParams("test", t.TempDir(), cfg, runtime.NewFake(), time.Now().UTC(), sessions, io.Discard)
	bp.attemptWorkStore = func(ref, id string, use func(beads.Store) error) error {
		if ref != "city:test" {
			t.Fatalf("unexpected store %q", ref)
		}
		return use(tasks)
	}
	plan := poolSessionCreatePlan{qualifiedInstance: "worker-1", slot: 1, metadata: map[string]string{
		beadmeta.TriggerBeadIDMetadataKey: task.ID, beadmeta.TriggerBeadStoreRefMetadataKey: "city:test",
	}}
	if _, err := executePlannedPoolSessionBeadCreate(bp, &agent, "worker", plan); err == nil {
		t.Fatal("creation failure not propagated")
	}
	if sessions.calls != 1 {
		t.Fatalf("create calls: %d, want 1", sessions.calls)
	}
	if _, err := executePlannedPoolSessionBeadCreate(bp, &agent, "worker", plan); !errors.Is(err, taskattempt.ErrRefused) {
		t.Fatalf("reservation refunded: %v", err)
	}
	if sessions.calls != 1 {
		t.Fatal("failed create was retried")
	}
}

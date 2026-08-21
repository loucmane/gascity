package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/events"
	"github.com/gastownhall/gascity/internal/runtime"
)

func TestResolveSessionTemplate_QualifiedGenericRigScope(t *testing.T) {
	rigPath := filepath.Join(t.TempDir(), "blog")
	cfg := &config.City{
		Rigs: []config.Rig{{Name: "blog", Path: rigPath}},
		Agents: []config.Agent{{
			Name:  "builder",
			Scope: "rig",
		}},
	}

	got, ok := resolveSessionTemplate(cfg, "blog/builder", "")
	if !ok {
		t.Fatal("resolveSessionTemplate(blog/builder) failed for generic scope=rig template")
	}
	if got.QualifiedName() != "blog/builder" {
		t.Fatalf("QualifiedName() = %q, want blog/builder", got.QualifiedName())
	}
	if got.Dir != "blog" {
		t.Fatalf("Dir = %q, want blog", got.Dir)
	}

	workDir, err := resolveWorkDir(t.TempDir(), cfg, &got)
	if err != nil {
		t.Fatalf("resolveWorkDir: %v", err)
	}
	if workDir != rigPath {
		t.Fatalf("workDir = %q, want registered rig root %q", workDir, rigPath)
	}
}

func TestBuildDesiredState_GenericRigScopedPoolRecoversRoutedUnassigned(t *testing.T) {
	cityPath := t.TempDir()
	blogPath := filepath.Join(cityPath, "rigs", "blog")
	gasCityPath := filepath.Join(cityPath, "rigs", "gascity")
	for _, dir := range []string{blogPath, gasCityPath} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	minSessions, maxSessions := 0, 2
	cfg := &config.City{
		Workspace: config.Workspace{Name: "city"},
		Rigs: []config.Rig{
			{Name: "blog", Path: blogPath},
			{Name: "gascity", Path: gasCityPath},
		},
		Agents: []config.Agent{{
			Name:              "builder",
			Scope:             "rig",
			Provider:          "mock",
			MinActiveSessions: &minSessions,
			MaxActiveSessions: &maxSessions,
		}},
		Providers: map[string]config.ProviderSpec{"mock": {Command: "true"}},
	}
	cityStore := beads.NewMemStore()
	blogStore := beads.NewMemStore()
	gasCityStore := beads.NewMemStore()
	if _, err := blogStore.Create(beads.Bead{
		ID:       "blog-zcz",
		Title:    "Fullbleed implementation",
		Type:     "task",
		Status:   "open",
		Metadata: map[string]string{"gc.routed_to": "blog/builder"},
	}); err != nil {
		t.Fatalf("create routed bead: %v", err)
	}

	result := buildDesiredStateWithSessionBeads(
		"city",
		cityPath,
		time.Now().UTC(),
		cfg,
		runtime.NewFake(),
		cityStore,
		map[string]beads.Store{"blog": blogStore, "gascity": gasCityStore},
		&sessionBeadSnapshot{},
		nil,
		io.Discard,
	)

	if got := result.ScaleCheckCounts["blog/builder"]; got != 1 {
		t.Fatalf("ScaleCheckCounts[blog/builder] = %d, want 1; all=%v", got, result.ScaleCheckCounts)
	}
	if got := result.ScaleCheckCounts["gascity/builder"]; got != 0 {
		t.Fatalf("ScaleCheckCounts[gascity/builder] = %d, want 0; all=%v", got, result.ScaleCheckCounts)
	}
	if len(result.State) != 1 {
		t.Fatalf("desired sessions = %d, want 1; state=%v", len(result.State), result.State)
	}
	for _, params := range result.State {
		if params.TemplateName != "blog/builder" {
			t.Fatalf("desired TemplateName = %q, want blog/builder", params.TemplateName)
		}
		if params.WorkDir != blogPath {
			t.Fatalf("desired WorkDir = %q, want %q", params.WorkDir, blogPath)
		}
	}
}

func TestGenericRigScopedPool_RemainsAwakeFromRoutedDemandThroughClaim(t *testing.T) {
	cityPath := t.TempDir()
	blogPath := filepath.Join(cityPath, "rigs", "blog")
	if err := os.MkdirAll(blogPath, 0o755); err != nil {
		t.Fatalf("mkdir blog rig: %v", err)
	}
	minSessions, maxSessions := 0, 1
	cfg := &config.City{
		Workspace: config.Workspace{Name: "city"},
		Rigs:      []config.Rig{{Name: "blog", Path: blogPath}},
		Agents: []config.Agent{
			{
				Name:              "builder",
				Scope:             "rig",
				Provider:          "mock",
				MinActiveSessions: &minSessions,
				MaxActiveSessions: &maxSessions,
			},
			{
				Name:              "builder",
				Dir:               "gas-city-template",
				Provider:          "mock",
				MinActiveSessions: &minSessions,
				MaxActiveSessions: &maxSessions,
			},
		},
		Providers: map[string]config.ProviderSpec{"mock": {Command: "true"}},
	}
	cityStore := beads.NewMemStore()
	blogStore := beads.NewMemStore()
	work, err := blogStore.Create(beads.Bead{
		ID:       "blog-zcz",
		Title:    "Fullbleed implementation",
		Type:     "task",
		Status:   "open",
		Metadata: map[string]string{"gc.routed_to": "blog/builder"},
	})
	if err != nil {
		t.Fatalf("create routed work: %v", err)
	}

	sp := runtime.NewFake()
	result := buildDesiredStateWithSessionBeads(
		"city",
		cityPath,
		time.Now().UTC(),
		cfg,
		sp,
		cityStore,
		map[string]beads.Store{"blog": blogStore},
		&sessionBeadSnapshot{},
		nil,
		io.Discard,
	)
	if len(result.State) != 1 {
		t.Fatalf("desired sessions = %d, want 1; state=%v", len(result.State), result.State)
	}
	var sessionName string
	for name := range result.State {
		sessionName = name
	}
	if got := result.ScaleCheckCounts["blog/builder"]; got != 1 {
		t.Fatalf("ScaleCheckCounts[blog/builder] = %d, want 1", got)
	}
	result.PoolDesiredCounts = map[string]int{"blog/builder": 1}

	sessionBead, err := cityStore.Create(beads.Bead{
		Title:  "blog/builder pool slot",
		Type:   sessionBeadType,
		Status: "open",
		Labels: []string{sessionBeadLabel, "agent:blog/builder"},
		Metadata: map[string]string{
			"session_name":         sessionName,
			"template":             "blog/builder",
			"agent_name":           "blog/builder",
			"pool_slot":            "1",
			poolManagedMetadataKey: boolMetadata(true),
			"state":                "awake",
			"continuation_epoch":   "1",
			"generation":           "1",
		},
	})
	if err != nil {
		t.Fatalf("create pool session: %v", err)
	}
	if err := sp.Start(context.Background(), sessionName, runtime.Config{}); err != nil {
		t.Fatalf("start pool runtime: %v", err)
	}
	snapshot := newSessionBeadSnapshot([]beads.Bead{sessionBead})
	result = buildDesiredStateWithSessionBeads(
		"city",
		cityPath,
		time.Now().UTC(),
		cfg,
		sp,
		cityStore,
		map[string]beads.Store{"blog": blogStore},
		snapshot,
		nil,
		io.Discard,
	)
	result.PoolDesiredCounts = map[string]int{"blog/builder": 1}

	cr := &CityRuntime{
		cityPath:            cityPath,
		cityName:            "city",
		cfg:                 cfg,
		sp:                  sp,
		standaloneCityStore: cityStore,
		standaloneRigStores: map[string]beads.Store{"blog": blogStore},
		sessionDrains:       newDrainTracker(),
		rec:                 events.Discard,
		stdout:              io.Discard,
		stderr:              io.Discard,
	}

	// The routed bead has not been claimed yet. Its demand created the slot, so
	// the exact same expanded config must keep that slot awake on later ticks.
	cr.beadReconcileTick(context.Background(), result, snapshot, nil, false)
	if drain := cr.sessionDrains.get(sessionBead.ID); drain != nil {
		t.Fatalf("routed-unassigned pool slot entered drain before claim: reason=%s", drain.reason)
	}
	if !sp.IsRunning(sessionName) {
		t.Fatal("routed-unassigned pool slot stopped before claim")
	}

	// Claim the existing routed work and rebuild from the same source config.
	// The transition must not create a second slot or start post-claim churn.
	inProgress, assignee := "in_progress", sessionName
	if err := blogStore.Update(work.ID, beads.UpdateOpts{Status: &inProgress, Assignee: &assignee}); err != nil {
		t.Fatalf("claim routed work: %v", err)
	}
	claimedResult := buildDesiredStateWithSessionBeads(
		"city",
		cityPath,
		time.Now().UTC(),
		cfg,
		sp,
		cityStore,
		map[string]beads.Store{"blog": blogStore},
		snapshot,
		nil,
		io.Discard,
	)
	claimedResult.PoolDesiredCounts = map[string]int{"blog/builder": 1}
	cr.beadReconcileTick(context.Background(), claimedResult, snapshot, nil, false)
	if drain := cr.sessionDrains.get(sessionBead.ID); drain != nil {
		t.Fatalf("claimed pool slot entered post-claim drain: reason=%s", drain.reason)
	}
	if !sp.IsRunning(sessionName) {
		t.Fatal("claimed pool slot stopped after claim")
	}
	if len(claimedResult.State) != 1 {
		t.Fatalf("post-claim desired sessions = %d, want 1", len(claimedResult.State))
	}
}

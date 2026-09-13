package session

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/taskattempt"
)

func boundedSessionFixture(t *testing.T) (*Manager, *beads.MemStore, *runtime.Fake, string) {
	t.Helper()
	tasks, sessions := beads.NewMemStore(), beads.NewMemStore()
	task, err := tasks.Create(beads.Bead{Title: "bounded task"})
	if err != nil {
		t.Fatal(err)
	}
	value, err := taskattempt.Request("rig:fixture", task.ID, "fixture/worker")
	if err != nil {
		t.Fatal(err)
	}
	if err := tasks.SetMetadata(task.ID, beadmeta.NativeAttemptMetadataKey, value); err != nil {
		t.Fatal(err)
	}
	token, err := taskattempt.Reserve(tasks, task.ID, "rig:fixture", "fixture/worker")
	if err != nil {
		t.Fatal(err)
	}
	b, err := sessions.Create(beads.Bead{Type: "session", Title: "bounded session", Metadata: map[string]string{
		"template": "fixture/worker", "session_name": "bounded", "state": "creating", "instance_token": "fixture-instance", "generation": "1",
		"session_key": "old-key", "resume_flag": "--resume", "command": "claude --resume old-key",
		beadmeta.TriggerBeadIDMetadataKey: task.ID, beadmeta.TriggerBeadStoreRefMetadataKey: "rig:fixture", beadmeta.NativeAttemptTokenMetadataKey: token,
	}})
	if err != nil {
		t.Fatal(err)
	}
	sp := runtime.NewFake()
	m := NewManagerWithOptions(sessions, sp, WithAttemptWorkStoreAccess(func(ref string, use func(beads.Store) error) error {
		if ref != "rig:fixture" {
			return errors.New("unknown fixture store")
		}
		return use(tasks)
	}))
	return m, sessions, sp, b.ID
}

func TestNativeAttemptStartBoundaryRefusesSecondProviderCall(t *testing.T) {
	m, _, sp, id := boundedSessionFixture(t)
	sp.StartErrors["bounded"] = errors.New("fixture startup failure")
	if err := m.startRuntime(context.Background(), id, "bounded", runtime.Config{}); err == nil {
		t.Fatal("missing provider failure")
	}
	if err := m.startRuntime(context.Background(), id, "bounded", runtime.Config{}); !errors.Is(err, taskattempt.ErrRefused) {
		t.Fatalf("second start: %v", err)
	}
	if got := sp.CountCalls("Start", "bounded"); got != 1 {
		t.Fatalf("provider calls: %d", got)
	}
}

func TestNativeAttemptStartBoundaryRechecksCurrentSession(t *testing.T) {
	for _, mutation := range []struct{ key, value string }{
		{beadmeta.NativeAttemptTokenMetadataKey, ""},
		{beadmeta.NativeAttemptTokenMetadataKey, "wrong"},
		{beadmeta.TriggerBeadIDMetadataKey, ""},
		{beadmeta.TriggerBeadIDMetadataKey, "different-task"},
		{beadmeta.TriggerBeadStoreRefMetadataKey, "rig:other"},
		{beadmeta.TriggerBeadStoreRefMetadataKey, ""},
		{"template", "fixture/other"},
	} {
		t.Run(mutation.key+"="+mutation.value, func(t *testing.T) {
			m, s, sp, id := boundedSessionFixture(t)
			if err := s.SetMetadata(id, mutation.key, mutation.value); err != nil {
				t.Fatal(err)
			}
			if err := m.startRuntime(context.Background(), id, "bounded", runtime.Config{}); err == nil {
				t.Fatal("mutated identity accepted")
			}
			if got := sp.CountCalls("Start", "bounded"); got != 0 {
				t.Fatalf("provider calls: %d", got)
			}
		})
	}
}

func TestNativeAttemptStaleKeyFallbackCannotStartAgain(t *testing.T) {
	m, _, sp, id := boundedSessionFixture(t)
	sp.StartErrors["bounded"] = runtime.ErrSessionDiedDuringStartup
	err := m.StartRuntimeOnly(context.Background(), id, "claude --resume old-key", runtime.Config{Command: "claude --resume old-key"})
	if !errors.Is(err, taskattempt.ErrRefused) {
		t.Fatalf("stale-key fallback: %v", err)
	}
	if got := sp.CountCalls("Start", "bounded"); got != 1 {
		t.Fatalf("provider calls: %d", got)
	}
}

func TestNativeAttemptMissingStoreAndCancellationRefuse(t *testing.T) {
	m, _, sp, id := boundedSessionFixture(t)
	m.attemptWorkStore = nil
	if err := m.startRuntime(context.Background(), id, "bounded", runtime.Config{}); err == nil {
		t.Fatal("missing resolver accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.startRuntime(ctx, id, "bounded", runtime.Config{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if got := sp.CountCalls("Start", "bounded"); got != 0 {
		t.Fatalf("provider calls: %d", got)
	}
}

func TestNativeAttemptProviderStartCallsStayInsideBoundary(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		n := strings.Count(string(data), "m.sp.Start(")
		if n > 0 && name != "task_attempt.go" {
			t.Errorf("%s bypasses native task-attempt admission", name)
		}
		calls += n
	}
	if calls != 2 {
		t.Fatalf("provider-start sites = %d, want two branches of the single boundary", calls)
	}
}

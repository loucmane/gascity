package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/runtime"
)

type fenceProjectionFixture struct {
	SchemaVersion       int    `json:"schema_version"`
	SessionID           string `json:"session_id"`
	InstanceTokenSHA256 string `json:"instance_token_sha256"`
	Generation          int    `json:"generation"`
	State               string `json:"state"`
	ProjectedAt         string `json:"projected_at"`
}

func fenceProjectionFixturePath(cityPath, sessionID string) string {
	return filepath.Join(cityPath, ".gc", "runtime", "session-fence", sessionID+".json")
}

func readFenceProjectionFixture(t *testing.T, cityPath, sessionID string) (fenceProjectionFixture, []byte) {
	t.Helper()
	path := fenceProjectionFixturePath(cityPath, sessionID)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read session-fence projection: %v", err)
	}
	var projection fenceProjectionFixture
	if err := json.Unmarshal(data, &projection); err != nil {
		t.Fatalf("decode session-fence projection: %v\n%s", err, data)
	}
	return projection, data
}

type fenceProjectionObservingProvider struct {
	*runtime.Fake
	cityPath       string
	projectionSeen fenceProjectionFixture
	projectionRaw  []byte
	stopState      string
}

type quarantineCommitFailStore struct {
	*beads.MemStore
}

func (s *quarantineCommitFailStore) SetMetadataBatch(id string, kvs map[string]string) error {
	if kvs["state"] == string(StateQuarantined) {
		return fmt.Errorf("injected quarantine commit failure")
	}
	return s.MemStore.SetMetadataBatch(id, kvs)
}

func (p *fenceProjectionObservingProvider) Start(ctx context.Context, name string, cfg runtime.Config) error {
	sessionID := cfg.Env["GC_SESSION_ID"]
	path := fenceProjectionFixturePath(p.cityPath, sessionID)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("projection absent before provider start: %w", err)
	}
	if err := json.Unmarshal(data, &p.projectionSeen); err != nil {
		return fmt.Errorf("projection malformed before provider start: %w", err)
	}
	p.projectionRaw = append([]byte(nil), data...)
	return p.Fake.Start(ctx, name, cfg)
}

func (p *fenceProjectionObservingProvider) Stop(name string) error {
	for _, call := range p.Calls {
		if call.Method != "Start" {
			continue
		}
		path := fenceProjectionFixturePath(p.cityPath, call.Config.Env["GC_SESSION_ID"])
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("projection absent before provider stop: %w", err)
		}
		var projection fenceProjectionFixture
		if err := json.Unmarshal(data, &projection); err != nil {
			return fmt.Errorf("projection malformed before provider stop: %w", err)
		}
		p.stopState = projection.State
		break
	}
	return p.Fake.Stop(name)
}

func newProjectedSessionFixture(t *testing.T) (*Manager, *fenceProjectionObservingProvider, Info, string) {
	t.Helper()
	cityPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityPath, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	store := beads.NewMemStore()
	provider := &fenceProjectionObservingProvider{Fake: runtime.NewFake(), cityPath: cityPath}
	manager := NewManagerWithOptions(store, provider, WithCityPath(cityPath))
	info, err := manager.CreateSession(context.Background(), CreateOptions{
		Template: "worker",
		Title:    "worker",
		Command:  "test-provider",
		Provider: "fake",
	})
	if err != nil {
		t.Fatalf("create projected session: %v", err)
	}
	return manager, provider, info, cityPath
}

func TestManagerPublishesHashedFenceProjectionBeforeProviderStart(t *testing.T) {
	_, provider, info, cityPath := newProjectedSessionFixture(t)
	projection, data := readFenceProjectionFixture(t, cityPath, info.ID)
	if projection.SchemaVersion != 1 || projection.SessionID != info.ID || projection.Generation != 1 || projection.State != string(StateActive) {
		t.Fatalf("projection = %+v, want version/id/generation/active", projection)
	}
	if projection.InstanceTokenSHA256 == "" || bytes.Contains(data, []byte(info.InstanceToken)) {
		t.Fatalf("projection does not contain only a token digest: %s", data)
	}
	if _, err := time.Parse(time.RFC3339Nano, projection.ProjectedAt); err != nil {
		t.Fatalf("projected_at = %q: %v", projection.ProjectedAt, err)
	}
	if provider.projectionSeen.SessionID != info.ID || bytes.Contains(provider.projectionRaw, []byte(info.InstanceToken)) {
		t.Fatalf("provider start observed invalid projection: %+v raw=%s", provider.projectionSeen, provider.projectionRaw)
	}
	entries, err := os.ReadDir(filepath.Dir(fenceProjectionFixturePath(cityPath, info.ID)))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != info.ID+".json" {
		t.Fatalf("projection directory entries = %v, want only atomically renamed final file", entries)
	}
}

func TestManagerTombstonesFenceProjectionBeforeLifecycleTeardown(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Manager, string) error
	}{
		{name: "suspend", mutate: func(m *Manager, id string) error { return m.Suspend(id) }},
		{name: "drain", mutate: func(m *Manager, id string) error { return m.BeginDrain(id, "test") }},
		{name: "quarantine", mutate: func(m *Manager, id string) error {
			return m.Quarantine(id, time.Now().UTC().Add(time.Hour), 1)
		}},
		{name: "close", mutate: func(m *Manager, id string) error { return m.Close(id) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager, provider, info, cityPath := newProjectedSessionFixture(t)
			if err := tc.mutate(manager, info.ID); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			projection, data := readFenceProjectionFixture(t, cityPath, info.ID)
			if projection.State != "tombstoned" {
				t.Fatalf("projection state after %s = %q, want tombstoned", tc.name, projection.State)
			}
			if bytes.Contains(data, []byte(info.InstanceToken)) {
				t.Fatalf("tombstone leaked raw token after %s: %s", tc.name, data)
			}
			if tc.name != "drain" && tc.name != "quarantine" && provider.stopState != "tombstoned" {
				t.Fatalf("provider Stop observed projection state %q, want tombstoned", provider.stopState)
			}
		})
	}
}

// TestManagerQuarantineTombstonesFenceBeforeFailedStateCommit proves the
// fail-closed ordering independently of the successful transition: even when
// the authoritative quarantine write fails, the already-running worker must
// lose claim eligibility before that commit is attempted.
func TestManagerQuarantineTombstonesFenceBeforeFailedStateCommit(t *testing.T) {
	cityPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityPath, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	store := &quarantineCommitFailStore{MemStore: beads.NewMemStore()}
	provider := &fenceProjectionObservingProvider{Fake: runtime.NewFake(), cityPath: cityPath}
	manager := NewManagerWithOptions(store, provider, WithCityPath(cityPath))
	info, err := manager.CreateSession(context.Background(), CreateOptions{
		Template: "worker",
		Title:    "worker",
		Command:  "test-provider",
		Provider: "fake",
	})
	if err != nil {
		t.Fatalf("create projected session: %v", err)
	}

	err = manager.Quarantine(info.ID, time.Now().UTC().Add(time.Hour), 1)
	if err == nil {
		t.Fatal("Quarantine succeeded, want injected state-commit failure")
	}
	projection, _ := readFenceProjectionFixture(t, cityPath, info.ID)
	if projection.State != sessionFenceProjectionStateTombstone {
		t.Fatalf("projection state after failed quarantine commit = %q, want tombstoned", projection.State)
	}
	persisted, getErr := store.Get(info.ID)
	if getErr != nil {
		t.Fatalf("get session after failed quarantine commit: %v", getErr)
	}
	if got := persisted.Metadata["state"]; got != string(StateActive) {
		t.Fatalf("persisted state after failed quarantine commit = %q, want active", got)
	}
}

func TestPublishLiveSessionFenceProjectionDoesNotReviveQuarantineTombstone(t *testing.T) {
	manager, _, staleActive, cityPath := newProjectedSessionFixture(t)
	if err := manager.Quarantine(staleActive.ID, time.Now().UTC().Add(time.Hour), 1); err != nil {
		t.Fatalf("Quarantine: %v", err)
	}

	err := WithSessionMutationLock(staleActive.ID, func() error {
		return PublishLiveSessionFenceProjection(cityPath, staleActive)
	})
	if err == nil {
		t.Fatal("stale active projection refresh succeeded after quarantine tombstone")
	}
	if !errors.Is(err, ErrLiveSessionFenceProjectionRefused) {
		t.Fatalf("stale active projection refresh error = %v, want expected refusal", err)
	}
	projection, _ := readFenceProjectionFixture(t, cityPath, staleActive.ID)
	if projection.State != sessionFenceProjectionStateTombstone {
		t.Fatalf("projection state after stale live refresh = %q, want tombstoned", projection.State)
	}
}

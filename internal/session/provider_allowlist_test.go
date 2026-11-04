package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/runtime"
)

func TestDirectStartRefusesProviderAllowlistAuthority(t *testing.T) {
	city := t.TempDir()
	dir := filepath.Join(city, ".gc", "platform")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "install-manifest.json"), []byte(`{"integrity":{"provider_allowlist":{}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := beads.NewMemStore()
	bead, err := store.Create(beads.Bead{Title: "test session"})
	if err != nil {
		t.Fatal(err)
	}
	provider := runtime.NewFake()
	manager := NewManagerWithOptions(store, provider, WithCityPath(city))
	err = manager.startRuntime(context.Background(), bead.ID, "test-session", runtime.Config{Command: "fake"})
	if err == nil || !strings.Contains(err.Error(), "provider allowlist") {
		t.Fatalf("direct start accepted invalid authority: %v", err)
	}
	for _, call := range provider.SnapshotCalls() {
		if call.Method == "Start" {
			t.Fatal("started provider after failed authority check")
		}
	}
}

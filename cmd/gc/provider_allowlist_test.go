package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreparedStartRefusesInvalidProviderAllowlistAuthority(t *testing.T) {
	city := t.TempDir()
	dir := filepath.Join(city, ".gc", "platform")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "install-manifest.json"), []byte(`{"integrity":{"provider_allowlist":{}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	item := preparedStart{}
	configureManagedWorkerPreflight(&item, city, nil, "", nil, nil)
	if err := item.preflight(context.Background()); err == nil || !strings.Contains(err.Error(), "provider allowlist") {
		t.Fatalf("invalid authority must refuse even without provisioning receipt: %v", err)
	}
}

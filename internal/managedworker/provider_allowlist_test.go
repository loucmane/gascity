package managedworker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/platforminstall"
	"github.com/gastownhall/gascity/internal/providerallowlist"
)

func TestReceiptProviderAllowlistApproval(t *testing.T) {
	receipt, encoded := finalizedReceipt(t)
	path := filepath.Join(t.TempDir(), "providers.json")
	data := []byte(`{"schema":"gc.provider-allowlist.v1","entries":[]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(encoded, &doc); err != nil {
		t.Fatal(err)
	}
	doc["provider_allowlist"] = map[string]string{"path": path, "sha256": sha256Hex(data)}
	delete(doc, "receipt_sha256")
	canonical, err := canonicalJSON(doc)
	if err != nil {
		t.Fatal(err)
	}
	doc["receipt_sha256"] = sha256Hex(canonical)
	encoded, err = canonicalJSON(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProvisioningReceipt(encoded); err != nil {
		t.Fatalf("allowlist receipt refused: %v", err)
	}
	if _, err := providerallowlist.ReadApproved(path, encoded, receipt.PermissionRevision); err != nil {
		t.Fatal(err)
	}
	probes := Probes{
		ReadFile: func(string) ([]byte, error) { return []byte("changed allowlist"), nil },
		InspectProvider: func(context.Context, platforminstall.ProviderPin) error {
			t.Fatal("executed provider before approval")
			return nil
		},
		InspectToolchain: func(context.Context, ToolchainPin, map[string]string) error { return nil },
		ProbeReadiness:   func(context.Context, string) error { return nil },
		ProbeSigner:      func(context.Context, string) error { return nil },
	}
	_, err = Preflight(context.Background(), PreflightRequest{
		Receipt: encoded, PermissionRevision: receipt.PermissionRevision, ProfileName: receipt.Profiles[0].Name,
	}, probes)
	if err == nil || !strings.Contains(err.Error(), "provider_allowlist sha256 mismatch") {
		t.Fatalf("unapproved allowlist was not refused: %v", err)
	}
}

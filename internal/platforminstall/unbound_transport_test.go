package platforminstall

import (
	"os"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/providerallowlist"
)

func TestRefuseUnboundProviderTransport(t *testing.T) {
	for _, state := range []string{"absent", "undelegated", "delegated", "missing-manifest", "malformed-manifest"} {
		t.Run(state, func(t *testing.T) {
			city := t.TempDir()
			if state != "absent" {
				manifest := testManifest(t, city, []byte("candidate"), []byte("installed"))
				if state != "undelegated" {
					manifest.Integrity = &IntegritySpec{ProviderAllowlist: &providerallowlist.Reference{
						Path: providerallowlist.Path(city), Bindings: []providerallowlist.Binding{{Provider: "codex", Path: city + "/codex"}},
					}}
				}
				data, err := MarshalManifest(finalizeManifest(t, manifest))
				if err != nil {
					t.Fatal(err)
				}
				writeProviderFixture(t, DefaultManifestPath(city), data)
				if state == "missing-manifest" {
					writeProviderFixture(t, providerallowlist.Path(city), []byte("{}"))
					if err := os.Remove(DefaultManifestPath(city)); err != nil {
						t.Fatal(err)
					}
				}
				if state == "malformed-manifest" {
					writeProviderFixture(t, DefaultManifestPath(city), []byte(`{"integrity":{"provider_allowlist":`))
				}
			}
			err := RefuseUnboundProviderTransport("fixture", "display-name", "", t.TempDir(), city)
			wantRefusal := state != "absent" && state != "undelegated"
			if !wantRefusal {
				if err != nil {
					t.Fatalf("legacy transport refused: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "fixture: provider allowlist") {
				t.Fatalf("wanted contextual refusal for %s, got %v", state, err)
			}
			if state == "delegated" && !strings.Contains(err.Error(), "cannot enforce the approved executable") {
				t.Fatalf("missing transport explanation: %v", err)
			}
		})
	}
}

package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/platforminstall"
	"github.com/gastownhall/gascity/internal/providerallowlist"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/shellquote"
	"github.com/gastownhall/gascity/internal/testutil/providerlaunch"
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

func TestNativeLaunchUsesApprovedExecutable(t *testing.T) {
	for _, shape := range []string{"stable", "immutable", "other path", "PATH name", "stored resume", "missing manifest", "missing list", "legacy stale manifest"} {
		t.Run(shape, func(t *testing.T) {
			city, stable, target := approvedNativeFixture(t)
			command := shellquote.Quote(stable) + " --resume 'stored conversation'"
			wantPass := shape == "stable" || shape == "immutable" || shape == "legacy stale manifest"
			switch shape {
			case "immutable":
				command = shellquote.Quote(target)
			case "other path":
				command = "/another/codex"
			case "PATH name":
				command = "codex --resume stored"
			case "stored resume":
				command = BuildResumeCommand(Info{Command: stable, ResumeCommand: "/another/codex --resume {{.SessionKey}}", SessionKey: "stored"})
			case "missing manifest":
				if err := os.Remove(platforminstall.DefaultManifestPath(city)); err != nil {
					t.Fatal(err)
				}
			case "missing list":
				if err := os.Remove(providerallowlist.Path(city)); err != nil {
					t.Fatal(err)
				}
			case "legacy stale manifest":
				if err := os.Remove(providerallowlist.Path(city)); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(providerallowlist.ReceiptPath(city)); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(platforminstall.DefaultManifestPath(city))
				if err != nil {
					t.Fatal(err)
				}
				manifest, err := platforminstall.LoadManifest(data)
				if err != nil {
					t.Fatal(err)
				}
				manifest.Integrity.ProviderAllowlist = nil
				data, err = platforminstall.MarshalManifest(manifest)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(platforminstall.DefaultManifestPath(city), data, 0o600); err != nil {
					t.Fatal(err)
				}
				command = "codex --resume stored"
			}
			store := beads.NewMemStore()
			bead, err := store.Create(beads.Bead{Title: "native session"})
			if err != nil {
				t.Fatal(err)
			}
			provider := runtime.NewFake()
			manager := NewManagerWithOptions(store, provider, WithCityPath(city))
			err = manager.startRuntime(context.Background(), bead.ID, "native", runtime.Config{Command: command, Env: map[string]string{"PATH": "/another"}})
			if wantPass {
				if err != nil {
					t.Fatalf("approved launch refused: %v", err)
				}
				for _, call := range provider.SnapshotCalls() {
					if call.Method == "Start" {
						if shape != "legacy stale manifest" && call.Config.Env["GC_CITY_PATH"] != city {
							t.Errorf("delegated runtime lost authoritative city: %q", call.Config.Env["GC_CITY_PATH"])
						}
						if shape == "legacy stale manifest" && call.Config.Env["GC_CITY_PATH"] != "" {
							t.Error("legacy runtime environment changed")
						}
						if shape == "legacy stale manifest" && call.Config.Command != command {
							t.Fatalf("legacy command changed: %q", call.Config.Command)
						}
						want := "/usr/bin/python3"
						if shape == "legacy stale manifest" {
							want = "codex"
						}
						if got := shellquote.Split(call.Config.Command)[0]; got != want {
							t.Fatalf("launched %q, want %q", got, want)
						}
					}
				}
			} else {
				if err == nil {
					t.Fatal("unapproved launch accepted")
				}
				for _, call := range provider.SnapshotCalls() {
					if call.Method == "Start" {
						t.Fatal("runtime launched after refusal")
					}
				}
			}
		})
	}
}

func approvedNativeFixture(t *testing.T) (city, stable, target string) {
	t.Helper()
	f := providerlaunch.New(t, "codex")
	return f.City, f.Stable, f.Target
}

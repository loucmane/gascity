package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/api"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/platforminstall"
	"github.com/gastownhall/gascity/internal/providerallowlist"
	"github.com/gastownhall/gascity/internal/testutil/providerlaunch"
)

func TestInitPreflightUsesCityApproval(t *testing.T) {
	old := initProbeProvidersReadiness
	initProbeProvidersReadiness = api.ProbeProviders
	t.Cleanup(func() { initProbeProvidersReadiness = old })
	for _, state := range []string{"approved", "replacement", "missing-list", "missing-manifest", "legacy", "legacy-malformed", "legacy-directory"} {
		t.Run(state, func(t *testing.T) {
			legacy := strings.HasPrefix(state, "legacy")
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("GC_BEADS", "file")
			marker := filepath.Join(home, "ran")
			auth := "\necho '{\"loggedIn\":true,\"authMethod\":\"claude.ai\",\"apiProvider\":\"firstParty\"}'\n"
			f := providerlaunch.NewWithCLI(t, "claude", []byte("#!/bin/sh\necho approved >> "+marker+auth))
			generic := filepath.Join(home, ".local/bin/claude")
			if err := os.MkdirAll(filepath.Dir(generic), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(generic, []byte("#!/bin/sh\necho generic >> "+marker+auth), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", filepath.Dir(generic)+":/usr/bin:/bin")
			switch state {
			case "replacement":
				if err := os.WriteFile(f.Target+".new", []byte("#!/bin/sh\necho replacement >> "+marker+auth), 0o555); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(f.Target+".new", f.Target); err != nil {
					t.Fatal(err)
				}
			case "missing-list":
				if err := os.Remove(providerallowlist.Path(f.City)); err != nil {
					t.Fatal(err)
				}
			case "missing-manifest":
				if err := os.Remove(platforminstall.DefaultManifestPath(f.City)); err != nil {
					t.Fatal(err)
				}
			case "legacy", "legacy-malformed", "legacy-directory":
				for _, path := range []string{providerallowlist.Path(f.City), providerallowlist.ReceiptPath(f.City), platforminstall.DefaultManifestPath(f.City)} {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				}
				if state == "legacy-malformed" {
					if err := os.WriteFile(platforminstall.DefaultManifestPath(f.City), []byte("not-json"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				if state == "legacy-directory" {
					if err := os.Mkdir(platforminstall.DefaultManifestPath(f.City), 0o755); err != nil {
						t.Fatal(err)
					}
				}
			}
			base := "builtin:claude"
			cfg := &config.City{
				Workspace: config.Workspace{Provider: "claude"},
				Providers: map[string]config.ProviderSpec{"claude": {Base: &base}},
			}
			var stdout, stderr bytes.Buffer
			err := runInitProviderPreflightForConfig(f.City, cfg, &stdout, &stderr, "gc init")
			data, readErr := os.ReadFile(marker)
			if state == "approved" || legacy {
				want := "approved\n"
				if legacy {
					want = "generic\n"
				}
				if err != nil || readErr != nil || string(data) != want {
					t.Fatalf("init ran %q, want %q: %v, %v; %s", data, want, err, readErr, stderr.String())
				}
			} else if err == nil || !os.IsNotExist(readErr) {
				t.Fatalf("init must refuse before auth: ran %q, err %v, read %v; %s", data, err, readErr, stderr.String())
			}
		})
	}
}

package api

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/testutil/providerlaunch"
)

func TestDelegatedReadinessRunsApprovedClaude(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "discovery", true: "replacement"}[replace], func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			marker := filepath.Join(home, "ran")
			auth := "\necho '{\"loggedIn\":true,\"authMethod\":\"claude.ai\",\"apiProvider\":\"firstParty\"}'\n"
			approved := []byte("#!/bin/sh\necho approved-2.1.280 >> " + marker + auth)
			f := providerlaunch.NewWithCLI(t, "claude", approved)
			generic := filepath.Join(home, ".local/bin/claude")
			if err := os.MkdirAll(filepath.Dir(generic), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(generic, []byte("#!/bin/sh\necho generic-2.1.318 >> "+marker+auth), 0o755); err != nil {
				t.Fatal(err)
			}
			if replace {
				old := providerProbeCommandContext
				t.Cleanup(func() { providerProbeCommandContext = old })
				providerProbeCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
					path := f.Target + ".replacement"
					if err := os.WriteFile(path, []byte("#!/bin/sh\necho replacement >> "+marker+auth), 0o555); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(path, f.Target); err != nil {
						t.Fatal(err)
					}
					return exec.CommandContext(ctx, name, args...)
				}
			}
			result, err := ProbeCityProvider(context.Background(), f.City, "", "claude")
			if err != nil || result.Status != ProbeStatusConfigured {
				t.Fatalf("probe = %+v, %v", result, err)
			}
			data, err := os.ReadFile(marker)
			if err != nil || string(data) != "approved-2.1.280\n" {
				t.Fatalf("ran %q: %v", data, err)
			}
		})
	}
}

func TestDelegatedTitleRefusesUnapprovedCommand(t *testing.T) {
	f := providerlaunch.New(t, "claude")
	command := filepath.Join(t.TempDir(), "unapproved")
	if err := os.WriteFile(command, []byte("#!/bin/sh\necho UNAPPROVED\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	provider := &config.ResolvedProvider{Command: command, PrintArgs: []string{"-p"}}
	if got := generateTitleWithContext(context.Background(), provider, "fallback", "", f.City); got != "fallback" {
		t.Fatalf("unapproved title command executed: %q", got)
	}
}

func TestCityReadinessDoesNotDiscoverUnapprovedClaude(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	marker := filepath.Join(home, "ran")
	path := filepath.Join(home, ".local/bin/claude")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho unapproved > "+marker+"\necho '{}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	f := providerlaunch.NewWithCLI(t, "claude", []byte("#!/bin/sh\necho '{\"loggedIn\":true,\"authMethod\":\"claude.ai\",\"apiProvider\":\"firstParty\"}'\n"))
	response, err := buildCityReadinessResponse(context.Background(), f.City, []string{"claude", "gemini"}, true)
	if err != nil || response.Items["claude"].Status != ProbeStatusConfigured {
		t.Fatalf("readiness: %+v, %v", response, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("generic Claude executed: %v", err)
	}
}

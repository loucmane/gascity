package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gastownhall/gascity/internal/managedworker"
	"github.com/gastownhall/gascity/internal/testutil/providerlaunch"
)

func TestDelegatedToolchainProbeRetainsVerifiedInode(t *testing.T) {
	root := t.TempDir()
	path, marker := filepath.Join(root, "tool"), filepath.Join(root, "ran")
	approved := "#!/bin/sh\necho approved > " + marker + "\necho version\n"
	if err := os.WriteFile(path, []byte(approved), 0o755); err != nil {
		t.Fatal(err)
	}
	pin := managedworker.ToolchainPin{Name: "tool", Executable: managedworker.ExecutablePin{Path: path, ResolvedPath: path, SHA256: preflightDigest(approved), Version: "version", VersionArgs: []string{"--version"}}}
	old := managedToolchainCommandContext
	t.Cleanup(func() { managedToolchainCommandContext = old })
	managedToolchainCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		replacement := path + ".new"
		if err := os.WriteFile(replacement, []byte("#!/bin/sh\necho replacement > "+marker+"\necho version\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(replacement, path); err != nil {
			t.Fatal(err)
		}
		return exec.CommandContext(ctx, name, args...)
	}
	if err := probeManagedWorkerToolchainDescriptor(context.Background(), pin, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "approved\n" {
		t.Fatalf("ran %q: %v", data, err)
	}
}

func TestPreflightReadinessUsesCityApproval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	marker := filepath.Join(home, "ran")
	auth := "\necho '{\"loggedIn\":true,\"authMethod\":\"claude.ai\",\"apiProvider\":\"firstParty\"}'\n"
	f := providerlaunch.NewWithCLI(t, "claude", []byte("#!/bin/sh\necho approved-2.1.280 >> "+marker+auth))
	generic := filepath.Join(home, ".local/bin/claude")
	if err := os.MkdirAll(filepath.Dir(generic), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(generic, []byte("#!/bin/sh\necho generic-2.1.318 >> "+marker+auth), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := cityManagedWorkerPreflightProbes(f.City, "").ProbeReadiness(context.Background(), "claude"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "approved-2.1.280\n" {
		t.Fatalf("readiness ran %q: %v", data, err)
	}
}

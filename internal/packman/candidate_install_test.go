package packman

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/fsys"
)

func TestInstallLockedCandidateCachesMissingCommitUsingCredentialCity(t *testing.T) {
	home := t.TempDir()
	candidate := t.TempDir()
	credentialCity := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GC_HOME", filepath.Join(home, ".gc"))
	source := "https://example.com/gascity-packs.git"
	commit := "0123456789abcdef"
	if err := os.WriteFile(filepath.Join(candidate, "pack.toml"), []byte(`[pack]
name = "city"
schema = 2

[imports.gascity]
source = "https://example.com/gascity-packs.git"
version = "sha:0123456789abcdef"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteLockfile(fsys.OSFS{}, candidate, &Lockfile{
		Schema: LockfileSchema,
		Packs: map[string]LockedPack{
			source: {Version: "sha:" + commit, Commit: commit, Fetched: time.Unix(10, 0).UTC()},
		},
	}); err != nil {
		t.Fatal(err)
	}

	previousRunGit := runGit
	previousRunNetworkGit := runNetworkGit
	cloneCalls := 0
	credentialRoots := make([]string, 0, 1)
	runNetworkGit = func(cityRoot, _ string, _ string, args ...string) (string, error) {
		credentialRoots = append(credentialRoots, cityRoot)
		cloneCalls++
		target := args[len(args)-1]
		if err := os.MkdirAll(filepath.Join(target, ".git"), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(target, "pack.toml"), []byte("[pack]\nname = \"gascity\"\nschema = 2\n"), 0o644); err != nil {
			return "", err
		}
		return "", nil
	}
	runGit = func(dir string, args ...string) (string, error) {
		switch args[0] {
		case "checkout":
			if err := os.WriteFile(filepath.Join(dir, ".candidate-test-commit"), []byte(args[len(args)-1]), 0o644); err != nil {
				return "", err
			}
			return "", nil
		case "rev-parse":
			data, err := os.ReadFile(filepath.Join(dir, ".candidate-test-commit"))
			return string(data), err
		case "status":
			return "", nil
		default:
			return previousRunGit(dir, args...)
		}
	}
	t.Cleanup(func() {
		runGit = previousRunGit
		runNetworkGit = previousRunNetworkGit
	})

	if _, err := InstallLockedCandidate(candidate, credentialCity); err != nil {
		t.Fatalf("InstallLockedCandidate() error = %v", err)
	}
	if cloneCalls != 1 || len(credentialRoots) != 1 || credentialRoots[0] != credentialCity {
		t.Fatalf("clone calls=%d credential roots=%v, want one call using %q", cloneCalls, credentialRoots, credentialCity)
	}
	cachePath, err := RepoCachePath(source, commit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cachePath, "pack.toml")); err != nil {
		t.Fatalf("candidate cache was not materialized: %v", err)
	}
	if _, err := InstallLockedCandidate(candidate, credentialCity); err != nil {
		t.Fatalf("InstallLockedCandidate() replay error = %v", err)
	}
	if cloneCalls != 1 {
		t.Fatalf("replay clone calls = %d, want 1", cloneCalls)
	}
}

func TestInstallLockedCandidateRejectsStaleLockEntry(t *testing.T) {
	home := t.TempDir()
	candidate := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GC_HOME", filepath.Join(home, ".gc"))
	stubCachedPackGit(t)
	source := "https://example.com/stale.git"
	commit := "abcdef0123456789"
	if err := os.WriteFile(filepath.Join(candidate, "pack.toml"), []byte("[pack]\nname = \"city\"\nschema = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteLockfile(fsys.OSFS{}, candidate, &Lockfile{
		Schema: LockfileSchema,
		Packs: map[string]LockedPack{
			source: {Version: "sha:" + commit, Commit: commit, Fetched: time.Unix(10, 0).UTC()},
		},
	}); err != nil {
		t.Fatal(err)
	}
	stageCachedPack(t, source, commit, "[pack]\nname = \"stale\"\nschema = 2\n")

	_, err := InstallLockedCandidate(candidate, candidate)
	if err == nil || !strings.Contains(err.Error(), "stale-lock-entry") {
		t.Fatalf("InstallLockedCandidate() error = %v, want stale-lock-entry refusal", err)
	}
}

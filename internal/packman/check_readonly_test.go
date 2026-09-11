package packman

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/config"
)

func TestCheckInstalledReadOnlyRequiresExistingCoordination(t *testing.T) {
	for _, scenario := range []string{"missing-root", "missing-lock"} {
		t.Run(scenario, func(t *testing.T) {
			home, city := t.TempDir(), t.TempDir()
			t.Setenv("GC_HOME", home)
			source, commit := "https://example.invalid/cache.git", "0123456789abcdef"
			writeTestLockfile(t, city, map[string]LockedPack{source: {Version: "sha:" + commit, Commit: commit}})
			root, err := RepoCacheRoot()
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "missing-lock" {
				if err := os.MkdirAll(root, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			_, err = CheckInstalledReadOnly(city, map[string]config.Import{"cache": {Source: source, Version: "sha:" + commit}})
			if err == nil || !strings.Contains(err.Error(), "existing repo cache") {
				t.Fatalf("read-only check error = %v, want existing coordination refusal", err)
			}
			if _, err := os.Lstat(filepath.Join(root, ".packman-cache.lock")); !os.IsNotExist(err) {
				t.Fatalf("check created cache lock: %v", err)
			}
		})
	}
}

func TestCheckInstalledReadOnlyRetainsCacheValidationAndSuppressesOptionalWrites(t *testing.T) {
	for _, scenario := range []string{"clean", "dirty", "wrong-head", "missing-cache"} {
		t.Run(scenario, func(t *testing.T) {
			home, city := t.TempDir(), t.TempDir()
			t.Setenv("GC_HOME", home)
			source, commit := "https://example.invalid/cache.git", "0123456789abcdef"
			writeTestLockfile(t, city, map[string]LockedPack{source: {Version: "sha:" + commit, Commit: commit}})
			cachePath, err := RepoCachePath(source, commit)
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Dir(cachePath)
			if err := os.MkdirAll(root, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, ".packman-cache.lock"), nil, 0o400); err != nil {
				t.Fatal(err)
			}
			if scenario != "missing-cache" {
				if err := os.MkdirAll(filepath.Join(cachePath, ".git"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(cachePath, "pack.toml"), []byte("[pack]\nname = \"cache\"\nschema = 2\n"), 0o400); err != nil {
					t.Fatal(err)
				}
			}
			previous := runGit
			t.Cleanup(func() { runGit = previous })
			runGit = func(directory string, arguments ...string) (string, error) {
				if directory != cachePath || len(arguments) < 2 || arguments[0] != "--no-optional-locks" {
					return "", fmt.Errorf("unsafe read-only Git invocation: %q %q", directory, arguments)
				}
				switch strings.Join(arguments[1:], " ") {
				case "rev-parse HEAD":
					if scenario == "wrong-head" {
						return "fedcba9876543210", nil
					}
					return commit, nil
				case "status --porcelain":
					if scenario == "dirty" {
						return " M pack.toml", nil
					}
					return "", nil
				default:
					return "", fmt.Errorf("unexpected Git command: %q", arguments)
				}
			}
			report, err := CheckInstalledReadOnly(city, map[string]config.Import{"cache": {Source: source, Version: "sha:" + commit}})
			if err != nil {
				t.Fatal(err)
			}
			wantIssue := map[string]string{"dirty": "cache-worktree-dirty", "wrong-head": "cache-checkout-mismatch", "missing-cache": "missing-cache"}[scenario]
			if wantIssue == "" {
				if report.HasIssues() || report.CheckedSources != 1 {
					t.Fatalf("clean cache report = %+v", report)
				}
			} else {
				assertSingleIssue(t, report, wantIssue)
			}
		})
	}
}

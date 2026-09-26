package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeLaunchPath(t *testing.T) {
	good := []string{"/home/u/wt/gct-1", "/a/b.c/d_e-f", "/"}
	for _, p := range good {
		if !SafeLaunchPath(p) {
			t.Errorf("SafeLaunchPath(%q) = false, want true", p)
		}
	}
	bad := []string{
		"", "relative/dir", "/a/../b", "/a/./b", "/a//b", "/a/b/",
		"/repo/.git", "/repo/.GIT/x", "/repo/.Git",
		"/a/b;rm -rf", "/a/$(curl x)", "/a/b c", "/a/`x`", "/a/b'q", "/a/b\"q", "/a/b|c", "/a/b&c", "/a/b\nc",
	}
	for _, p := range bad {
		if SafeLaunchPath(p) {
			t.Errorf("SafeLaunchPath(%q) = true, want false", p)
		}
	}
}

func TestGuardLaunchWorkDirContainment(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "worktrees")
	sibling := filepath.Join(root, "worktrees-evil")
	outside := filepath.Join(root, "outside")
	for _, d := range []string{allowed, sibling, outside, filepath.Join(allowed, "task", ".git")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A symlink inside the root that points outside it.
	escape := filepath.Join(allowed, "escape")
	if err := os.Symlink(outside, escape); err != nil {
		t.Fatal(err)
	}
	// A symlink inside the root that points into a .git directory.
	toGit := filepath.Join(allowed, "togit")
	if err := os.Symlink(filepath.Join(allowed, "task", ".git"), toGit); err != nil {
		t.Fatal(err)
	}

	accept := []string{
		allowed,
		filepath.Join(allowed, "task"),
		filepath.Join(allowed, "not-yet-created", "deeper"),
	}
	for _, p := range accept {
		if _, ok := GuardLaunchWorkDir(p, []string{allowed}, ""); !ok {
			t.Errorf("GuardLaunchWorkDir(%q) refused, want accepted", p)
		}
	}
	refuse := []string{
		sibling,                                // string-prefix sibling
		outside,                                // outside every root
		escape,                                 // symlink escape
		filepath.Join(escape, "sub"),           // through the escape
		toGit,                                  // symlink into .git
		filepath.Join(allowed, "task", ".git"), // .git component
		filepath.Join(allowed, "x;y"),          // shell metacharacter
	}
	for _, p := range refuse {
		if _, ok := GuardLaunchWorkDir(p, []string{allowed}, ""); ok {
			t.Errorf("GuardLaunchWorkDir(%q) accepted, want refused", p)
		}
	}
}

func TestGuardLaunchWorkDirExactRoot(t *testing.T) {
	city := t.TempDir()
	sub := filepath.Join(city, "rigs")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := GuardLaunchWorkDir(city, nil, city); !ok {
		t.Fatalf("exact root refused")
	}
	if _, ok := GuardLaunchWorkDir(sub, nil, city); ok {
		t.Fatalf("descendant of an exact-only root accepted")
	}
}

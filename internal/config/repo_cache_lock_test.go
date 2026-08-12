//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestWithRepoCacheReadLockDoesNotCreateMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	called := false
	if err := WithRepoCacheReadLock(root, func() error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("WithRepoCacheReadLock: %v", err)
	}
	if !called {
		t.Fatal("read lock callback was not called")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("root stat err = %v, want not exist", err)
	}
}

func TestWithRepoCacheReadLockFailsClosedWhenLockFileIsMissing(t *testing.T) {
	root := t.TempDir()
	called := false
	err := WithRepoCacheReadLock(root, func() error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("WithRepoCacheReadLock succeeded without an existing lock file")
	}
	if called {
		t.Fatal("read lock callback ran without an existing lock file")
	}
	if _, statErr := os.Stat(filepath.Join(root, repoCacheLockName)); !os.IsNotExist(statErr) {
		t.Fatalf("lock file stat err = %v, want not exist", statErr)
	}
}

func TestWithRepoCacheReadLockOpensExistingLockReadOnly(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, repoCacheLockName)
	wantContents := []byte("lock-sentinel\n")
	if err := os.WriteFile(lockPath, wantContents, 0o444); err != nil {
		t.Fatalf("write read-only lock file: %v", err)
	}
	if err := os.Chmod(lockPath, 0o444); err != nil {
		t.Fatalf("chmod read-only lock file: %v", err)
	}

	called := false
	if err := WithRepoCacheReadLock(root, func() error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("WithRepoCacheReadLock on read-only lock file: %v", err)
	}
	if !called {
		t.Fatal("read lock callback was not called")
	}
	gotContents, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read lock file: %v", err)
	}
	if string(gotContents) != string(wantContents) {
		t.Fatalf("lock contents = %q, want %q", gotContents, wantContents)
	}
	info, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("stat lock file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o444 {
		t.Fatalf("lock mode = %04o, want 0444", got)
	}
}

func TestWithRepoCacheWriteLockCreatesLockFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	called := false
	result, err := WithRepoCacheWriteLock(root, func() (string, error) {
		called = true
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("WithRepoCacheWriteLock: %v", err)
	}
	if !called || result != "ok" {
		t.Fatalf("write callback called=%v result=%q, want true and ok", called, result)
	}
	if _, err := os.Stat(filepath.Join(root, repoCacheLockName)); err != nil {
		t.Fatalf("lock file stat: %v", err)
	}
}

func TestRepoCacheRootForPathUsesKnownCacheRootsOnly(t *testing.T) {
	home := t.TempDir()
	gcHome := filepath.Join(t.TempDir(), "gc-home")
	t.Setenv("HOME", home)
	t.Setenv("GC_HOME", filepath.Join(home, ".gc"))
	t.Setenv("GC_HOME", gcHome)

	homeRoot := filepath.Join(home, ".gc", "cache", "repos")
	gcHomeRoot := filepath.Join(gcHome, "cache", "repos")
	for _, tc := range []struct {
		name string
		path string
		root string
		ok   bool
	}{
		{
			name: "home cache child",
			path: filepath.Join(homeRoot, "abc123", "pack.toml"),
			root: homeRoot,
			ok:   true,
		},
		{
			name: "gc home cache child",
			path: filepath.Join(gcHomeRoot, "def456", "pack.toml"),
			root: gcHomeRoot,
			ok:   true,
		},
		{
			name: "unrelated cache repos substring",
			path: filepath.Join(t.TempDir(), "cache", "repos", "project", "pack.toml"),
			ok:   false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := repoCacheRootForPath(tc.path)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (root %q)", ok, tc.ok, got)
			}
			if ok && got != tc.root {
				t.Fatalf("root = %q, want %q", got, tc.root)
			}
		})
	}
}

func TestWithRepoCacheReadLockWaitsOnCacheRoot(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, repoCacheLockName)
	lockDir, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("Open(root): %v", err)
	}
	defer lockDir.Close() //nolint:errcheck
	if err := syscall.Flock(int(lockDir.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatalf("Flock(exclusive): %v", err)
	}

	entered := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- WithRepoCacheReadLock(root, func() error {
			close(entered)
			return nil
		})
	}()

	select {
	case <-entered:
		t.Fatal("read lock entered while exclusive lock was held")
	case <-time.After(50 * time.Millisecond):
	}

	if err := syscall.Flock(int(lockDir.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatalf("Flock(unlock): %v", err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("read lock did not enter after exclusive lock released")
	}
	if err := <-errCh; err != nil {
		t.Fatalf("WithRepoCacheReadLock: %v", err)
	}
}

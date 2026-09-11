package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadOnlyCacheCoordinationNeverInitializesMissingLock(t *testing.T) {
	root := t.TempDir()
	called := false
	err := WithExistingRepoCacheReadLock(root, func() error {
		called = true
		return nil
	})
	if err == nil || called {
		t.Fatalf("missing existing lock: error=%v callback=%t; want refusal before callback", err, called)
	}
	if _, err := os.Lstat(filepath.Join(root, repoCacheLockName)); !os.IsNotExist(err) {
		t.Fatalf("read-only coordination created a lock: %v", err)
	}
}

func TestExistingRepoCacheReadLockPreservesReadOnlyFileAndCallbackError(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, repoCacheLockName)
	if err := os.WriteFile(lockPath, []byte("preserved"), 0o400); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	wantError := errors.New("callback failure")
	err = WithExistingRepoCacheReadLock(root, func() error { return wantError })
	if !errors.Is(err, wantError) {
		t.Fatalf("callback error = %v, want %v", err, wantError)
	}
	after, err := os.Stat(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) || string(contents) != "preserved" {
		t.Fatal("read-only coordination changed the existing lock")
	}
}

func TestExistingRepoCacheReadLockRejectsMissingRootAndNonregularLock(t *testing.T) {
	for _, scenario := range []string{"missing-root", "directory-lock", "symlink-lock"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			lockPath := filepath.Join(root, repoCacheLockName)
			switch scenario {
			case "missing-root":
				root = filepath.Join(root, "absent")
			case "directory-lock":
				if err := os.Mkdir(lockPath, 0o700); err != nil {
					t.Fatal(err)
				}
			case "symlink-lock":
				if err := os.Symlink(filepath.Join(root, "absent"), lockPath); err != nil {
					t.Fatal(err)
				}
			}
			called := false
			err := WithExistingRepoCacheReadLock(root, func() error {
				called = true
				return nil
			})
			if err == nil || called {
				t.Fatalf("unsafe existing lock: error=%v callback=%t", err, called)
			}
			if scenario == "missing-root" {
				if _, err := os.Stat(root); !os.IsNotExist(err) {
					t.Fatalf("missing cache root was created: %v", err)
				}
			}
		})
	}
}

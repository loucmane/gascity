package config

import (
	"os"
	"path/filepath"
	"strings"
)

const repoCacheLockName = ".packman-cache.lock"

func repoCacheLockOpenFlags(createLock bool) int {
	if createLock {
		return os.O_RDWR | os.O_CREATE
	}
	return os.O_RDONLY
}

func openRepoCacheLockFile(path string, exclusive bool) (*os.File, error) {
	if exclusive {
		return os.OpenFile(path, repoCacheLockOpenFlags(true), 0o644)
	}

	lockFile, err := os.OpenFile(path, repoCacheLockOpenFlags(false), 0o644)
	if err == nil || !os.IsNotExist(err) {
		return lockFile, err
	}

	// Preserve the established lazy-initialization behavior for writable
	// caches, but never require write access once the lock exists. O_EXCL
	// makes a concurrent initializer harmless; either way readers reopen the
	// resulting lock read-only before acquiring the shared OS lock.
	initializer, createErr := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if createErr != nil {
		if !os.IsExist(createErr) {
			return nil, createErr
		}
	} else if closeErr := initializer.Close(); closeErr != nil {
		return nil, closeErr
	}
	return os.OpenFile(path, repoCacheLockOpenFlags(false), 0o644)
}

// WithRepoCacheReadLock runs fn while holding the shared repo-cache lock if
// the cache root exists. It never creates the cache root or cache content. A
// writable cache may lazily initialize its coordination lock; existing locks
// are always opened read-only.
func WithRepoCacheReadLock(root string, fn func() error) error {
	return withRepoCacheLock(root, repoCacheLockShared, false, fn)
}

// WithRepoCacheWriteLock runs fn while holding the exclusive repo-cache lock.
func WithRepoCacheWriteLock(root string, fn func() (string, error)) (string, error) {
	var result string
	err := withRepoCacheLock(root, repoCacheLockExclusive, true, func() error {
		var fnErr error
		result, fnErr = fn()
		return fnErr
	})
	return result, err
}

func withRepoCacheReadLockForPath(path string, fn func() error) error {
	root, ok := repoCacheRootForPath(path)
	if !ok {
		return fn()
	}
	return WithRepoCacheReadLock(root, fn)
}

func repoCacheRootForPath(path string) (string, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	for _, root := range repoCacheRootCandidates() {
		if pathWithinDir(abs, root) {
			return root, true
		}
	}
	return "", false
}

func repoCacheRootCandidates() []string {
	var roots []string
	add := func(root string) {
		if root == "" {
			return
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			abs = root
		}
		for _, existing := range roots {
			if existing == abs {
				return
			}
		}
		roots = append(roots, abs)
	}
	if gcHome := ImplicitGCHome(); gcHome != "" {
		add(filepath.Join(gcHome, "cache", "repos"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		add(filepath.Join(home, ".gc", "cache", "repos"))
	}
	return roots
}

func pathWithinDir(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || (!filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

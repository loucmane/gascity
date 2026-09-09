package tmuxtest

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"
)

func TestIsolatedSocketParentPreservesHistoricalSibling(t *testing.T) {
	root := t.TempDir()
	orphan := pidPrefixedTestDir(t, root, SocketParentDirPrefix, nonLivePID(t))
	evidence := filepath.Join(orphan, "preserved.txt")
	if err := os.WriteFile(evidence, []byte("historical evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	backdatePastSweepAge(t, orphan)
	dir, sentinel, err := NewIsolatedSocketParentDir(root)
	if err != nil {
		t.Fatal(err)
	}
	defer sentinel.Close() //nolint:errcheck
	if got, err := os.ReadFile(evidence); err != nil || string(got) != "historical evidence" {
		t.Fatalf("creation changed historical sibling: bytes=%q err=%v", got, err)
	}
	if exists, held := aliveSentinelHeld(dir); !exists || !held {
		t.Fatalf("new socket parent sentinel = (%v,%v), want held", exists, held)
	}
}

func TestIsolatedSocketCleanupCannotSelectSibling(t *testing.T) {
	root := t.TempDir()
	dir, sentinel, err := NewIsolatedSocketParentDir(root)
	if err != nil {
		t.Fatal(err)
	}
	defer sentinel.Close() //nolint:errcheck
	active := filepath.Join(dir, "tmux")
	sibling := filepath.Join(root, "gct-preserved", "tmux")
	var paths []string
	for _, base := range []string{active, sibling} {
		path := filepath.Join(base, "tmux-"+strconv.Itoa(os.Getuid()), "gctest-preserved")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("socket selection fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-2 * tmuxSiblingSocketStaleAfter)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	t.Setenv(tmuxTmpEnv, active)
	if got := listTestSocketPaths(); !slices.Equal(got, paths[:1]) {
		t.Fatalf("cleanup selects %v, want only owned socket %v", got, paths[:1])
	}
	if got := tmuxSocketRootPatterns(active); len(got) != 0 {
		t.Fatalf("isolated namespace still enables sibling scanning: %v", got)
	}
}

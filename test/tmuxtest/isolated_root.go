package tmuxtest

import "os"

// NewIsolatedSocketParentDir creates a socket parent without touching siblings.
// The caller owns its cleanup and must retain the sentinel until then.
func NewIsolatedSocketParentDir(root string) (dir string, sentinel *os.File, err error) {
	// This prefix is deliberately outside the shared orphan-directory and
	// sibling-socket sweep namespaces. Cleanup may select only this run's root.
	dir, err = os.MkdirTemp(root, "gc-tmux-")
	if err != nil {
		return "", nil, err
	}
	sentinel, err = HoldAliveSentinel(dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, err
	}
	return dir, sentinel, nil
}

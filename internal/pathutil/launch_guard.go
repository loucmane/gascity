package pathutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ga-6umo: a session working directory can come from worker-writable bead
// metadata. It becomes the launched process's cwd (which codex workspace-write
// and claude treat as writable) and is substituted unquoted into pre_start,
// session_setup and session_live shell templates. These helpers bound it.

// SafeLaunchPath reports whether p is an absolute, clean path made only of
// [A-Za-z0-9._/-] characters, with no "..", no empty component and no ".git"
// component (compared case-insensitively). Such a path cannot inject shell
// syntax when substituted into a template.
func SafeLaunchPath(p string) bool {
	if p == "" || !filepath.IsAbs(p) || filepath.Clean(p) != p {
		return false
	}
	for _, r := range p {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-' || r == '/':
		default:
			return false
		}
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." || strings.EqualFold(part, ".git") {
			return false
		}
	}
	return true
}

func hasGitComponent(p string) bool {
	for _, part := range strings.Split(filepath.ToSlash(p), "/") {
		if strings.EqualFold(part, ".git") {
			return true
		}
	}
	return false
}

// ResolveExistingPrefix resolves symlinks on the longest existing prefix of p
// and re-appends the non-existent tail, so a path whose leaf does not exist yet
// is still compared by where it would really land.
func ResolveExistingPrefix(p string) (string, error) {
	p = filepath.Clean(p)
	tail := ""
	cur := p
	for {
		if _, err := os.Lstat(cur); err == nil {
			resolved, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return "", err
			}
			if tail == "" {
				return resolved, nil
			}
			return filepath.Join(resolved, tail), nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p, nil
		}
		if tail == "" {
			tail = filepath.Base(cur)
		} else {
			tail = filepath.Join(filepath.Base(cur), tail)
		}
		cur = parent
	}
}

// ContainedIn reports whether candidate equals root or lies under it, by path
// component (never a string prefix), after resolving symlinks on both sides.
func ContainedIn(candidate, root string) bool {
	if candidate == "" || root == "" {
		return false
	}
	c, err := ResolveExistingPrefix(candidate)
	if err != nil {
		return false
	}
	r, err := ResolveExistingPrefix(root)
	if err != nil {
		return false
	}
	if c == r {
		return true
	}
	rel, err := filepath.Rel(r, c)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, "../") && !filepath.IsAbs(rel)
}

// GuardLaunchWorkDir returns candidate when it is a SafeLaunchPath whose
// resolved location is contained in one of roots (or equals exactRoot), and
// ok=false otherwise. Callers fall back to their configured work_dir.
func GuardLaunchWorkDir(candidate string, roots []string, exactRoot string) (string, bool) {
	candidate = strings.TrimSpace(candidate)
	if !SafeLaunchPath(candidate) {
		return "", false
	}
	// A symlink inside a root may land in a .git directory; check where the
	// path really resolves too.
	if resolved, err := ResolveExistingPrefix(candidate); err != nil || hasGitComponent(resolved) {
		return "", false
	}
	if exactRoot != "" {
		c, err := ResolveExistingPrefix(candidate)
		if err == nil {
			if r, err := ResolveExistingPrefix(exactRoot); err == nil && c == r {
				return candidate, true
			}
		}
	}
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		if ContainedIn(candidate, root) {
			return candidate, true
		}
	}
	return "", false
}

package workdir

import (
	"path/filepath"
	"strings"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/pathutil"
)

// LaunchWorkDirRoots returns where a session working directory taken from bead
// or session metadata, which is worker-writable (ga-6umo), may point for agent
// a: under its configured work_dir or its pack parent, under the rig's session
// worktrees (<worktrees root>/<rig>) or under an explicit work_dir_roots entry.
// When the configured work_dir is the city root itself only that exact
// directory is allowed, never an arbitrary descendant of the city.
func LaunchWorkDirRoots(cityPath, cityName, qualifiedName string, a config.Agent, rigs []config.Rig) (roots []string, exactRoot string) {
	qualifiedName = SafeAgentIdentity(cityPath, a, rigs, qualifiedName)
	configured := ResolveWorkDirPath(cityPath, cityName, qualifiedName, a, rigs)
	if configured != "" && !UsableConfiguredRoot(cityPath, configured) {
		configured = ""
	}
	if configured != "" {
		if cityPath != "" && pathutil.SamePath(configured, cityPath) {
			exactRoot = configured
		} else {
			roots = append(roots, configured)
			if packRoot := PackSiblingRoot(cityPath, configured, rigs); packRoot != "" {
				roots = append(roots, packRoot)
			}
		}
	}
	if rig := rigNameForQualifiedAgent(cityPath, qualifiedName, a, rigs); rig != "" && cityPath != "" {
		roots = append(roots, filepath.Join(WorktreesRoot(cityPath), rig))
	}
	for _, root := range a.WorkDirRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		roots = append(roots, ResolveDirPath(cityPath, root))
	}
	return roots, exactRoot
}

// SafeIdentityName reports whether every segment of qualifiedName is a plain
// name ([A-Za-z0-9][A-Za-z0-9._-]*, never "." or ".."). A stored agent_name is
// worker-writable and expands work_dir and startup templates, so a value that
// fails this check must not be used as a session identity (ga-6umo).
func SafeIdentityName(qualifiedName string) bool {
	qualifiedName = strings.TrimSpace(qualifiedName)
	if qualifiedName == "" {
		return false
	}
	for _, seg := range strings.Split(qualifiedName, "/") {
		if !safeIdentitySegment(seg) {
			return false
		}
	}
	return true
}

// SafeAgentIdentity returns qualifiedName when it can safely expand the
// work_dir template of agent a: it passes SafeIdentityName and its directory
// part is the agent dir or the rig the agent dir resolves to. Otherwise it
// returns a.QualifiedName(), so a traversal such as "x/.." or another dir
// prefix never moves the work_dir or its roots (ga-6umo).
func SafeAgentIdentity(cityPath string, a config.Agent, rigs []config.Rig, qualifiedName string) string {
	qualifiedName = strings.TrimSpace(qualifiedName)
	if !SafeIdentityName(qualifiedName) {
		return a.QualifiedName()
	}
	dir := ""
	if i := strings.LastIndex(qualifiedName, "/"); i >= 0 {
		dir = qualifiedName[:i]
	}
	agentDir := strings.Trim(strings.TrimSpace(a.Dir), "/")
	if dir == agentDir {
		return qualifiedName
	}
	if rig := ConfiguredRigName(cityPath, a, rigs); rig != "" && dir == rig {
		return qualifiedName
	}
	return a.QualifiedName()
}

func safeIdentitySegment(seg string) bool {
	if seg == "" || seg == "." || seg == ".." {
		return false
	}
	for i, r := range seg {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case i > 0 && (r == '.' || r == '_' || r == '-'):
		default:
			return false
		}
	}
	return true
}

// UsableConfiguredRoot reports whether a configured work_dir may serve as a
// containment root: a safe launch path that is neither the city .gc
// directory nor a strict ancestor of the city root.
func UsableConfiguredRoot(cityPath, configured string) bool {
	if !pathutil.SafeLaunchPath(configured) {
		return false
	}
	if cityPath == "" {
		return true
	}
	if pathutil.SamePath(configured, filepath.Join(cityPath, ".gc")) {
		return false
	}
	return pathutil.SamePath(configured, cityPath) || !pathutil.ContainedIn(cityPath, configured)
}

// PackSiblingRoot returns the parent of the configured work_dir, where
// pack-routed work lands (<parent>/<gc.pack>[/<gc.pack_workspace>]), when that
// parent lies strictly inside the city and is neither the city root nor its
// .gc directory, and holds no configured rig root. Otherwise it returns "".
// The parent also holds sibling directories of other agents; that reach is a
// documented residual of the ga-6umo hotfix (full containment is ga-5eix),
// but a parent that holds a rig root (for example <city>/rigs for an agent
// whose work_dir is its rig root) would reach whole other rigs and is refused.
func PackSiblingRoot(cityPath, configured string, rigs []config.Rig) string {
	if cityPath == "" || configured == "" {
		return ""
	}
	parent := filepath.Dir(filepath.Clean(configured))
	if pathutil.SamePath(parent, cityPath) || pathutil.SamePath(parent, filepath.Join(cityPath, ".gc")) {
		return ""
	}
	if !pathutil.ContainedIn(parent, cityPath) {
		return ""
	}
	for _, rig := range rigs {
		if strings.TrimSpace(rig.Path) == "" {
			continue
		}
		if pathutil.ContainedIn(ResolveDirPath(cityPath, rig.Path), parent) {
			return ""
		}
	}
	return parent
}

// GuardMetadataWorkDir returns candidate when it is a safe launch path inside
// the roots LaunchWorkDirRoots allows for a, and ok=false otherwise. Callers
// keep the configured work_dir on refusal.
func GuardMetadataWorkDir(candidate, cityPath, cityName, qualifiedName string, a config.Agent, rigs []config.Rig) (string, bool) {
	roots, exact := LaunchWorkDirRoots(cityPath, cityName, qualifiedName, a, rigs)
	return pathutil.GuardLaunchWorkDir(candidate, roots, exact)
}

// SessionLaunchWorkDir returns the working directory to launch or resume a
// session whose stored work_dir is candidate. An empty candidate keeps the
// city root. A candidate outside the allowed roots of agent a is replaced by
// the configured work_dir. With no agent (a provider-kind session) only the
// exact city root is allowed. The bool reports whether candidate was refused.
func SessionLaunchWorkDir(candidate, cityPath string, cfg *config.City, a *config.Agent, qualifiedName string) (string, bool) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return cityPath, false
	}
	candidate = ResolveDirPath(cityPath, candidate)
	if a == nil {
		if wd, ok := pathutil.GuardLaunchWorkDir(candidate, nil, cityPath); ok {
			return wd, false
		}
		return cityPath, true
	}
	var rigs []config.Rig
	if cfg != nil {
		rigs = cfg.Rigs
	}
	qualifiedName = SafeAgentIdentity(cityPath, *a, rigs, qualifiedName)
	cityName := CityName(cityPath, cfg)
	if wd, ok := GuardMetadataWorkDir(candidate, cityPath, cityName, qualifiedName, *a, rigs); ok {
		return wd, false
	}
	// The configured work_dir is expanded from a safe identity of this agent,
	// so it is the fallback even when it cannot serve as a containment root
	// (for example a configured path with a space); the city root is used only
	// when nothing is configured.
	if configured := ResolveWorkDirPath(cityPath, cityName, qualifiedName, *a, rigs); configured != "" && !pathutil.SamePath(configured, filepath.Join(cityPath, ".gc")) {
		return configured, true
	}
	return cityPath, true
}

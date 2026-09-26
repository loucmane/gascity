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
	configured := ResolveWorkDirPath(cityPath, cityName, qualifiedName, a, rigs)
	if configured != "" {
		if cityPath != "" && pathutil.SamePath(configured, cityPath) {
			exactRoot = configured
		} else {
			roots = append(roots, configured)
			if packRoot := PackSiblingRoot(cityPath, configured); packRoot != "" {
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

// PackSiblingRoot returns the parent of the configured work_dir, where
// pack-routed work lands (<parent>/<gc.pack>[/<gc.pack_workspace>]), when that
// parent lies strictly inside the city and is neither the city root nor its
// .gc directory. Otherwise it returns "". The parent also holds sibling
// directories of other agents; that reach is a documented residual of the
// ga-6umo hotfix (full containment is ga-5eix).
func PackSiblingRoot(cityPath, configured string) string {
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
	if strings.TrimSpace(qualifiedName) == "" {
		qualifiedName = a.QualifiedName()
	}
	cityName := CityName(cityPath, cfg)
	if wd, ok := GuardMetadataWorkDir(candidate, cityPath, cityName, qualifiedName, *a, rigs); ok {
		return wd, false
	}
	if configured := ResolveWorkDirPath(cityPath, cityName, qualifiedName, *a, rigs); configured != "" {
		return configured, true
	}
	return cityPath, true
}

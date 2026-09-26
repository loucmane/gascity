package main

import (
	"io"
	"log"
	"path/filepath"
	"strings"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/gastownhall/gascity/internal/pathutil"
	"github.com/gastownhall/gascity/internal/session"
	workdirutil "github.com/gastownhall/gascity/internal/workdir"
)

// ga-6umo launch-parameter guard helpers for cmd/gc. See
// internal/config/launch_guard.go for the option classifier.

const boundedLogValueLimit = 256

// boundedLogValue truncates a worker-controlled value for logs and escapes
// control characters so a log line cannot be forged.
func boundedLogValue(v string) string {
	if len(v) > boundedLogValueLimit {
		v = v[:boundedLogValueLimit] + "…"
	}
	var b strings.Builder
	for _, r := range v {
		if r < 0x20 || r == 0x7f {
			b.WriteString("\\x")
			const hex = "0123456789abcdef"
			b.WriteByte(hex[r>>4&0xf])
			b.WriteByte(hex[r&0xf])
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// appendRuntimeProviderSettings adds the gc-managed Claude settings projection
// to a command rebuilt from config, matching the reconciler's template_resolve
// construction and buildResumeCommand. It replaces what the old stored-command
// preservation (#799) kept: rebuilt resume commands must not lose --settings.
func appendRuntimeProviderSettings(cityPath string, resolved *config.ResolvedProvider, command string) string {
	if resolved == nil || strings.TrimSpace(command) == "" || storedCommandHasSettingsArg(command) {
		return command
	}
	providerFamily := resolvedProviderLaunchFamily(resolved)
	if sa, err := ensureClaudeSettingsArgs(fsys.OSFS{}, cityPath, providerFamily, io.Discard); err == nil {
		if sa != "" {
			return command + " " + sa
		}
		return command
	}
	if probe := settingsArgsIfReadable(cityPath, providerFamily); probe != "" {
		return command + " " + probe
	}
	return command
}

// filterMetadataOptionOverridesLogged applies the metadata option filter and
// logs each ignored override once per call.
func filterMetadataOptionOverridesLogged(resolved *config.ResolvedProvider, sessionID string, overrides map[string]string) map[string]string {
	if resolved == nil || len(overrides) == 0 {
		return overrides
	}
	allowed, rejected := config.FilterMetadataOptionOverrides(resolved.OptionsSchema, overrides)
	for _, r := range rejected {
		log.Printf("session %s: ignoring metadata option %s=%q: %s (ga-6umo)", boundedLogValue(sessionID), r.Key, boundedLogValue(r.Value), r.Reason)
	}
	return allowed
}

// templateLaunchWorkDirRoots returns where a metadata-supplied working
// directory may point for a session built from tp: under its configured
// work_dir, under <worktrees root>/<rig>, or under the agent's work_dir_roots.
// A configured work_dir equal to the city root allows only that exact
// directory (ga-6umo).
func templateLaunchWorkDirRoots(cityPath string, cfg *config.City, tp TemplateParams) (roots []string, exactRoot string) {
	if configured := strings.TrimSpace(tp.WorkDir); configured != "" {
		if cityPath != "" && pathutil.SamePath(configured, cityPath) {
			exactRoot = configured
		} else {
			roots = append(roots, configured)
			if packRoot := workdirutil.PackSiblingRoot(cityPath, configured); packRoot != "" {
				roots = append(roots, packRoot)
			}
		}
	}
	if rig := strings.TrimSpace(tp.RigName); rig != "" && cityPath != "" {
		roots = append(roots, filepath.Join(workdirutil.WorktreesRoot(cityPath), rig))
	}
	if a := findAgentByTemplate(cfg, tp.TemplateName); a != nil {
		for _, root := range a.WorkDirRoots {
			if root = strings.TrimSpace(root); root != "" {
				roots = append(roots, workdirutil.ResolveDirPath(cityPath, root))
			}
		}
	}
	return roots, exactRoot
}

// guardedSessionInfoWorkDir returns the working directory for a session
// resumed from its bead: the stored work_dir when it lies inside the allowed
// roots of the session template, else the configured work_dir (ga-6umo).
func guardedSessionInfoWorkDir(cityPath string, cfg *config.City, info session.Info) string {
	a := findAgentByTemplate(cfg, info.Template)
	wd, refused := workdirutil.SessionLaunchWorkDir(info.WorkDir, cityPath, cfg, a, info.AgentName)
	if refused {
		log.Printf("session %s: ignoring stored work_dir %q: outside the allowed roots or not a safe path (ga-6umo)", boundedLogValue(info.ID), boundedLogValue(info.WorkDir))
	}
	return wd
}

// guardTemplateLaunchWorkDir returns candidate when it may be used as the
// working directory of a session built from tp. Otherwise it logs and returns
// ok=false so the caller keeps the configured work_dir.
func guardTemplateLaunchWorkDir(cityPath string, cfg *config.City, tp TemplateParams, sessionID, source, candidate string) (string, bool) {
	roots, exact := templateLaunchWorkDirRoots(cityPath, cfg, tp)
	if wd, ok := pathutil.GuardLaunchWorkDir(candidate, roots, exact); ok {
		return wd, true
	}
	log.Printf("session %s: ignoring %s work_dir %q: outside the allowed roots or not a safe path (ga-6umo)", boundedLogValue(sessionID), source, boundedLogValue(candidate))
	return "", false
}

package session

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/pathutil"
)

// IsPoolManaged reports whether the persisted identity denotes a pool session,
// including the historical numbered identities used before explicit markers.
func IsPoolManaged(info Info) bool {
	if info.PoolManaged || strings.TrimSpace(info.PoolSlot) != "" {
		return true
	}
	if origin := strings.TrimSpace(info.SessionOrigin); origin != "" {
		return origin == "ephemeral"
	}
	if info.ConfiguredNamedSession || info.ManualSessionMetadata == "true" {
		return false
	}
	if info.DependencyOnly {
		return true
	}
	agentName := info.AgentName
	if agentName == "" {
		for _, label := range info.Labels {
			if strings.HasPrefix(label, "agent:") {
				agentName = strings.TrimPrefix(label, "agent:")
				break
			}
		}
	}
	template := strings.TrimSpace(info.Template)
	if template != "" {
		for _, name := range []string{agentName, info.SessionNameMetadata} {
			if suffix, ok := strings.CutPrefix(strings.TrimSpace(name), template+"-"); ok {
				slot, _ := strconv.Atoi(strings.TrimPrefix(suffix, "gc-"))
				if slot > 0 {
					return true
				}
			}
		}
	}
	return false
}

// ClaimWorktreeCompatible is the ga-g189 workspace selection rule shared by
// selection and the atomic publication fence (ga-gwav). Only fully managed
// worktrees restrict pool sessions; triggers remain scheduling hints.
func ClaimWorktreeCompatible(info Info, work beads.Bead) bool {
	workDir := strings.TrimSpace(work.Metadata[beadmeta.WorkDirMetadataKey])
	for _, key := range []string{
		beadmeta.WorkDirMetadataKey, beadmeta.WorkBranchMetadataKey,
		beadmeta.WorktreeRepoMetadataKey, beadmeta.WorktreeRootMetadataKey,
		beadmeta.WorktreeBaseRefMetadataKey, beadmeta.WorktreeBaseSHAMetadataKey,
		beadmeta.WorktreeCreatorMetadataKey, beadmeta.WorktreeOwnerMetadataKey,
		beadmeta.WorktreeGenerationMetadataKey, beadmeta.WorktreeLifecycleMetadataKey,
	} {
		if strings.TrimSpace(work.Metadata[key]) == "" {
			return true
		}
	}
	if !IsPoolManaged(info) {
		return true
	}
	sessionDir := strings.TrimSpace(info.WorkDir)
	return filepath.IsAbs(workDir) && filepath.IsAbs(sessionDir) && pathutil.SamePath(workDir, sessionDir)
}

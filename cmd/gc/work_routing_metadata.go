package main

import (
	"strings"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
)

// isGraphV2WorkflowRoot reports whether b is a graph.v2 workflow root. The root
// is controller-owned (its workflow-finalize control closes it), yet it carries
// its pool's gc.routed_to and, behind its non-blocking tracks edge to the
// finalizer, stays Ready() while the workflow runs. Claim and pool-demand
// readers skip it; legacy workflow roots without the graph.v2 contract are
// unaffected.
func isGraphV2WorkflowRoot(b beads.Bead) bool {
	return strings.TrimSpace(b.Metadata[beadmeta.KindMetadataKey]) == beadmeta.KindWorkflow &&
		strings.TrimSpace(b.Metadata[beadmeta.FormulaContractMetadataKey]) == beadmeta.FormulaContractGraphV2
}

func legacyWorkflowRunTarget(b beads.Bead) string {
	if strings.TrimSpace(b.Metadata[beadmeta.KindMetadataKey]) != beadmeta.KindWorkflow {
		return ""
	}
	if strings.TrimSpace(b.Metadata[beadmeta.RoutedToMetadataKey]) != "" {
		return ""
	}
	return strings.TrimSpace(b.Metadata[beadmeta.RunTargetMetadataKey])
}

func routedToOrLegacyWorkflowTarget(b beads.Bead) string {
	if routedTo := strings.TrimSpace(b.Metadata[beadmeta.RoutedToMetadataKey]); routedTo != "" {
		return routedTo
	}
	return legacyWorkflowRunTarget(b)
}

func routedToAndLegacyWorkflowCandidates(b beads.Bead) []string {
	routedTo := strings.TrimSpace(b.Metadata[beadmeta.RoutedToMetadataKey])
	legacy := legacyWorkflowRunTarget(b)
	if routedTo == "" {
		if legacy == "" {
			return nil
		}
		return []string{legacy}
	}
	return []string{routedTo}
}

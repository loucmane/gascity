// Incarnation-addressed drain acknowledgements are inert on recycled runtimes.
package main

import (
	"strings"

	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/session"
)

const drainAckIncarnationPrefix = runtime.DrainAckIncarnationPrefix

// The acknowledgement itself carries its author. Separate source/requester
// keys are useful diagnostics but cannot authenticate a write across name
// recycling: any of them can belong to a different occupant.
func verifiedDrainAckValue(v *VerifiedSession) string {
	return drainAckIncarnationPrefix + v.workIncarnation()
}

func drainAckIncarnationMatches(value, generation, token string) bool {
	return runtime.DrainAckMatchesIncarnation(value, generation, token)
}

func providerDrainAckValueMatches(sp runtime.Provider, name, value string) (bool, error) {
	return runtime.DrainAckMatchesRuntime(sp, name, value)
}

// Both controller stop paths require the acknowledgement to name the row AND
// runtime they are considering. Legacy acknowledgements retain their operator
// semantics, but a legacy agent ack with an explicit mismatched requester is
// rejected too. Missing/read-error/malformed addressed acks never authorize stop.
func controllerDrainAckMatches(info session.Info, sp runtime.Provider, name string) bool {
	value, err := sp.GetMeta(name, "GC_DRAIN_ACK")
	if err != nil {
		return false
	}
	if strings.HasPrefix(value, drainAckIncarnationPrefix) {
		if !drainAckIncarnationMatches(value, info.Generation, info.InstanceToken) {
			return false
		}
		matched, err := providerDrainAckValueMatches(sp, name, value)
		return err == nil && matched
	}
	source, err := sp.GetMeta(name, reconcilerDrainAckSourceKey)
	if err != nil {
		return false
	}
	if source == drainAckSourceAgentValue {
		requester, err := sp.GetMeta(name, drainAckRequesterInstanceTokenKey)
		if err != nil {
			return false
		}
		if requester != "" {
			token, err := sp.GetMeta(name, "GC_INSTANCE_TOKEN")
			return err == nil && requester == drainAckInstanceTokenDigest(info.InstanceToken) &&
				requester == drainAckInstanceTokenDigest(token)
		}
	}
	return true
}

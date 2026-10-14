// Incarnation-addressed drain acknowledgements are inert on recycled runtimes.
package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/session"
)

const drainAckIncarnationPrefix = "incarnation:"

// The acknowledgement itself carries its author. Separate source/requester
// keys are useful diagnostics but cannot authenticate a write across name
// recycling: any of them can belong to a different occupant.
func verifiedDrainAckValue(v *VerifiedSession) string {
	return drainAckIncarnationPrefix + v.workIncarnation()
}

func drainAckIncarnationMatches(value, generation, token string) bool {
	n, err := strconv.Atoi(generation)
	return err == nil && n > 0 && token != "" &&
		value == fmt.Sprintf("%s%d:%s", drainAckIncarnationPrefix, n, drainAckInstanceTokenDigest(token))
}

func providerDrainAckValueMatches(sp runtime.Provider, name, value string) (bool, error) {
	if !strings.HasPrefix(value, drainAckIncarnationPrefix) {
		return value == "1", nil
	}
	token, err := sp.GetMeta(name, "GC_INSTANCE_TOKEN")
	if err != nil {
		return false, err
	}
	generation, err := sp.GetMeta(name, "GC_RUNTIME_EPOCH")
	if err != nil {
		return false, err
	}
	return drainAckIncarnationMatches(value, generation, token), nil
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

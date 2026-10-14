package main

import (
	"fmt"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
)

func TestControllerDrainAckCannotStopSuccessor(t *testing.T) {
	for _, desired := range []bool{false, true} {
		for _, kind := range []string{"addressed-token", "addressed-generation", "runtime-mismatch", "legacy-requester"} {
			t.Run(fmt.Sprintf("desired=%v/%s", desired, kind), func(t *testing.T) {
				e := newReconcilerTestEnv()
				e.cfg = &config.City{Agents: []config.Agent{{Name: "worker"}}}
				e.addDesired("worker", "worker", true)
				if !desired {
					delete(e.desiredState, "worker")
				}
				row := e.createSessionBead("worker", "worker")
				e.markSessionActive(&row)
				e.setSessionMetadata(&row, map[string]string{"generation": "2", "instance_token": "successor"})
				mustSetMeta(t, e.sp, "worker", "GC_INSTANCE_TOKEN", "successor")
				mustSetMeta(t, e.sp, "worker", "GC_RUNTIME_EPOCH", "2")
				mustSetMeta(t, e.sp, "worker", reconcilerDrainAckSourceKey, drainAckSourceAgentValue)
				// Even current-looking auxiliary keys cannot authenticate an old
				// addressed acknowledgement: the ack value carries its author.
				mustSetMeta(t, e.sp, "worker", drainAckRequesterInstanceTokenKey, drainAckInstanceTokenDigest("successor"))
				ack := drainAckIncarnationPrefix + "2:" + drainAckInstanceTokenDigest("old")
				switch kind {
				case "addressed-generation":
					ack = drainAckIncarnationPrefix + "1:" + drainAckInstanceTokenDigest("successor")
				case "runtime-mismatch":
					ack = drainAckIncarnationPrefix + "2:" + drainAckInstanceTokenDigest("successor")
					mustSetMeta(t, e.sp, "worker", "GC_INSTANCE_TOKEN", "later-runtime")
				case "legacy-requester":
					ack = "1"
					mustSetMeta(t, e.sp, "worker", drainAckRequesterInstanceTokenKey, drainAckInstanceTokenDigest("old"))
				}
				mustSetMeta(t, e.sp, "worker", "GC_DRAIN_ACK", ack)
				dops := &providerDrainOps{sp: e.sp}
				e.reconcileWithPoolDesiredAndDrainOps([]beads.Bead{row}, nil, dops)
				got, _ := e.store.Get(row.ID)
				if got.Status == "closed" || isDrainAckStopPendingInfo(e.sessionInfo(row.ID)) || !e.sp.IsRunning("worker") {
					t.Fatalf("stale ack stopped successor: status=%s state=%s reason=%s stderr=%s", got.Status, got.Metadata["state"], got.Metadata["state_reason"], &e.stderr)
				}
				for _, call := range e.sp.SnapshotCalls() {
					if call.Method == "Stop" {
						t.Fatal("controller stopped successor")
					}
				}
			})
		}
	}
}

func TestControllerDrainAckAcceptsCurrentIncarnationOnly(t *testing.T) {
	e := newReconcilerTestEnv()
	e.addDesired("worker", "worker", true)
	row := e.createSessionBead("worker", "worker")
	info := e.sessionInfo(row.ID)
	ack := drainAckIncarnationPrefix + "1:" + drainAckInstanceTokenDigest(info.InstanceToken)
	mustSetMeta(t, e.sp, "worker", "GC_INSTANCE_TOKEN", info.InstanceToken)
	mustSetMeta(t, e.sp, "worker", "GC_RUNTIME_EPOCH", info.Generation)
	mustSetMeta(t, e.sp, "worker", "GC_DRAIN_ACK", ack)
	dops := &providerDrainOps{sp: e.sp}
	if ok, err := dops.isDrainAcked("worker"); err != nil || !ok || !controllerDrainAckMatches(info, e.sp, "worker") {
		t.Fatalf("own addressed ack refused: %v %v", ok, err)
	}
	// The row check is independent of the runtime check, including generation.
	info.Generation = "2"
	if controllerDrainAckMatches(info, e.sp, "worker") {
		t.Fatal("controller trusted runtime over successor row")
	}
	for _, bad := range []string{drainAckIncarnationPrefix, drainAckIncarnationPrefix + "0:bad", drainAckIncarnationPrefix + "1:bad"} {
		mustSetMeta(t, e.sp, "worker", "GC_DRAIN_ACK", bad)
		if ok, _ := dops.isDrainAcked("worker"); ok {
			t.Fatal("malformed ack admitted")
		}
	}
}

func TestControllerDrainAckReadErrorsNeverAuthorizeStop(t *testing.T) {
	for _, key := range []string{"GC_DRAIN_ACK", "GC_INSTANCE_TOKEN", "GC_RUNTIME_EPOCH", reconcilerDrainAckSourceKey, drainAckRequesterInstanceTokenKey} {
		t.Run(key, func(t *testing.T) {
			e := newReconcilerTestEnv()
			e.addDesired("worker", "worker", true)
			row := e.createSessionBead("worker", "worker")
			info := e.sessionInfo(row.ID)
			ack := drainAckIncarnationPrefix + "1:" + drainAckInstanceTokenDigest(info.InstanceToken)
			if key == reconcilerDrainAckSourceKey || key == drainAckRequesterInstanceTokenKey {
				ack = "1"
			}
			mustSetMeta(t, e.sp, "worker", "GC_DRAIN_ACK", ack)
			mustSetMeta(t, e.sp, "worker", "GC_INSTANCE_TOKEN", info.InstanceToken)
			mustSetMeta(t, e.sp, "worker", "GC_RUNTIME_EPOCH", info.Generation)
			mustSetMeta(t, e.sp, "worker", reconcilerDrainAckSourceKey, drainAckSourceAgentValue)
			sp := &ackBindingUnreadableProvider{Fake: e.sp, key: key}
			if controllerDrainAckMatches(info, sp, "worker") {
				t.Fatal("unreadable acknowledgement authorized stop")
			}
		})
	}
}

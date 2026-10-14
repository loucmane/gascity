package t3bridge

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Use the real SetMeta path and native metadata store, with a cached snapshot
// and a refusal-only transport. No socket is opened; RPC attempts are counted
// separately from the durable drained flag used by Stop's archive path.
func TestRound4DT3DrainAckMatchesRuntime(t *testing.T) {
	ack := fmt.Sprintf("incarnation:2:%x", sha256.Sum256([]byte("current")))
	for _, tc := range []struct {
		name, value, token, epoch string
		want                      bool
	}{
		{"legacy", "1", "", "", true},
		{"current", ack, "current", "2", true},
		{"wrong-token", ack, "later", "2", false},
		{"wrong-generation", ack, "current", "3", false},
		{"missing-token", ack, "", "2", false},
		{"missing-epoch", ack, "current", "", false},
		{"malformed", "incarnation:bad", "current", "2", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetBridgeAuthCacheForTest(t)
			t.Setenv("GC_T3BRIDGE_STATE_DIR", t.TempDir())
			t.Setenv("T3_HOME", t.TempDir())
			t.Setenv("T3_WS_URL", "ws://fixture.invalid")
			t.Setenv("T3_BEARER_TOKEN", "fixture-bearer")
			oldDefaults, oldDialer := defaultWSURLCandidates, websocket.DefaultDialer
			defaultWSURLCandidates = nil
			dialer := *websocket.DefaultDialer
			dialer.NetDialContext = func(context.Context, string, string) (net.Conn, error) {
				return nil, errors.New("fixture RPC disabled")
			}
			websocket.DefaultDialer = &dialer
			t.Cleanup(func() { defaultWSURLCandidates, websocket.DefaultDialer = oldDefaults, oldDialer })
			p := &Provider{snapshotCacheAt: time.Now().Add(time.Hour), snapshotSnapshot: map[string]interface{}{
				"threads": []interface{}{map[string]interface{}{
					"id": "thread-1", "projectId": "project-1", "customMetadata": map[string]interface{}{"gc.sessionName": "worker"},
				}},
			}}
			if err := writeMetaValue("worker", "GC_INSTANCE_TOKEN", tc.token); err != nil {
				t.Fatal(err)
			}
			if err := writeMetaValue("worker", "GC_RUNTIME_EPOCH", tc.epoch); err != nil {
				t.Fatal(err)
			}
			if err := p.SetMeta("worker", "GC_DRAIN_ACK", tc.value); err != nil {
				t.Fatal(err)
			}
			drained, err := p.GetMeta("worker", "drained")
			if err != nil {
				t.Fatal(err)
			}
			if got := drained == "1"; got != tc.want {
				t.Fatalf("drained=%v, want %v", got, tc.want)
			}
			// Command IDs and RPC attempts share this sequence. We assert
			// reflection was requested, not that a remote server applied it.
			if requested := p.reqSeq > 0; requested != tc.want {
				t.Fatalf("state reflection requested=%v, want %v", requested, tc.want)
			}
		})
	}
}

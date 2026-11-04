package t3bridge

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/platforminstall"
	"github.com/gastownhall/gascity/internal/providerallowlist"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gorilla/websocket"
)

// A turn-based transport must refuse delegated launches before any RPC, even
// when it would reuse a thread instead of consuming the verified command.
func TestDelegatedProviderRefusesT3Launch(t *testing.T) {
	for _, mode := range []string{"fresh", "reuse", "resume", "fallback", "nudge"} {
		for _, source := range []string{"config", "envelope", "context", "legacy-city"} {
			t.Run(mode+"/"+source, func(t *testing.T) {
				city := t.TempDir()
				writeT3ApprovalManifest(t, city, true)
				p, cfg := approvalLaunchFixture(t, city, mode, source)
				var err error
				if mode == "nudge" {
					err = p.Nudge("approval-session", []runtime.ContentBlock{{Type: "text", Text: "continue"}})
				} else {
					err = p.Start(context.Background(), "approval-session", cfg)
				}
				if err == nil || !strings.Contains(err.Error(), "t3bridge") || !strings.Contains(err.Error(), "provider allowlist") {
					t.Errorf("wanted explicit delegated T3 refusal, got %v", err)
				}
				if p.reqSeq != 0 {
					t.Errorf("delegated launch reached %d RPC calls", p.reqSeq)
				}
			})
		}
	}
}

func TestUndelegatedT3LaunchStillReachesRPC(t *testing.T) {
	for _, manifest := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "undelegated"}[manifest], func(t *testing.T) {
			city := t.TempDir()
			if manifest {
				writeT3ApprovalManifest(t, city, false)
			}
			for _, mode := range []string{"fresh", "reuse", "resume", "fallback", "nudge"} {
				t.Run(mode, func(t *testing.T) {
					p, cfg := approvalLaunchFixture(t, city, mode, "config")
					if mode == "nudge" {
						_ = p.Nudge("approval-session", []runtime.ContentBlock{{Type: "text", Text: "continue"}})
					} else {
						_ = p.Start(context.Background(), "approval-session", cfg)
					}
					if p.reqSeq == 0 {
						t.Fatal("legacy launch no longer reaches RPC")
					}
				})
			}
		})
	}
}

func approvalLaunchFixture(t *testing.T, city, mode, source string) (*Provider, runtime.Config) {
	t.Helper()
	// A recording transport failure makes the old launch observable without a
	// listener, credentials, network connection, or a real T3 service.
	t.Setenv("T3_BEARER_TOKEN", "")
	t.Setenv("T3_WS_URL", "ws://127.0.0.1:1/ws")
	t.Setenv("GC_T3BRIDGE_STATE_DIR", t.TempDir())
	previous := websocket.DefaultDialer
	websocket.DefaultDialer = &websocket.Dialer{NetDialContext: func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("approval fixture transport stop")
	}}
	t.Cleanup(func() { websocket.DefaultDialer = previous })
	envelope := StartupEnvelope{
		GC:      GCSection{Agent: "worker", Template: "worker", SessionName: "approval-session"},
		Runtime: RuntimeSection{Provider: "codex", Model: "fixture", WorkDir: city},
		Startup: StartupSection{StartupPrompt: "start"},
		Resume:  ResumeSection{Policy: "fallback", AllowThreadReuse: mode != "fresh", RequiredThreadProvider: "codex", RequiredThreadModel: "fixture"},
	}
	cfg := runtime.Config{WorkDir: city, Command: "/approved/codex", Env: map[string]string{}}
	switch source {
	case "config":
		cfg.Env["GC_CITY_PATH"] = city
	case "envelope":
		envelope.GC.CityPath = city
	case "context":
		envelope.Context.GCEnv = map[string]string{"GC_CITY_PATH": city}
	case "legacy-city":
		cfg.Env["GC_CITY"] = city
	}
	if mode == "resume" {
		cfg.Command += " resume fixture-session"
	}
	if mode == "fallback" {
		envelope.Resume.RequiredThreadProvider = "claudeAgent"
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Env["GC_STARTUP_ENVELOPE"] = string(data)
	meta := buildGCMetadata(envelope, "codex", map[string]string{"GC_CITY_PATH": city})
	thread := map[string]interface{}{
		"id": "thread-approval", "projectId": "project-approval", "customMetadata": meta,
		"session": map[string]interface{}{"status": "ready"},
	}
	snapshot := map[string]interface{}{"projects": []interface{}{map[string]interface{}{"id": "project-approval", "workspaceRoot": city}}}
	if mode != "fresh" {
		snapshot["threads"] = []interface{}{thread}
	}
	p := &Provider{watchers: make(map[string]context.CancelFunc), recentStarts: make(map[string]time.Time)}
	p.cacheSnapshot(snapshot)
	t.Cleanup(func() { p.stopEventWatcher("approval-session") })
	return p, cfg
}

func writeT3ApprovalManifest(t *testing.T, city string, delegated bool) {
	t.Helper()
	manifest := platforminstall.Manifest{
		Schema: platforminstall.ManifestSchemaV1, ReleaseID: "approval-fixture", CityPath: city,
		Core:           platforminstall.Artifact{Name: "gc", Source: filepath.Join(city, "candidate"), Destination: filepath.Join(city, "gc"), SHA256: strings.Repeat("a", 64), Mode: 0o755},
		PreviousSHA256: strings.Repeat("b", 64), BackupPath: filepath.Join(city, "backup"), ReceiptPath: filepath.Join(city, "receipt"),
	}
	if delegated {
		manifest.Integrity = &platforminstall.IntegritySpec{ProviderAllowlist: &providerallowlist.Reference{
			Path: providerallowlist.Path(city), Bindings: []providerallowlist.Binding{{Provider: "codex", Path: filepath.Join(city, "codex")}},
		}}
	}
	data, err := platforminstall.MarshalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	_, data, err = platforminstall.FinalizeManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	path := platforminstall.DefaultManifestPath(city)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

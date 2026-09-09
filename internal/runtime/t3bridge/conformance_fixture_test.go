//go:build integration

package t3bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
)

// The no-argument production constructor performs legacy-state cleanup. Own
// that namespace before any test constructs it, not in a later Config helper.
// This is an integration-only protocol fixture: no T3 process or model runs.
var t3ContractServer *t3ContractProtocol

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "gascity-t3-conformance-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for key, value := range map[string]string{
		"HOME": root, "T3_HOME": filepath.Join(root, ".t3"),
		"GC_EXEC_STATE_DIR":     filepath.Join(root, "legacy"),
		"GC_T3BRIDGE_STATE_DIR": filepath.Join(root, "native"),
		"T3_BEARER_TOKEN":       "synthetic-contract-token",
	} {
		if err := os.Setenv(key, value); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	t3ContractServer = &t3ContractProtocol{
		projects: make(map[string]map[string]interface{}),
		threads:  make(map[string]map[string]interface{}),
	}
	server := httptest.NewServer(http.HandlerFunc(t3ContractServer.serveHTTP))
	t3TestDialTargets.Store(server.Listener.Addr().String(), true)
	if err := os.Setenv("T3_WS_URL", "ws"+strings.TrimPrefix(server.URL, "http")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	previous := websocket.DefaultDialer
	dialer := *previous
	dialer.Proxy, dialer.NetDial, dialer.NetDialTLSContext = nil, nil, nil
	// Only listeners actually created by this package's fixtures are dialable.
	// Stale configured URLs and fallback ports cannot reach host services.
	dialer.NetDialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if _, owned := t3TestDialTargets.Load(address); !owned {
			return nil, fmt.Errorf("T3 test fixture refuses address %q", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	websocket.DefaultDialer = &dialer
	code := m.Run()
	websocket.DefaultDialer = previous
	server.Close()
	t3TestDialTargets.Delete(server.Listener.Addr().String())
	for _, violation := range t3ContractServer.violations {
		fmt.Fprintln(os.Stderr, violation)
		code = 1
	}
	if err := os.RemoveAll(root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}

// t3ContractProtocol models only the actual wire operations used by the full
// Provider suite. Unsupported operations fail instead of succeeding silently.
// Each reply is serialized under the same lock as command application, so
// concurrent provider calls never observe test-owned shared maps directly.
type t3ContractProtocol struct {
	mu          sync.Mutex
	projects    map[string]map[string]interface{}
	threads     map[string]map[string]interface{}
	commands    []string
	rejectState string
	violations  []error
}

func (s *t3ContractProtocol) serveHTTP(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	var request struct {
		ID      string                 `json:"id"`
		Tag     string                 `json:"tag"`
		Payload map[string]interface{} `json:"payload"`
	}
	if err := conn.ReadJSON(&request); err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var value map[string]interface{}
	switch request.Tag {
	case "orchestration.getSnapshot":
		projects, threads := []interface{}{}, []interface{}{}
		for _, project := range s.projects {
			projects = append(projects, project)
		}
		for _, thread := range s.threads {
			threads = append(threads, thread)
		}
		value = map[string]interface{}{"projects": projects, "threads": threads}
	case "orchestration.dispatchCommand":
		err = s.dispatch(request.Payload)
		value = map[string]interface{}{}
	case "gc.peekThreadMessages":
		value = map[string]interface{}{"results": []interface{}{}}
	default:
		err = fmt.Errorf("unsupported contract RPC %q", request.Tag)
		s.violations = append(s.violations, err)
	}
	exit := map[string]interface{}{"_tag": "Success", "value": value}
	if err != nil {
		exit = map[string]interface{}{"_tag": "Failure", "cause": err.Error()}
	}
	_ = conn.WriteJSON(map[string]interface{}{"_tag": "Exit", "requestId": request.ID, "exit": exit})
}

func (s *t3ContractProtocol) dispatch(command map[string]interface{}) error {
	if wrapped, ok := command["command"].(map[string]interface{}); ok {
		command = wrapped
	}
	typ, _ := command["type"].(string)
	id, _ := command["threadId"].(string)
	thread := s.threads[id]
	s.commands = append(s.commands, typ)
	switch typ {
	case "project.create":
		projectID, _ := command["projectId"].(string)
		if projectID == "" || s.projects[projectID] != nil {
			return fmt.Errorf("invalid duplicate project")
		}
		s.projects[projectID] = map[string]interface{}{"id": projectID, "workspaceRoot": command["workspaceRoot"]}
	case "thread.create":
		projectID, _ := command["projectId"].(string)
		if id == "" || thread != nil || s.projects[projectID] == nil {
			return fmt.Errorf("invalid thread creation")
		}
		s.threads[id] = map[string]interface{}{
			"id": id, "projectId": projectID, "title": command["title"],
			"customMetadata": command["customMetadata"], "session": map[string]interface{}{"status": "none"},
		}
	case "thread.meta.update", "thread.activity.append", "thread.turn.start", "thread.turn.interrupt", "thread.session.stop", "thread.archive":
		if thread == nil {
			return fmt.Errorf("unknown contract thread %q", id)
		}
		switch typ {
		case "thread.meta.update":
			updates, _ := command["customMetadata"].(map[string]interface{})
			if s.rejectState != "" && updates["gc.state"] == s.rejectState {
				return fmt.Errorf("synthetic rejection of state %s", s.rejectState)
			}
			meta, _ := thread["customMetadata"].(map[string]interface{})
			if meta == nil {
				meta = make(map[string]interface{})
				thread["customMetadata"] = meta
			}
			for key, value := range updates {
				meta[key] = value
			}
		case "thread.turn.start":
			thread["session"] = map[string]interface{}{"status": "running"}
		case "thread.turn.interrupt":
			thread["session"] = map[string]interface{}{"status": "ready"}
		case "thread.session.stop":
			thread["session"] = map[string]interface{}{"status": "stopped"}
		case "thread.archive":
			thread["deletedAt"] = "synthetic-archive"
		}
	default:
		err := fmt.Errorf("unsupported contract command %q", typ)
		s.violations = append(s.violations, err)
		return err
	}
	return nil
}

func (s *t3ContractProtocol) threadFor(t *testing.T, name string) map[string]interface{} {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, thread := range s.threads {
		if SessionNameFromMetadata(threadCustomMetadata(thread)) == name {
			data, err := json.Marshal(thread)
			if err != nil {
				t.Fatal(err)
			}
			var copy map[string]interface{}
			if err := json.Unmarshal(data, &copy); err != nil {
				t.Fatal(err)
			}
			return copy
		}
	}
	t.Fatalf("fixture has no thread for %q", name)
	return nil
}

func (s *t3ContractProtocol) commandCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.commands)
}

func (s *t3ContractProtocol) refuseState(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rejectState = state
}

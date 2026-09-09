//go:build integration

// Package k8sfixture serves the credential-free Kubernetes protocol used by runtime conformance tests.
package k8sfixture

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/gastownhall/gascity/internal/testutil"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/httpstream/spdy"
	protocol "k8s.io/apimachinery/pkg/util/remotecommand"
	"k8s.io/client-go/kubernetes/scheme"
)

// This is a remote protocol fixture, not a runtime.Provider implementation.
// It accepts only the pod API and exact exec command families exercised by the
// real adapter. Unsupported traffic fails the containing test process, including
// commands whose errors the production facade intentionally treats best-effort.
// Fixture implements only the pod/exec protocol required by the shared runtime contract.
type Fixture struct {
	mu                      sync.Mutex
	pods                    map[string]*corev1.Pod
	meta                    map[string]map[string]string
	problems                []string
	created, deleted, execs int
	streams                 sync.WaitGroup
}

// New creates a fresh in-memory protocol endpoint, never a production cluster client.
func New() *Fixture {
	return &Fixture{pods: make(map[string]*corev1.Pod), meta: make(map[string]map[string]string)}
}

func (f *Fixture) problem(message string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.problems = append(f.problems, message)
}

func fixtureJSON(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}

func fixtureStatus(w http.ResponseWriter, code int, reason metav1.StatusReason) {
	fixtureJSON(w, code, &metav1.Status{
		TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"},
		Status:   metav1.StatusFailure, Code: int32(code), Reason: reason,
	})
}

// ServeHTTP enforces the synthetic pod and exec protocol, recording every unsupported request.
func (f *Fixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const prefix = "/api/v1/namespaces/conformance/pods"
	if r.Header.Get("Authorization") != "" {
		f.problem("unexpected authorization header")
		fixtureStatus(w, http.StatusForbidden, metav1.StatusReasonForbidden)
		return
	}
	if r.URL.Path != prefix && !strings.HasPrefix(r.URL.Path, prefix+"/") {
		f.problem("unexpected API path: " + r.URL.Path)
		fixtureStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound)
		return
	}
	name := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if pod, exec := strings.CutSuffix(name, "/exec"); exec && pod != "" {
		f.serveExec(w, r, pod)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && name == "":
		labelSelector, labelErr := labels.Parse(r.URL.Query().Get("labelSelector"))
		fieldSelector, fieldErr := fields.ParseSelector(r.URL.Query().Get("fieldSelector"))
		if labelErr != nil || fieldErr != nil {
			f.problems = append(f.problems, "invalid selector")
			fixtureStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest)
			return
		}
		result := &corev1.PodList{TypeMeta: metav1.TypeMeta{Kind: "PodList", APIVersion: "v1"}, Items: []corev1.Pod{}}
		for _, pod := range f.pods {
			if labelSelector.Matches(labels.Set(pod.Labels)) && fieldSelector.Matches(fields.Set{"status.phase": string(pod.Status.Phase)}) {
				result.Items = append(result.Items, *pod.DeepCopy())
			}
		}
		fixtureJSON(w, http.StatusOK, result)
	case r.Method == http.MethodPost && name == "":
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			f.problems = append(f.problems, "cannot read pod request")
			fixtureStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest)
			return
		}
		object, _, err := scheme.Codecs.UniversalDeserializer().Decode(body, nil, nil)
		pod, ok := object.(*corev1.Pod)
		if err != nil || !ok || pod.Name == "" || pod.Namespace != "conformance" || pod.Labels["gc-session"] == "" {
			f.problems = append(f.problems, fmt.Sprintf("invalid pod request: %v", err))
			fixtureStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest)
			return
		}
		if f.pods[pod.Name] != nil {
			fixtureStatus(w, http.StatusConflict, metav1.StatusReasonAlreadyExists)
			return
		}
		pod = pod.DeepCopy()
		pod.Status.Phase = corev1.PodRunning
		pod.CreationTimestamp = metav1.Now()
		f.pods[pod.Name] = pod
		f.meta[pod.Name] = make(map[string]string)
		f.created++
		fixtureJSON(w, http.StatusCreated, pod)
	case r.Method == http.MethodGet && name != "":
		if pod := f.pods[name]; pod != nil {
			fixtureJSON(w, http.StatusOK, pod)
		} else {
			fixtureStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound)
		}
	case r.Method == http.MethodDelete && name != "":
		if f.pods[name] == nil {
			fixtureStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound)
			return
		}
		delete(f.pods, name)
		delete(f.meta, name)
		f.deleted++
		fixtureJSON(w, http.StatusOK, &metav1.Status{TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"}, Status: metav1.StatusSuccess})
	default:
		f.problems = append(f.problems, "unexpected API method: "+r.Method)
		fixtureStatus(w, http.StatusMethodNotAllowed, metav1.StatusReasonMethodNotAllowed)
	}
}

type k8sFixtureStream struct {
	stream httpstream.Stream
	reply  <-chan struct{}
}

// The v4 SPDY handshake follows client-go's own remotecommand protocol fixture.
// All input/output remains synthetic; received command strings are never executed.
func (f *Fixture) serveExec(w http.ResponseWriter, r *http.Request, pod string) {
	query := r.URL.Query()
	if r.Method != http.MethodPost || query.Get("container") != "agent" ||
		query.Get("stdout") != "true" || query.Get("stderr") != "true" ||
		query.Get("stdin") == "true" || query.Get("tty") == "true" {
		f.problem("unsupported exec options")
		fixtureStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest)
		return
	}
	if _, err := httpstream.Handshake(r, w, []string{protocol.StreamProtocolV4Name}); err != nil {
		f.problem("SPDY handshake failed: " + err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), testutil.GoroutineRaceTimeout)
	defer cancel()
	incoming := make(chan k8sFixtureStream, 3)
	conn := spdy.NewResponseUpgrader().UpgradeResponse(w, r, func(stream httpstream.Stream, reply <-chan struct{}) error {
		select {
		case incoming <- k8sFixtureStream{stream, reply}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if conn == nil {
		f.problem("SPDY upgrade failed")
		return
	}
	f.streams.Add(1)
	defer f.streams.Done()
	defer conn.Close() //nolint:errcheck // private fixture connection
	conn.SetIdleTimeout(testutil.GoroutineRaceTimeout)
	streams := make(map[string]httpstream.Stream)
	for len(streams) < 3 {
		select {
		case item := <-incoming:
			select {
			case <-item.reply:
			case <-ctx.Done():
				f.problem("SPDY stream reply timed out")
				return
			}
			kind := item.stream.Headers().Get(corev1.StreamType)
			if streams[kind] != nil || (kind != corev1.StreamTypeStdout && kind != corev1.StreamTypeStderr && kind != corev1.StreamTypeError) {
				f.problem("unsupported/duplicate SPDY stream")
				return
			}
			streams[kind] = item.stream
		case <-ctx.Done():
			f.problem("SPDY stream creation timed out")
			return
		}
	}
	out, exitCode := f.command(pod, query["command"])
	if _, err := io.WriteString(streams[corev1.StreamTypeStdout], out); err != nil {
		f.problem("SPDY stdout failed: " + err.Error())
		return
	}
	_ = streams[corev1.StreamTypeStdout].Close()
	_ = streams[corev1.StreamTypeStderr].Close()
	status := metav1.Status{Status: metav1.StatusSuccess}
	if exitCode != 0 {
		status.Status = metav1.StatusFailure
		status.Reason = protocol.NonZeroExitCodeReason
		status.Details = &metav1.StatusDetails{Causes: []metav1.StatusCause{{
			Type: protocol.ExitCodeCauseType, Message: fmt.Sprint(exitCode),
		}}}
	}
	if err := json.NewEncoder(streams[corev1.StreamTypeError]).Encode(status); err != nil {
		f.problem("SPDY status failed: " + err.Error())
	}
}

func (f *Fixture) command(pod string, cmd []string) (string, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.execs++
	if f.pods[pod] == nil {
		return "", 1
	}
	if len(cmd) >= 4 && cmd[0] == "tmux" && cmd[2] == "-t" && cmd[3] == "main" {
		switch cmd[1] {
		case "has-session":
			if len(cmd) == 4 {
				return "", 0
			}
		case "set-environment":
			if len(cmd) == 6 {
				if cmd[4] == "-u" {
					delete(f.meta[pod], cmd[5])
				} else {
					f.meta[pod][cmd[4]] = cmd[5]
				}
				return "", 0
			}
		case "show-environment":
			if len(cmd) == 5 {
				if value, ok := f.meta[pod][cmd[4]]; ok {
					return cmd[4] + "=" + value, 0
				}
				return "-" + cmd[4], 0
			}
		case "display-message":
			if len(cmd) == 6 && cmd[4] == "-p" {
				switch cmd[5] {
				case "#{session_activity}":
					return "1700000000", 0
				case "#{session_attached}":
					return "0", 0
				}
			}
		case "capture-pane":
			if len(cmd) == 7 && cmd[4] == "-p" && cmd[5] == "-S" {
				return "synthetic pane\n", 0
			}
		case "send-keys":
			if len(cmd) == 5 && (cmd[4] == "Enter" || cmd[4] == "C-c") {
				return "", 0
			}
			if len(cmd) == 6 && cmd[4] == "-l" {
				return "", 0
			}
		case "clear-history":
			if len(cmd) == 4 {
				return "", 0
			}
		case "pipe-pane":
			if len(cmd) == 6 && cmd[4] == "-o" && cmd[5] == "cat >> /tmp/agent-output.log" {
				return "", 0
			}
		}
	}
	f.problems = append(f.problems, fmt.Sprintf("unsupported exec command: %q", cmd))
	return "", 127
}

// Snapshot is a consistent observation of one protocol fixture.
type Snapshot struct {
	Created, Deleted, Remaining, Execs int
	Problems                           []string
	Names                              []string
}

// Snapshot copies counters and pod names without exposing mutable fixture state.
func (f *Fixture) Snapshot() Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := Snapshot{Created: f.created, Deleted: f.deleted, Remaining: len(f.pods), Execs: f.execs, Problems: append([]string(nil), f.problems...)}
	for name := range f.pods {
		s.Names = append(s.Names, name)
	}
	return s
}

// Wait waits for owned SPDY streams after the HTTP server has been closed.
func (f *Fixture) Wait() { f.streams.Wait() }

//go:build integration

package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/testutil/k8sfixture"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func hybridConformanceName(route string, n int64) string {
	if route == "mixed" {
		route = "local"
		if n%2 == 0 {
			route = "remote"
		}
	}
	return fmt.Sprintf("gc-hybrid-%s-%d", route, n)
}

func hybridConformanceConfig(t *testing.T) config.SessionConfig {
	t.Helper()
	sc, _ := hybridConformanceFixture(t)
	return sc
}

type hybridFixtureContext struct {
	config   config.SessionConfig
	protocol *k8sfixture.Fixture
}

var hybridFixtures = struct {
	sync.Mutex
	byTest map[*testing.T]hybridFixtureContext
}{byTest: make(map[*testing.T]hybridFixtureContext)}

// Each leaf test owns one credential-free endpoint shared by its factories.
// Register teardown once, before any provider Start cleanup, so discovery tests
// that create a second factory cannot tear down the first factory's session.
// tmux stays inside cmd/gc TestMain's private namespace, with one socket per
// leaf test; no default/city socket, installed controller or cluster is used.
func hybridConformanceFixture(t *testing.T) (config.SessionConfig, *k8sfixture.Fixture) {
	t.Helper()
	hybridFixtures.Lock()
	defer hybridFixtures.Unlock()
	if current, ok := hybridFixtures.byTest[t]; ok {
		return current.config, current.protocol
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Fatalf("hybrid conformance requires real tmux: %v", err)
	}
	if os.Getenv("TMUX_TMPDIR") == "" {
		t.Fatal("cmd/gc private tmux namespace missing")
	}
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if strings.HasPrefix(key, "GC_K8S_") || strings.HasPrefix(key, "KUBERNETES_") ||
			strings.EqualFold(key, "http_proxy") || strings.EqualFold(key, "https_proxy") || strings.EqualFold(key, "all_proxy") {
			t.Setenv(key, "")
		}
	}
	t.Setenv("GC_HYBRID_REMOTE_MATCH", "")
	root := t.TempDir()
	t.Setenv("HOME", root)
	fixture := k8sfixture.New()
	server := httptest.NewServer(fixture)
	t.Cleanup(func() {
		server.Close()
		fixture.Wait()
		s := fixture.Snapshot()
		if s.Remaining != 0 || s.Created != s.Deleted || len(s.Problems) != 0 {
			t.Errorf("hybrid remote fixture not quiescent: %+v", s)
		}
		if s.Created != 0 {
			t.Logf("hybrid remote: created=%d deleted=%d exec=%d remaining=%d unexpected=%d", s.Created, s.Deleted, s.Execs, s.Remaining, len(s.Problems))
		}
	})
	cfg := clientcmdapi.Config{
		Clusters:       map[string]*clientcmdapi.Cluster{"fixture": {Server: server.URL}},
		AuthInfos:      map[string]*clientcmdapi.AuthInfo{"fixture": {}},
		Contexts:       map[string]*clientcmdapi.Context{"fixture": {Cluster: "fixture", AuthInfo: "fixture", Namespace: "conformance"}},
		CurrentContext: "fixture",
	}
	data, err := clientcmd.Write(cfg)
	if err != nil {
		t.Fatal(err)
	}
	kubeconfig := filepath.Join(root, "config")
	if err := os.WriteFile(kubeconfig, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kubeconfig)
	t.Setenv("GC_K8S_NAMESPACE", "conformance")
	t.Setenv("GC_K8S_IMAGE", "fixture.invalid/no-pull:synthetic")
	t.Setenv("GC_K8S_PREBAKED", "true")
	socket := fmt.Sprintf("gc-hybrid-%x", sha256.Sum256([]byte(t.Name())))[:26]
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "tmux", "-L", socket, "list-sessions", "-F", "#{session_name}").Output()
		// Stop intentionally leaves the empty server alive. Retire only this
		// fixture-owned, proven-empty server, then require native absence.
		if err == nil && len(out) == 0 {
			if err := exec.CommandContext(ctx, "tmux", "-L", socket, "kill-server").Run(); err != nil {
				t.Errorf("retiring empty fixture server: %v", err)
			}
			out, err = exec.CommandContext(ctx, "tmux", "-L", socket, "list-sessions", "-F", "#{session_name}").Output()
		}
		var exited *exec.ExitError
		if len(out) != 0 || !errors.As(err, &exited) || exited.ExitCode() != 1 {
			t.Errorf("hybrid local fixture not quiescent: socket=%s output=%q err=%v", socket, out, err)
			// Narrow fallback applies only to this test's socket, after recording FAIL.
			_ = exec.CommandContext(ctx, "tmux", "-L", socket, "kill-server").Run()
		}
	})
	// Hybrid discovery queries both backends. A ready local server with zero
	// sessions is distinct from a missing/unreachable backend. Provision only
	// this private empty server, like the synthetic remote API above.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "tmux", "-L", socket, "-f", "/dev/null",
		"start-server", ";", "set-option", "-g", "exit-empty", "off").CombinedOutput(); err != nil {
		t.Fatalf("starting isolated empty tmux fixture: %v: %s", err, out)
	}
	sc := config.SessionConfig{Socket: socket, RemoteMatch: "remote", NudgeReadyTimeout: "250ms"}
	hybridFixtures.byTest[t] = hybridFixtureContext{config: sc, protocol: fixture}
	t.Cleanup(func() {
		hybridFixtures.Lock()
		delete(hybridFixtures.byTest, t)
		hybridFixtures.Unlock()
	})
	return sc, fixture
}

func TestHybridConformanceRoutesAndMergesBothBackends(t *testing.T) {
	sc, f := hybridConformanceFixture(t)
	p, err := newHybridProvider(sc, "unused-fixture-city", "")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"gc-hybrid-local-proof", "gc-hybrid-remote-proof"}
	for _, name := range names {
		if err := p.Start(context.Background(), name, runtime.Config{Command: "sleep 300", WorkDir: t.TempDir()}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := p.Stop(name); err != nil {
				t.Error(err)
			}
		})
	}
	if s := f.Snapshot(); s.Created != 1 || !slices.Equal(s.Names, []string{names[1]}) {
		t.Fatalf("wrong remote route: %+v", s)
	}
	got, err := p.ListRunning("gc-hybrid-")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if !slices.Equal(got, names) {
		t.Fatalf("combined discovery=%v, want %v", got, names)
	}
}

func TestHybridConformanceEnvironmentOverrideAndInvalidConfig(t *testing.T) {
	t.Run("environment overrides configured match", func(t *testing.T) {
		sc, f := hybridConformanceFixture(t)
		t.Setenv("GC_HYBRID_REMOTE_MATCH", "local")
		p, err := newHybridProvider(sc, "unused-fixture-city", "")
		if err != nil {
			t.Fatal(err)
		}
		name := "gc-hybrid-local-overridden"
		if err := p.Start(context.Background(), name, runtime.Config{Command: "sleep 300"}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := p.Stop(name); err != nil {
				t.Error(err)
			}
		})
		if s := f.Snapshot(); s.Created != 1 || !slices.Equal(s.Names, []string{name}) {
			t.Fatalf("override did not select remote: %+v", s)
		}
	})
	t.Run("invalid kubeconfig refuses constructor", func(t *testing.T) {
		sc, f := hybridConformanceFixture(t)
		path := filepath.Join(t.TempDir(), "invalid-config")
		if err := os.WriteFile(path, []byte("{malformed"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("KUBECONFIG", path)
		p, err := newHybridProvider(sc, "unused-fixture-city", "")
		if err == nil || p != nil || f.Snapshot().Created != 0 {
			t.Fatalf("invalid config did not refuse pre-mutation: provider=%T err=%v", p, err)
		}
	})
}

func TestHybridConformanceMissingLocalBackendReported(t *testing.T) {
	sc, _ := hybridConformanceFixture(t)
	p, err := newHybridProvider(sc, "unused-fixture-city", "")
	if err != nil {
		t.Fatal(err)
	}
	name := "gc-hybrid-remote-survivor"
	if err := p.Start(context.Background(), name, runtime.Config{Command: "sleep 300"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Stop(name); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "tmux", "-L", sc.Socket, "kill-server").Run(); err != nil {
		t.Fatal(err)
	}
	names, err := p.ListRunning("gc-hybrid-")
	if err == nil || !strings.Contains(err.Error(), "local") || !slices.Equal(names, []string{name}) {
		t.Fatalf("missing local backend must report partial failure and retain remote inventory: names=%v err=%v", names, err)
	}
}

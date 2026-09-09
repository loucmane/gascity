//go:build integration

package k8s

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/testutil/k8sfixture"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// The integration process owns one credential-free loopback endpoint. Setup
// fails before m.Run on error; no missing-dependency/short-mode skip exists.
// No pod, tmux process, kubectl process or real cluster is started.
func TestMain(m *testing.M) {
	os.Exit(runK8sConformanceMain(m))
}

var conformanceEndpoint string

func runK8sConformanceMain(m *testing.M) (code int) {
	root, err := os.MkdirTemp("", "gc-k8s-conformance-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() {
		if err := os.RemoveAll(root); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}()
	// InClusterConfig must fail before looking for a service-account token.
	// Explicit KUBECONFIG below prevents reading a personal default config.
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if strings.HasPrefix(key, "GC_K8S_") || strings.HasPrefix(key, "KUBERNETES_") ||
			strings.EqualFold(key, "http_proxy") || strings.EqualFold(key, "https_proxy") ||
			strings.EqualFold(key, "all_proxy") {
			if err := os.Unsetenv(key); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
		}
	}
	fixture := k8sfixture.New()
	server := httptest.NewServer(fixture)
	defer server.Close()
	conformanceEndpoint = server.URL
	cfg := clientcmdapi.Config{
		Clusters:  map[string]*clientcmdapi.Cluster{"fixture": {Server: server.URL}},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{"fixture": {}},
		Contexts: map[string]*clientcmdapi.Context{"fixture": {
			Cluster: "fixture", AuthInfo: "fixture", Namespace: "conformance",
		}},
		CurrentContext: "fixture",
	}
	data, err := clientcmd.Write(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	kubeconfig := filepath.Join(root, "config")
	if err := os.WriteFile(kubeconfig, data, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for key, value := range map[string]string{
		"KUBECONFIG": kubeconfig, "HOME": root, "GC_K8S_NAMESPACE": "conformance",
		"GC_K8S_IMAGE": "fixture.invalid/no-pull:synthetic", "GC_K8S_PREBAKED": "true",
	} {
		if err := os.Setenv(key, value); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	code = m.Run()
	server.Close()
	fixture.Wait()
	stats := fixture.Snapshot()
	fmt.Fprintf(os.Stderr, "k8s protocol fixture: created=%d deleted=%d remaining=%d exec=%d unexpected=%d\n",
		stats.Created, stats.Deleted, stats.Remaining, stats.Execs, len(stats.Problems))
	if stats.Remaining != 0 || stats.Created != stats.Deleted || len(stats.Problems) != 0 {
		fmt.Fprintf(os.Stderr, "k8s fixture containment/contract failure: %v\n", stats.Problems)
		code = 1
	}
	return code
}

func TestK8sConformanceConstructorUsesOnlyFixtureConfig(t *testing.T) {
	provider, err := NewSeamBacked()
	if err != nil {
		t.Fatal(err)
	}
	wrapper, ok := provider.(*seamBackedProvider)
	if !ok {
		t.Fatalf("constructor returned %T", provider)
	}
	ops, ok := wrapper.raw.ops.(*realK8sOps)
	if !ok {
		t.Fatalf("production ops replaced by %T", wrapper.raw.ops)
	}
	cfg := ops.restConfig
	if cfg.Host != conformanceEndpoint || cfg.BearerToken != "" || cfg.BearerTokenFile != "" ||
		cfg.Username != "" || cfg.Password != "" || cfg.ExecProvider != nil || cfg.AuthProvider != nil ||
		cfg.CertFile != "" || cfg.KeyFile != "" || len(cfg.CertData) != 0 || len(cfg.KeyData) != 0 {
		t.Fatal("constructor did not use the exact credential-free fixture config")
	}
}

func TestK8sConformanceConstructorRejectsInvalidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid-config")
	if err := os.WriteFile(path, []byte("{malformed"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", path)
	provider, err := NewSeamBacked()
	if err == nil || provider != nil {
		t.Fatal("invalid isolated kubeconfig did not fail closed")
	}
}

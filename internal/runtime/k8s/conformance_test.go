//go:build integration

package k8s

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/runtime/runtimetest"
)

// TestMain owns the isolated kubeconfig and HTTP/SPDY endpoint. This factory
// exercises the exact registered constructor without replacing its client/ops.
func TestK8sConformance(t *testing.T) {
	var counter int64
	runtimetest.RunProviderTests(t, func(t *testing.T) (runtime.Provider, runtime.Config, string) {
		provider, err := NewSeamBacked()
		if err != nil {
			t.Fatal(err)
		}
		return provider, runtime.Config{Command: "sleep 300"}, fmt.Sprintf("gc-conformance-%d", atomic.AddInt64(&counter, 1))
	})
}

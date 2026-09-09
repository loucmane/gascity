//go:build integration

package herdr

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/runtime/runtimetest"
)

// TestHerdrConformance runs the full runtime.Provider conformance suite against
// the herdr provider backed by a real herdr binary. Each session gets its own
// isolated herdr session-server so the contract's session-scoped assertions
// (ListRunning, orphan detection, …) don't observe sibling sessions. The
// integration TestMain requires the binary and supplies a private HOME; missing
// setup fails instead of silently skipping the production contract.
func TestHerdrConformance(t *testing.T) {
	var counter int64
	runtimetest.RunProviderTests(t, func(t *testing.T) (runtime.Provider, runtime.Config, string) {
		return New(herdrConformanceSession(t), t.TempDir(), t.TempDir(), 0, 0),
			runtime.Config{WorkDir: t.TempDir()}, fmt.Sprintf("conf-%d", atomic.AddInt64(&counter, 1))
	})
}

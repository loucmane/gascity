//go:build integration

package ssh

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/runtime/runtimetest"
)

// TestSSHConformance exercises the production constructor and real OpenSSH
// transport against a loopback fixture running only a private tmux server.
func TestSSHConformance(t *testing.T) {
	var counter int64
	runtimetest.RunProviderTests(t, func(t *testing.T) (runtime.Provider, runtime.Config, string) {
		return NewSeamBacked(sshConformance.ep),
			runtime.Config{WorkDir: sshConformance.home},
			fmt.Sprintf("conf-ssh-%d", atomic.AddInt64(&counter, 1))
	})
}

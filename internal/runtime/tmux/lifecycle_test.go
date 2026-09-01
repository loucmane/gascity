package tmux

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestConfigureServerSendsSetOptionExitEmptyOff verifies that ConfigureServer
// issues set-option -g exit-empty off through the executor.
func TestConfigureServerSendsSetOptionExitEmptyOff(t *testing.T) {
	fe := &fakeExecutor{}
	tm := &Tmux{cfg: DefaultConfig(), exec: fe}

	if err := tm.ConfigureServer(); err != nil {
		t.Fatalf("ConfigureServer() error = %v", err)
	}

	for _, call := range fe.calls {
		if containsSetOptionExitEmptyWithValue(call, "off") {
			return
		}
	}
	t.Fatalf("ConfigureServer did not issue set-option -g exit-empty off; calls = %v", fe.calls)
}

// TestConfigureServerReappliesExitEmptyForReplacementServer verifies that
// server configuration is applied on every call. A Tmux wrapper can outlive
// the server bound to its socket; per-instance sync.Once would leave a
// replacement server at tmux's unsafe exit-empty=on default.
func TestConfigureServerReappliesExitEmptyForReplacementServer(t *testing.T) {
	fe := &fakeExecutor{}
	tm := &Tmux{cfg: DefaultConfig(), exec: fe}

	for i := range 2 {
		if err := tm.ConfigureServer(); err != nil {
			t.Fatalf("ConfigureServer() call %d error = %v", i, err)
		}
	}

	count := 0
	for _, call := range fe.calls {
		if containsSetOptionExitEmptyWithValue(call, "off") {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("set-option -g exit-empty off issued %d times across 2 ConfigureServer calls, want 2", count)
	}
}

// TestTeardownServerCallsKillServer verifies that TeardownServer delegates to
// tmux kill-server via the executor.
func TestTeardownServerCallsKillServer(t *testing.T) {
	fe := &fakeExecutor{}
	tm := &Tmux{cfg: DefaultConfig(), exec: fe}

	if err := tm.TeardownServer(); err != nil {
		t.Fatalf("TeardownServer() error = %v", err)
	}

	for _, call := range fe.calls {
		for _, arg := range call {
			if arg == "kill-server" {
				return
			}
		}
	}
	t.Fatalf("TeardownServer did not call kill-server; calls = %v", fe.calls)
}

// TestTeardownServerTreatsAlreadyGoneServerAsSuccess verifies that TeardownServer
// returns nil when the tmux server is already gone (ErrNoServer), consistent with
// KillServer's existing semantics.
func TestTeardownServerTreatsAlreadyGoneServerAsSuccess(t *testing.T) {
	fe := &fakeExecutor{err: ErrNoServer}
	tm := &Tmux{cfg: DefaultConfig(), exec: fe}

	if err := tm.TeardownServer(); err != nil {
		t.Fatalf("TeardownServer() = %v, want nil for already-gone server", err)
	}
}

func TestCloseSessionArmsNativeExitEmptyAroundExactStop(t *testing.T) {
	fe := &fakeExecutor{}
	p := NewProviderWithConfig(DefaultConfig())
	p.tm.exec = fe

	if err := p.CloseSession("reviewer"); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	on, kill, off := -1, -1, -1
	for i, call := range fe.calls {
		switch {
		case containsSetOptionExitEmptyWithValue(call, "on"):
			on = i
		case containsSetOptionExitEmptyWithValue(call, "off"):
			off = i
		}
		for _, arg := range call {
			if arg == "kill-session" {
				kill = i
			}
			if arg == "kill-server" {
				t.Fatalf("CloseSession issued racy kill-server call: %v", fe.calls)
			}
		}
	}
	if on < 0 || kill <= on || off <= kill {
		t.Fatalf("calls = %v, want exit-empty on before kill-session and off afterward", fe.calls)
	}
}

func TestCloseSessionRestoresExitEmptyAfterStopFailure(t *testing.T) {
	stopErr := errors.New("kill failed")
	fe := &fakeExecutor{errs: []error{nil, nil, stopErr, nil}}
	p := NewProviderWithConfig(DefaultConfig())
	p.tm.exec = fe

	err := p.CloseSession("reviewer")
	if !errors.Is(err, stopErr) {
		t.Fatalf("CloseSession error = %v, want %v", err, stopErr)
	}
	for _, call := range fe.calls {
		if containsSetOptionExitEmptyWithValue(call, "off") {
			return
		}
	}
	t.Fatalf("CloseSession did not restore exit-empty off after failure; calls = %v", fe.calls)
}

func TestCloseSessionLockExcludesConcurrentProviderProcess(t *testing.T) {
	runtimeDir := t.TempDir()
	p := NewProviderWithConfig(Config{RuntimeDir: runtimeDir})
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- p.withCloseSessionLock(func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	lockPath := filepath.Join(runtimeDir, closeSessionLockFile)
	contender, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		close(release)
		t.Fatalf("open contender: %v", err)
	}
	defer contender.Close() //nolint:errcheck
	if err := syscall.Flock(int(contender.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); !errors.Is(err, syscall.EWOULDBLOCK) {
		close(release)
		t.Fatalf("concurrent lock error = %v, want EWOULDBLOCK", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("held close lock: %v", err)
	}
	if err := syscall.Flock(int(contender.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	_ = syscall.Flock(int(contender.Fd()), syscall.LOCK_UN)
}

// containsSetOptionExitEmptyWithValue returns true if args contains the
// sequence "set-option -g exit-empty <value>", possibly preceded by socket flags.
func containsSetOptionExitEmptyWithValue(args []string, value string) bool {
	for i, arg := range args {
		if arg == "set-option" && i+3 < len(args) &&
			args[i+1] == "-g" && args[i+2] == "exit-empty" && args[i+3] == value {
			return true
		}
	}
	return false
}

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/citylayout"
	"github.com/gastownhall/gascity/internal/pidutil"
)

// stopNudgePollersAtCleanup registers a cleanup that stops every detached
// nudge poller started for cityPath. Call it right after creating the city so
// it runs before the city's TempDir removal: a live poller keeps writing under
// .gc/nudges and makes that removal fail with "directory not empty".
func stopNudgePollersAtCleanup(t *testing.T, cityPath string) {
	t.Helper()
	t.Cleanup(func() {
		if err := stopCityNudgePollers(cityPath, syscall.Kill); err != nil {
			t.Errorf("stop nudge pollers for %s: %v", cityPath, err)
		}
	})
}

// stopCityNudgePollers sends SIGTERM to the process group of each live nudge
// poller named by a pid file in cityPath's pollers dir (ensureNudgePoller
// starts every poller as a group leader) and waits for the poller to exit,
// escalating to SIGKILL. A pid that is gone, or whose command line is not a
// nudge poller for cityPath, is never signaled.
func stopCityNudgePollers(cityPath string, kill func(int, syscall.Signal) error) error {
	pidPaths, err := filepath.Glob(filepath.Join(citylayout.RuntimePath(cityPath, "nudges", "pollers"), "*.pid"))
	if err != nil {
		return err
	}
	var errs []error
	for _, pidPath := range pidPaths {
		pid, ok := liveCityNudgePollerPID(pidPath, cityPath)
		if !ok {
			continue
		}
		if err := kill(-pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
			errs = append(errs, fmt.Errorf("signal nudge poller %d: %w", pid, err))
			continue
		}
		if err := waitForPIDExit(pid, 5*time.Second, time.Second); err != nil {
			errs = append(errs, fmt.Errorf("nudge poller from %s: %w", pidPath, err))
		}
	}
	return errors.Join(errs...)
}

// liveCityNudgePollerPID returns the pid recorded at pidPath when it names a
// live "nudge poll --city <cityPath>" process. Without /proc the command line
// cannot be checked, so the pid is left alone.
func liveCityNudgePollerPID(pidPath, cityPath string) (int, bool) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || !pidutil.Alive(pid) {
		return 0, false
	}
	argv, err := pidutil.Cmdline(pid)
	if err != nil || !pidutil.ArgvContainsSequence(argv, "nudge", "poll") {
		return 0, false
	}
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == "--city" && samePath(argv[i+1], cityPath) {
			return pid, true
		}
	}
	return 0, false
}

func TestStopCityNudgePollersSignalsOnlyThisCitysPollerGroups(t *testing.T) {
	if goruntime.GOOS != "linux" {
		t.Skip("poller ownership check uses /proc on linux")
	}
	cityPath := t.TempDir()
	poller := startPollerLikeProcess(t, cityPath, "session-id")
	otherCityPoller := startPollerLikeProcess(t, t.TempDir(), "other-city")
	for agentName, pid := range map[string]int{
		"session-id": poller.Process.Pid,
		"other-city": otherCityPoller.Process.Pid,
		"recycled":   os.Getpid(),
		"stale":      deadPID(t),
	} {
		pidPath := nudgePollerPIDPath(cityPath, "sess-worker", agentName)
		if err := os.MkdirAll(filepath.Dir(pidPath), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", pid)), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	type sentSignal struct {
		pid int
		sig syscall.Signal
	}
	var sent []sentSignal
	// Deliver only the poller group's signal, and only to the fake poller, so
	// the unit lane never signals a real process group.
	kill := func(pid int, sig syscall.Signal) error {
		sent = append(sent, sentSignal{pid: pid, sig: sig})
		if pid != -poller.Process.Pid {
			return nil
		}
		return poller.Process.Signal(sig)
	}

	if err := stopCityNudgePollers(cityPath, kill); err != nil {
		t.Fatalf("stopCityNudgePollers: %v", err)
	}

	if want := []sentSignal{{pid: -poller.Process.Pid, sig: syscall.SIGTERM}}; !slices.Equal(sent, want) {
		t.Fatalf("signals = %+v, want %+v", sent, want)
	}
	if pidutil.Alive(poller.Process.Pid) {
		t.Fatalf("city poller %d still alive after stopCityNudgePollers", poller.Process.Pid)
	}
}

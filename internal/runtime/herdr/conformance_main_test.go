//go:build integration

package herdr

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// The integration process, including the real CLI's children, gets a private
// short HOME and no inherited provider credentials, shell hooks or city state.
func TestMain(m *testing.M) {
	os.Exit(runHerdrConformanceMain(m))
}

var herdrConformanceState struct {
	sync.Mutex
	next     int64
	home     string
	sessions []string
}

func runHerdrConformanceMain(m *testing.M) int {
	binary, err := exec.LookPath("herdr")
	if err != nil {
		fmt.Fprintf(os.Stderr, "herdr conformance requires the real herdr executable: %v\n", err)
		return 1
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// Keep the complete Unix socket pathname below the portable sun_path limit.
	home, err := os.MkdirTemp("/tmp", "gc-herdr-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	herdrConformanceState.home = home
	os.Clearenv()
	for key, value := range map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, ".config"),
		"XDG_CACHE_HOME": filepath.Join(home, ".cache"),
		"XDG_DATA_HOME":  filepath.Join(home, ".local", "share"),
		"PATH":           filepath.Dir(binary) + string(os.PathListSeparator) + "/usr/bin:/bin",
		"SHELL":          "/bin/sh", "TERM": "xterm-256color", "LANG": "C.UTF-8",
	} {
		if err := os.Setenv(key, value); err != nil {
			fmt.Fprintf(os.Stderr, "herdr fixture setup failed; preserved %s: %v\n", home, err)
			return 1
		}
	}
	dir := filepath.Join(home, ".config", "herdr")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"),
		[]byte("[update]\nversion_check = false\nmanifest_check = false\n"), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	code := m.Run()
	herdrConformanceState.Lock()
	names := append([]string(nil), herdrConformanceState.sessions...)
	herdrConformanceState.Unlock()
	live := 0
	for _, name := range names {
		if newClient(name, "").serverAlive() {
			fmt.Fprintf(os.Stderr, "herdr fixture server remains alive: %s\n", name)
			live++
		}
	}
	fmt.Fprintf(os.Stderr, "herdr conformance fixture: home=%s namespaces=%d live=%d\n", home, len(names), live)
	if live != 0 {
		fmt.Fprintf(os.Stderr, "preserving failed fixture root %s; no fallback cleanup\n", home)
		return 1
	}
	if err := os.RemoveAll(home); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}

// The factory calls this only to allocate its unique namespace and register the
// distinct server teardown. Provider.Stop remains owned by the shared suite.
func herdrConformanceSession(t *testing.T) string {
	t.Helper()
	name := fmt.Sprintf("gctest-c-%d", atomic.AddInt64(&herdrConformanceState.next, 1))
	herdrConformanceState.Lock()
	herdrConformanceState.sessions = append(herdrConformanceState.sessions, name)
	herdrConformanceState.Unlock()
	t.Cleanup(func() {
		c := newClient(name, "")
		if err := c.stopServer(); err != nil {
			t.Errorf("herdr server teardown %s: %v", name, err)
		}
		if c.serverAlive() {
			t.Errorf("herdr server %s still accepts connections after teardown", name)
		}
	})
	return name
}

func TestHerdrConformanceEnvironmentIsIsolated(t *testing.T) {
	home := herdrConformanceState.home
	if home == "" || os.Getenv("HOME") != home || filepath.Dir(home) != "/tmp" || !strings.HasPrefix(filepath.Base(home), "gc-herdr-") {
		t.Fatal("Herdr integration process is not bound to its private HOME")
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "herdr", "config.toml"))
	if err != nil || string(data) != "[update]\nversion_check = false\nmanifest_check = false\n" {
		t.Fatalf("background update/manifest checks are not disabled: %v", err)
	}
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		for _, prefix := range []string{"GC_", "HERDR_", "ANTHROPIC_", "OPENAI_", "CLAUDE_", "BEADS_", "DOLT_"} {
			if strings.HasPrefix(key, prefix) {
				t.Fatalf("inherited context variable remains: %s", key)
			}
		}
	}
}

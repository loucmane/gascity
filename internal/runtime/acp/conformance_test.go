//go:build integration

package acp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/runtime/runtimetest"
)

type acpConformanceFixture struct {
	once    sync.Once
	dir     string
	command string
	err     error
}

type acpDefaultConformanceFixture struct {
	once sync.Once
	acp  acpConformanceFixture
}

// TestACPDefaultDirConformance exercises the production default constructor,
// including shared default-directory state across its independently constructed
// providers. Only the process temporary root is isolated; the provider receives
// no injected directory and retains its normal socket and metadata behavior.
func TestACPDefaultDirConformance(t *testing.T) {
	var fixture acpDefaultConformanceFixture
	var counter int64

	runtimetest.RunProviderTests(t, func(caseT *testing.T) (runtime.Provider, runtime.Config, string) {
		return NewSeamBacked(acpDefaultConformanceConfig(caseT, t, &fixture)), runtime.Config{
			Command: acpConformanceCommand(caseT, t, &fixture.acp),
			WorkDir: caseT.TempDir(),
		}, fmt.Sprintf("gc-acp-default-%d-%d", os.Getpid(), atomic.AddInt64(&counter, 1))
	})
}

func acpDefaultConformanceConfig(caseT, ownerT *testing.T, fixture *acpDefaultConformanceFixture) Config {
	caseT.Helper()
	fixture.once.Do(func() {
		if err := prepareACPConformanceFixture(ownerT, &fixture.acp); err != nil {
			return
		}
		// Build the protocol fixture before changing its temporary environment.
		// Setenv restores the caller's environment, and the existing fixture owns
		// only its newly created directory. Never touch the host's gc-acp state.
		root := filepath.Dir(fixture.acp.dir)
		ownerT.Setenv("TMPDIR", root)
		ownerT.Setenv("TMP", root)
		ownerT.Setenv("TEMP", root)
	})
	if fixture.acp.err != nil {
		caseT.Fatal(fixture.acp.err)
	}
	return Config{}
}

func TestACPDefaultConformanceFixtureUsesIsolatedDefaultState(t *testing.T) {
	var fixture acpDefaultConformanceFixture
	cfg := acpDefaultConformanceConfig(t, t, &fixture)
	root := filepath.Dir(fixture.acp.dir)
	if os.TempDir() != root {
		t.Fatalf("temporary root = %q, want fixture %q", os.TempDir(), root)
	}
	first, second := NewProvider(cfg), NewProvider(cfg)
	want := filepath.Join(root, fmt.Sprintf("gc-acp-%d", os.Geteuid()))
	if first.dir != want || second.dir != want || want == fixture.acp.dir {
		t.Fatalf("default constructors must share %q, not injected %q: %q, %q", want, fixture.acp.dir, first.dir, second.dir)
	}
}

func TestACPConformance(t *testing.T) {
	var fixture acpConformanceFixture
	var counter int64

	runtimetest.RunProviderTests(t, func(caseT *testing.T) (runtime.Provider, runtime.Config, string) {
		return NewSeamBackedWithDir(acpConformanceDir(caseT, t, &fixture), Config{}), runtime.Config{
			Command: acpConformanceCommand(caseT, t, &fixture),
			WorkDir: caseT.TempDir(),
		}, fmt.Sprintf("gc-acp-conform-%d", atomic.AddInt64(&counter, 1))
	})
}

// TestACPDefaultDirConformance runs the same full Provider conformance
// suite against the constructor cmd/gc's "acp" registration calls when a city
// path is absent: NewSeamBacked, which (via NewProvider -> defaultProviderDir)
// keeps socket and meta files under the shared, per-euid default temporary
// directory (os.TempDir()/gc-acp-<euid>) rather than an injected one. That
// directory is process-shared by design, so session names carry the PID — the
// suite only asserts membership of its own names, and PID-scoped names keep
// concurrent runs on one machine from colliding there.
//
// The on-disk socket filename is always a short fixed-length hash of the
// session name (sockKey: "s" + 4 bytes of hex, 14 bytes total with the
// ".sock" suffix — see acp.go, "same as subprocess") regardless of how long
// the session name itself is, so this exercises the exact mechanism that
// keeps the default provider directory within the platform sun_path limit
// (104 bytes on Darwin, 108 on Linux) without any dir injection, TMPDIR
// mutation, or skip.
func TestACPSharedDefaultDirConformance(t *testing.T) {
	var fixture acpConformanceFixture
	var counter int64

	runtimetest.RunProviderTests(t, func(caseT *testing.T) (runtime.Provider, runtime.Config, string) {
		return NewSeamBacked(Config{}), runtime.Config{
			Command: acpConformanceCommand(caseT, t, &fixture),
			WorkDir: caseT.TempDir(),
		}, fmt.Sprintf("gc-acp-default-%d-%d", os.Getpid(), atomic.AddInt64(&counter, 1))
	})
}

func acpConformanceDir(caseT, ownerT *testing.T, fixture *acpConformanceFixture) string {
	caseT.Helper()
	if err := prepareACPConformanceFixture(ownerT, fixture); err != nil {
		caseT.Fatal(err)
	}
	return fixture.dir
}

func acpConformanceCommand(caseT, ownerT *testing.T, fixture *acpConformanceFixture) string {
	caseT.Helper()
	if err := prepareACPConformanceFixture(ownerT, fixture); err != nil {
		caseT.Fatal(err)
	}
	return fixture.command
}

func prepareACPConformanceFixture(ownerT *testing.T, fixture *acpConformanceFixture) error {
	fixture.once.Do(func() {
		// Unix socket paths are capped at 104 bytes on macOS (vs 108 on
		// Linux), so root the fixture directly under /tmp on Darwin.
		root := os.TempDir()
		if goruntime.GOOS == "darwin" {
			root = "/tmp"
		}
		fixtureRoot, err := os.MkdirTemp(root, "acp-conform")
		if err != nil {
			fixture.err = fmt.Errorf("create ACP conformance fixture: %w", err)
			return
		}
		ownerT.Cleanup(func() { _ = os.RemoveAll(fixtureRoot) })

		fixture.dir = filepath.Join(fixtureRoot, "acp")
		if err := os.MkdirAll(fixture.dir, 0o755); err != nil {
			fixture.err = fmt.Errorf("mkdir %q: %w", fixture.dir, err)
			return
		}

		modRoot, err := moduleRoot()
		if err != nil {
			fixture.err = err
			return
		}
		fixture.command = filepath.Join(fixtureRoot, "fakeacp")
		cmd := exec.Command("go", "build", "-o", fixture.command, "./testdata/fakeacp")
		cmd.Dir = filepath.Join(modRoot, "internal", "runtime", "acp")
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fixture.err = fmt.Errorf("building fakeacp: %w", err)
		}
	})
	return fixture.err
}

func moduleRoot() (string, error) {
	cmd := exec.Command("go", "env", "GOMOD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go env GOMOD: %w", err)
	}
	mod := strings.TrimSpace(string(out))
	if mod == "" || mod == "/dev/null" {
		return "", fmt.Errorf("not in a Go module")
	}
	return filepath.Dir(filepath.Clean(mod)), nil
}

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/platforminstall"
)

func TestPlatformRuntimeVersionOutputUsesVersionSubcommand(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "gc")
	script := `#!/bin/sh
if [ "$#" -eq 1 ] && [ "$1" = "version" ]; then
  printf '1.4.1-test\n'
  exit 0
fi
printf 'unexpected arguments: %s\n' "$*" >&2
exit 64
`
	if err := os.WriteFile(executable, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	output, err := platformRuntimeVersionOutput(context.Background(), executable)
	if err != nil {
		t.Fatalf("platformRuntimeVersionOutput() error = %v, output = %q", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != "1.4.1-test" {
		t.Fatalf("platformRuntimeVersionOutput() = %q, want 1.4.1-test", got)
	}
}

func TestPlatformSupervisorLifecycleRefusesSecondRestartAttempt(t *testing.T) {
	restartCalls := 0
	lifecycle := &platformSupervisorLifecycle{
		restart: func(context.Context, platforminstall.Manifest) (int, error) {
			restartCalls++
			return 41, nil
		},
		inspect: func(context.Context, int, bool) (platforminstall.RuntimeProof, error) {
			return platforminstall.RuntimeProof{}, nil
		},
	}
	manifest := platforminstall.Manifest{}
	if err := lifecycle.Restart(context.Background(), manifest); err != nil {
		t.Fatalf("first Restart() error = %v", err)
	}
	if err := lifecycle.Restart(context.Background(), manifest); err == nil || !strings.Contains(err.Error(), "already attempted") {
		t.Fatalf("second Restart() error = %v, want refusal", err)
	}
	if restartCalls != 1 {
		t.Fatalf("restart calls = %d, want 1", restartCalls)
	}
}

func TestPlatformSupervisorLifecyclePreRestartVerifyIsOneShot(t *testing.T) {
	inspectCalls := 0
	lifecycle := &platformSupervisorLifecycle{
		restart: func(context.Context, platforminstall.Manifest) (int, error) { return 0, nil },
		inspect: func(_ context.Context, oldPID int, requireReplacement bool) (platforminstall.RuntimeProof, error) {
			inspectCalls++
			if oldPID != 0 || requireReplacement {
				t.Fatalf("pre-restart inspect oldPID=%d requireReplacement=%t", oldPID, requireReplacement)
			}
			return platforminstall.RuntimeProof{}, errors.New("old runtime")
		},
	}

	if _, err := lifecycle.Verify(context.Background(), platforminstall.Manifest{}); err == nil || err.Error() != "old runtime" {
		t.Fatalf("Verify() error = %v, want old runtime", err)
	}
	if inspectCalls != 1 {
		t.Fatalf("inspect calls = %d, want 1", inspectCalls)
	}
}

func TestPlatformSupervisorLifecyclePollsReplacementWithoutRestartingAgain(t *testing.T) {
	restartCalls := 0
	inspectCalls := 0
	want := platforminstall.RuntimeProof{ExecutableSHA256: strings.Repeat("a", 64), Commit: strings.Repeat("b", 40), Version: "gc version test"}
	lifecycle := &platformSupervisorLifecycle{
		restart: func(context.Context, platforminstall.Manifest) (int, error) {
			restartCalls++
			return 73, nil
		},
		inspect: func(_ context.Context, oldPID int, requireReplacement bool) (platforminstall.RuntimeProof, error) {
			inspectCalls++
			if oldPID != 73 || !requireReplacement {
				t.Fatalf("post-restart inspect oldPID=%d requireReplacement=%t", oldPID, requireReplacement)
			}
			if inspectCalls == 1 {
				return platforminstall.RuntimeProof{}, errors.New("replacement not ready")
			}
			return want, nil
		},
	}
	if err := lifecycle.Restart(context.Background(), platforminstall.Manifest{}); err != nil {
		t.Fatal(err)
	}
	got, err := lifecycle.Verify(context.Background(), platforminstall.Manifest{})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if got != want {
		t.Fatalf("Verify() = %+v, want %+v", got, want)
	}
	if restartCalls != 1 || inspectCalls != 2 {
		t.Fatalf("calls restart=%d inspect=%d, want 1/2", restartCalls, inspectCalls)
	}
}

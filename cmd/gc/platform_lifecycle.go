package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/platforminstall"
)

const (
	platformRuntimeVerifyTimeout = 30 * time.Second
	platformRuntimePollInterval  = 100 * time.Millisecond
)

type platformSupervisorLifecycle struct {
	restart      func(context.Context, platforminstall.Manifest) (int, error)
	inspect      func(context.Context, int, bool) (platforminstall.RuntimeProof, error)
	restartTried bool
	oldPID       int
}

func newPlatformSupervisorLifecycle() *platformSupervisorLifecycle {
	return &platformSupervisorLifecycle{
		restart: restartPlatformSupervisor,
		inspect: inspectPlatformRuntime,
	}
}

func (lifecycle *platformSupervisorLifecycle) Restart(ctx context.Context, manifest platforminstall.Manifest) error {
	if lifecycle.restartTried {
		return fmt.Errorf("platform supervisor restart was already attempted")
	}
	lifecycle.restartTried = true
	oldPID, err := lifecycle.restart(ctx, manifest)
	if err != nil {
		return err
	}
	lifecycle.oldPID = oldPID
	return nil
}

func (lifecycle *platformSupervisorLifecycle) Verify(ctx context.Context, _ platforminstall.Manifest) (platforminstall.RuntimeProof, error) {
	if !lifecycle.restartTried {
		return lifecycle.inspect(ctx, 0, false)
	}
	deadline := time.Now().Add(platformRuntimeVerifyTimeout)
	var lastErr error
	for {
		proof, err := lifecycle.inspect(ctx, lifecycle.oldPID, true)
		if err == nil {
			return proof, nil
		}
		lastErr = err
		if !time.Now().Before(deadline) {
			return platforminstall.RuntimeProof{}, fmt.Errorf("supervisor runtime not verified within %s: %w", platformRuntimeVerifyTimeout, lastErr)
		}
		select {
		case <-ctx.Done():
			return platforminstall.RuntimeProof{}, ctx.Err()
		case <-time.After(platformRuntimePollInterval):
		}
	}
}

func restartPlatformSupervisor(ctx context.Context, _ platforminstall.Manifest) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	pid := supervisorAliveHook()
	if pid == 0 {
		return 0, fmt.Errorf("supervisor is not running")
	}
	exePath, exeErr := readSupervisorExePathHook(pid)
	delegation, delegated, err := supervisorSystemdDelegation()
	if err != nil {
		return 0, fmt.Errorf("resolve supervisor delegation: %w", err)
	}
	serviceName := supervisorSystemdServiceName()
	systemdManaged := !delegated && supervisorSystemctlActive(serviceName)
	launchdLabel := supervisorLaunchdLabel()
	launchdManaged := !delegated && supervisorRuntimeGOOS == "darwin" && supervisorLaunchdActive(launchdLabel)
	if exeErr != nil && !delegated && !systemdManaged && !launchdManaged {
		return 0, fmt.Errorf("resolve direct supervisor executable for pid %d: %w", pid, exeErr)
	}
	spec := restartSpec{
		SystemdManaged: systemdManaged,
		LaunchdManaged: launchdManaged,
		PID:            pid,
		ExePath:        exePath,
		Argv:           []string{"supervisor", "run"},
		ServiceName:    serviceName,
		LaunchdLabel:   launchdLabel,
	}
	if delegated {
		restartErr := runDelegatedSystemctlTimeout(delegation, "try-restart", delegatedSystemctlJobTimeout)
		if restartErr != nil && !isDelegatedSystemctlTimeout(restartErr) {
			return 0, restartErr
		}
		return pid, nil
	}
	if err := restartSupervisor(spec, restartHelpersHook()); err != nil {
		return 0, err
	}
	return pid, nil
}

func inspectPlatformRuntime(ctx context.Context, oldPID int, requireReplacement bool) (platforminstall.RuntimeProof, error) {
	pid := supervisorAliveHook()
	if pid == 0 {
		return platforminstall.RuntimeProof{}, fmt.Errorf("supervisor is not running")
	}
	if requireReplacement && oldPID != 0 && pid == oldPID {
		return platforminstall.RuntimeProof{}, fmt.Errorf("supervisor pid %d has not been replaced", pid)
	}
	procExecutable := fmt.Sprintf("/proc/%d/exe", pid)
	executableSHA, err := sha256File(procExecutable)
	if err != nil {
		return platforminstall.RuntimeProof{}, fmt.Errorf("hash running supervisor executable: %w", err)
	}
	versionOutput, err := platformRuntimeVersionOutput(ctx, procExecutable)
	if err != nil {
		return platforminstall.RuntimeProof{}, fmt.Errorf("read running supervisor version: %w: %s", err, strings.TrimSpace(string(versionOutput)))
	}
	baseURL, err := supervisorAPIBaseURLHook()
	if err != nil {
		return platforminstall.RuntimeProof{}, fmt.Errorf("resolve supervisor API: %w", err)
	}
	statusCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	status, err := newHTTPSupervisorClient(baseURL).Status(statusCtx)
	if err != nil {
		return platforminstall.RuntimeProof{}, fmt.Errorf("read supervisor status: %w", err)
	}
	return platforminstall.RuntimeProof{
		ExecutableSHA256: executableSHA,
		Commit:           status.BuildID,
		Version:          strings.TrimSpace(string(versionOutput)),
	}, nil
}

func platformRuntimeVersionOutput(ctx context.Context, executable string) ([]byte, error) {
	return exec.CommandContext(ctx, executable, "--version").CombinedOutput()
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close() //nolint:errcheck // read-only verification handle
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

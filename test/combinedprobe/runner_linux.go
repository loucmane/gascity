package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"time"

	"golang.org/x/sys/unix"
)

func execute(value plan, planHash string) (returnErr error) {
	if err := os.Mkdir(runRoot, 0o700); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(value.Timeout)*time.Second)
	defer cancel()
	fixture := command(ctx, []string{binaryPath, "fixture"})
	fixtureInput, err := fixture.StdinPipe()
	if err != nil {
		return err
	}
	fixtureOutput, err := fixture.StdoutPipe()
	if err != nil {
		_ = fixtureInput.Close()
		return err
	}
	var fixtureStderr limitedBuffer
	fixture.Stderr = &fixtureStderr
	if err := fixture.Start(); err != nil {
		_ = fixtureInput.Close()
		_ = fixtureOutput.Close()
		return err
	}
	fixtureFD, err := unix.PidfdOpen(fixture.Process.Pid, 0)
	if err != nil {
		_ = fixtureInput.Close()
		_ = fixture.Wait()
		return err
	}
	report := observation{Kind: "observation-only-requires-outer-exit-zero", Plan: planHash, Cases: map[string]outcome{}, Status: "INCONCLUSIVE"}
	defer func() {
		_ = fixtureInput.Close()
		waitErr := fixture.Wait()
		polls := []unix.PollFd{{Fd: int32(fixtureFD), Events: unix.POLLIN}}
		count, pollErr := unix.Poll(polls, 0)
		closeErr := unix.Close(fixtureFD)
		if waitErr != nil || pollErr != nil || closeErr != nil || count != 1 {
			returnErr = errors.Join(returnErr, fmt.Errorf("fixture teardown incomplete: %w %w %w", waitErr, pollErr, closeErr))
		}
		report.FixtureReaped = waitErr == nil && pollErr == nil && closeErr == nil && count == 1
		if returnErr == nil {
			report.Status = "PASS"
			returnErr = validateObservation(report, planHash)
		}
		if returnErr != nil {
			report.Status = "INCONCLUSIVE"
		}
		data, marshalErr := encodeObservation(report)
		if marshalErr != nil {
			returnErr = errors.Join(returnErr, marshalErr)
			return
		}
		_, publishErr := publishObservation(data, publicationOps{create: func() (observationStage, error) {
			return os.OpenFile(runRoot+"/result.stage", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		}, rename: func() error { return os.Rename(runRoot+"/result.stage", runRoot+"/result.frames") }, syncDirectory: func() error { return syncDirectory(runRoot) }})
		returnErr = errors.Join(returnErr, publishErr)
	}()
	var fixtureID fixtureIdentity
	if err := json.NewDecoder(fixtureOutput).Decode(&fixtureID); err != nil {
		return err
	}
	if fixtureID.PID != fixture.Process.Pid {
		return fmt.Errorf("fixture PID mismatch")
	}
	if err := fixtureStillLive(fixtureID, fixtureFD); err != nil {
		return err
	}
	if err := os.WriteFile(runRoot+"/fixture.json", jsonBytes(fixtureID), 0o400); err != nil {
		return err
	}
	controls := processAttempts(ctx, fixtureID)
	if err := fixtureStillLive(fixtureID, fixtureFD); err != nil {
		return err
	}
	report.Fixture = fixtureID
	report.ProcessControls = controls
	if err := validateProcessAttempts(controls, fixtureID, true); err != nil {
		return err
	}
	if err := os.WriteFile(runRoot+"/process-controls.json", jsonBytes(controls), 0o400); err != nil {
		return err
	}
	for _, name := range cases {
		result, err := runCase(ctx, name, fixtureID, fixtureFD, value.ExpectedVersions[name])
		report.Cases[name] = result
		if err != nil {
			return err
		}
	}
	return nil
}

func runCase(ctx context.Context, name string, fixture fixtureIdentity, fixtureFD int, expectedVersion string) (result outcome, returnErr error) {
	base := runRoot + "/" + name
	if err := os.Mkdir(base, 0o700); err != nil {
		return result, err
	}
	if err := createFixture(base + "/cache"); err != nil {
		return result, err
	}
	if err := createFixture(base + "/control"); err != nil {
		return result, err
	}
	for _, root := range []string{base + "/cache", base + "/control"} {
		if err := prepareACL(root); err != nil {
			return result, err
		}
	}
	aclControls := aclChanges(base + "/control")
	if err := validateACL(aclControls, true); err != nil {
		return result, err
	}
	if err := os.WriteFile(base+"/acl-controls.json", jsonBytes(aclControls), 0o400); err != nil {
		return result, err
	}
	if err := os.Mkdir(base+"/metadata", 0o700); err != nil {
		return result, err
	}
	control := mutations(ctx, base+"/control", binaryPath)
	if len(control) != 13 {
		return result, fmt.Errorf("incomplete controls")
	}
	for operation, attempt := range control {
		if !attempt.OK {
			return result, fmt.Errorf("positive control inconclusive: %s: %+v", operation, attempt)
		}
	}
	if err := os.WriteFile(base+"/controls.json", jsonBytes(control), 0o400); err != nil {
		return result, err
	}
	before, err := snapshot(base + "/cache")
	if err != nil {
		return result, err
	}
	if err := os.WriteFile(base+"/cache-before.json", jsonBytes(before), 0o400); err != nil {
		return result, err
	}
	oldManifest := []byte(`{"schema":"synthetic-old-manifest.v0"}`)
	oldReceipt := []byte(`{"schema":"synthetic-old-receipt.v0"}`)
	if name == "concurrent" {
		oldReceipt = receiptBytes("previous-transaction", "previous-epoch", digest(oldManifest))
	}
	if err := os.WriteFile(base+"/metadata/manifest.json", oldManifest, 0o600); err != nil {
		return result, err
	}
	if err := os.WriteFile(base+"/metadata/receipt.json", oldReceipt, 0o600); err != nil {
		return result, err
	}
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		return result, err
	}
	listenerFile, err := listener.File()
	if err != nil {
		_ = listener.Close()
		return result, err
	}
	_ = listener.Close()
	supervisor := command(ctx, []string{binaryPath, "supervisor", name})
	supervisor.ExtraFiles = []*os.File{listenerFile}
	input, err := supervisor.StdinPipe()
	if err != nil {
		_ = listenerFile.Close()
		return result, err
	}
	output, err := supervisor.StdoutPipe()
	if err != nil {
		_ = listenerFile.Close()
		_ = input.Close()
		return result, err
	}
	var stderr limitedBuffer
	supervisor.Stderr = &stderr
	if err := supervisor.Start(); err != nil {
		_ = listenerFile.Close()
		_ = input.Close()
		_ = output.Close()
		return result, err
	}
	_ = listenerFile.Close()
	pidfd, err := unix.PidfdOpen(supervisor.Process.Pid, 0)
	if err != nil {
		_ = input.Close()
		_ = supervisor.Wait()
		return result, err
	}
	defer func() {
		_ = input.Close()
		waitErr := supervisor.Wait()
		polls := []unix.PollFd{{Fd: int32(pidfd), Events: unix.POLLIN}}
		count, pollErr := unix.Poll(polls, 0)
		_ = unix.Close(pidfd)
		result.SupervisorReaped = waitErr == nil && pollErr == nil && count == 1
		if !result.SupervisorReaped {
			returnErr = errors.Join(returnErr, fmt.Errorf("supervisor terminal witness failed: %w: %s", waitErr, stderr.String()))
		}
	}()
	var actual identity
	if err := receive(output, &actual); err != nil {
		return result, err
	}
	if actual.PID != supervisor.Process.Pid {
		return result, fmt.Errorf("supervisor PID mismatch")
	}
	hash, err := fileHash(binaryPath)
	if err != nil || actual.Hash != hash {
		return result, fmt.Errorf("supervisor executable hash")
	}
	nonce, err := randomID()
	if err != nil {
		return result, err
	}
	channel, err := randomID()
	if err != nil {
		return result, err
	}
	now := mono()
	if now < 0 {
		return result, fmt.Errorf("monotonic unavailable")
	}
	request := request{Case: name, Identity: actual, ExpectedVersion: expectedVersion, Binding: binding{Protocol: domain, Transaction: "combined-r6-" + name, Nonce: nonce, Channel: channel, Epoch: actual.Instance, Deadline: now + 20_000_000_000}, LeaseDeadline: now + 22_000_000_000, PreManifest: digest(oldManifest), PreReceipt: digest(oldReceipt)}
	request.Binding.Request = digest(jsonBytes(request))
	if err := os.WriteFile(base+"/request.json", jsonBytes(request), 0o400); err != nil {
		return result, err
	}
	verifier := command(ctx, []string{binaryPath, "verifier", name})
	var verifierOutput, verifierError limitedBuffer
	verifier.Stdout = &verifierOutput
	verifier.Stderr = &verifierError
	verifierErr := verifier.Run()
	if err := os.WriteFile(base+"/verifier-output.bin", verifierOutput.Bytes(), 0o400); err != nil {
		return result, err
	}
	if err := os.WriteFile(base+"/verifier-stderr.txt", verifierError.Bytes(), 0o400); err != nil {
		return result, err
	}
	if err := receive(&verifierOutput, &result); err != nil {
		return result, fmt.Errorf("missing verifier observation: %w: %w", err, verifierErr)
	}
	result.VerifierExit = verifier.ProcessState.ExitCode()
	result.ACLControls = aclControls
	if verifierErr != nil && (name == "success" || name == "concurrent") {
		return result, fmt.Errorf("positive verifier failed: %w", verifierErr)
	}
	if verifierErr == nil && (name == "before-loss" || name == "after-loss") {
		return result, fmt.Errorf("fault unexpectedly returned success")
	}
	if name != "unsupported" && name != "version-mismatch" && name != "same-build-impostor" && name != "wrong-image" {
		if err := validateBoundary(result.Boundary, fixture); err != nil {
			return result, err
		}
		if err := fixtureStillLive(fixture, fixtureFD); err != nil {
			return result, err
		}
		if result.PostControls == nil {
			result.PostControlsStartedAt = mono()
			controlCtx, cancel := context.WithTimeout(ctx, time.Duration(controlFreshnessNS))
			result.PostControls = processAttempts(controlCtx, fixture)
			cancel()
			result.PostControlsCompletedAt = mono()
		}
		if err := fixtureStillLive(fixture, fixtureFD); err != nil {
			return result, err
		}
		if err := validateControlRefresh(result, fixture); err != nil {
			return result, err
		}
		if err := os.WriteFile(base+"/refreshed-process-controls.json", jsonBytes(result.PostControls), 0o400); err != nil {
			return result, err
		}
	}
	after, err := snapshot(base + "/cache")
	if err != nil {
		return result, err
	}
	result.CacheUnchanged = reflect.DeepEqual(before, after)
	if err := os.WriteFile(base+"/cache-after.json", jsonBytes(after), 0o400); err != nil {
		return result, err
	}
	result.Pair, err = pairOnDisk(base)
	if err != nil {
		return result, err
	}
	if result.Pair {
		result.Committed = true
		if verifierErr != nil {
			result.Status = "committed_evidence_incomplete"
		}
	}
	return result, nil
}

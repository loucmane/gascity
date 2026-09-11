// Command combinedprobe preserves the synthetic verifier/writer diagnostic harness.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type acceptanceEvidence struct {
	Mismatch          *identityMismatch `json:"mismatch,omitempty"`
	Phase             string            `json:"phase,omitempty"`
	Action            string            `json:"action,omitempty"`
	ObservedAt        int64             `json:"observed_at,omitempty"`
	Original          identity          `json:"original"`
	Replacement       identity          `json:"replacement"`
	Terminal          bool              `json:"terminal"`
	ReplacementReaped bool              `json:"replacement_reaped"`
	Refusal           string            `json:"refusal,omitempty"`
	OldReaders        int               `json:"old_readers"`
	MixedReaders      int               `json:"mixed_readers"`
	NewReaders        int               `json:"new_readers"`
	CompetingRefusal  string            `json:"competing_refusal,omitempty"`
	FaultErrno        int               `json:"fault_errno,omitempty"`
	CrossingRefused   bool              `json:"crossing_refused"`
	Deadline          int64             `json:"deadline,omitempty"`
}

func writerFault(value request, phase string) error {
	if faultPhase(value.Case) != phase {
		return nil
	}
	action := strings.TrimSuffix(strings.TrimSuffix(value.Case, "-before"), "-after")
	if action != "crash" && action != "fsync" {
		return nil
	}
	proof := acceptanceEvidence{Phase: phase, Action: action, ObservedAt: mono()}
	if action == "fsync" {
		fd, err := unix.Open("/metadata", unix.O_PATH|unix.O_CLOEXEC, 0)
		if err != nil {
			return err
		}
		syncErr := unix.Fsync(fd)
		closeErr := unix.Close(fd)
		if !errors.Is(syncErr, unix.EBADF) || closeErr != nil {
			return errors.Join(fmt.Errorf("declared O_PATH fsync fault not observed: %w", syncErr), closeErr)
		}
		proof.FaultErrno = int(unix.EBADF)
		proof.Refusal = "fixture O_PATH directory fsync EBADF"
	} else {
		proof.Refusal = "fixture deliberate exit23"
	}
	message := frameFor(value, "FAULT_READY", 7, "")
	message.Evidence = string(jsonBytes(proof))
	if err := send(os.Stdout, message); err != nil {
		return err
	}
	if action == "crash" {
		os.Exit(23)
	}
	return errors.New(proof.Refusal)
}

func validateAcceptance(name string, result outcome) error {
	proof := result.Acceptance
	if name == "concurrent" {
		if proof.OldReaders != 4 || proof.MixedReaders != 4 || proof.NewReaders != 4 || !proof.CrossingRefused || proof.CompetingRefusal != "lease occupied; renewal refused" {
			return fmt.Errorf("concurrent reader/competitor evidence incomplete")
		}
		return nil
	}
	if name == "same-build-impostor" || name == "wrong-image" {
		if err := expectedIdentityMismatch(name, proof.Original, proof.Replacement, proof.Mismatch); err != nil {
			return err
		}
		if proof.Refusal != proof.Mismatch.Error() {
			return fmt.Errorf("identity refusal does not match observed mismatch")
		}
		if proof.Phase != "identity" || proof.Action != name || !proof.ReplacementReaped || proof.Original != result.Version.Process || proof.Original.PID <= 0 || proof.Replacement.PID <= 0 || proof.Original.PID == proof.Replacement.PID || proof.Refusal == "" || result.Error != proof.Refusal {
			return fmt.Errorf("identity negative evidence incomplete")
		}
		if (name == "wrong-image") == (proof.Original.Hash == proof.Replacement.Hash) {
			return fmt.Errorf("wrong-image/same-build evidence confused")
		}
		if result.Committed || result.Pair || result.WriterState != "writer_not_started" || result.VerifierExit != 0 || result.Status != "aborted_preserved" || result.LastStep != "identity" {
			return fmt.Errorf("identity negative changed metadata")
		}
		return nil
	}
	phase := faultPhase(name)
	if phase == "" {
		return nil
	}
	action := strings.TrimSuffix(strings.TrimSuffix(name, "-before"), "-after")
	if proof.Phase != phase || result.LastStep != phase || proof.Action != action || proof.ObservedAt <= 0 || proof.Refusal == "" || !strings.Contains(result.Error, proof.Refusal) || result.VerifierExit != 1 {
		return fmt.Errorf("phase fault evidence missing: %s", name)
	}
	committed := phase == "published"
	if result.Committed != committed || result.Pair != committed || result.Status != disposition(committed, errors.New("fault")) {
		return fmt.Errorf("irreversible fault misclassified")
	}
	if action == "exit" || action == "restart" {
		if !proof.Terminal {
			return fmt.Errorf("real exit not proven")
		}
	}
	if (action == "exit" || action == "exec" || action == "restart") && proof.Original != result.Version.Process {
		return fmt.Errorf("generation evidence targets wrong process")
	}
	if action == "restart" && (!proof.ReplacementReaped || proof.Replacement.PID <= 0 || proof.Replacement.PID == proof.Original.PID || proof.Replacement.Instance == proof.Original.Instance) {
		return fmt.Errorf("restart generation not proven")
	}
	if action == "exec" && (proof.Replacement.PID != proof.Original.PID || proof.Replacement.Start != proof.Original.Start || proof.Replacement.Instance == proof.Original.Instance || proof.Replacement.Hash != proof.Original.Hash) {
		return fmt.Errorf("same-PID exec generation not proven")
	}
	if action == "expiry" && (proof.Deadline <= 0 || proof.ObservedAt < proof.Deadline) {
		return fmt.Errorf("expiry deadline not observed")
	}
	if action == "eof" && !strings.Contains(result.Error, "EOF") {
		return fmt.Errorf("writer EOF not observed")
	}
	if action == "crash" && result.WriterExit != 23 {
		return fmt.Errorf("writer crash exit not observed")
	}
	if action == "fsync" && (proof.FaultErrno != int(unix.EBADF) || result.WriterExit != 1) {
		return fmt.Errorf("fsync adapter fault not observed")
	}
	return nil
}

func identityNegative(ctx context.Context, value request) (acceptanceEvidence, error) {
	proof := acceptanceEvidence{Phase: "identity", Action: value.Case, Original: value.Identity}
	image := binaryPath
	if value.Case == "wrong-image" {
		image = impostorPath
	}
	other, finish, err := replacementFixture(ctx, image)
	if err != nil {
		return proof, err
	}
	proof.Replacement = other
	targetPID := value.Identity.PID
	if value.Case == "wrong-image" {
		targetPID = other.PID
	}
	fd, err := unix.PidfdOpen(targetPID, 0)
	if err != nil {
		return proof, errors.Join(err, finish())
	}
	probe := value
	probe.Identity.Port = other.Port
	if value.Case == "wrong-image" {
		probe.Identity = other
	}
	refusal := verifyHost(ctx, probe, fd, "health")
	closeErr := unix.Close(fd)
	finishErr := finish()
	proof.ReplacementReaped = finishErr == nil
	mismatch, classifyErr := classifyIdentityMismatch(ctx, value.Case, value.Identity, other, refusal)
	if classifyErr != nil {
		return proof, errors.Join(classifyErr, closeErr, finishErr)
	}
	proof.Mismatch = mismatch
	proof.Refusal = mismatch.Error()
	proof.ObservedAt = mono()
	return proof, errors.Join(closeErr, finishErr)
}

func concurrentReaders(ctx context.Context, base string, want bool) (int, error) {
	results := make(chan error, 4)
	start := make(chan struct{})
	for index := 0; index < 4; index++ {
		go func() {
			<-start
			pair, err := pairOnDisk(base)
			if err == nil && pair != want {
				err = fmt.Errorf("concurrent pair reader accepted wrong phase")
			}
			results <- err
		}()
	}
	close(start)
	for count := 0; count < 4; count++ {
		select {
		case err := <-results:
			if err != nil {
				return count, err
			}
		case <-ctx.Done():
			return count, ctx.Err()
		}
	}
	return 4, nil
}

func crossingReader(ctx context.Context, base string) (func() error, error) {
	first, err := os.ReadFile(base + "/metadata/receipt.json")
	if err != nil {
		return nil, err
	}
	release := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		select {
		case <-release:
		case <-ctx.Done():
			finished <- ctx.Err()
			return
		}
		manifest, err := os.ReadFile(base + "/metadata/manifest.json")
		if err != nil {
			finished <- err
			return
		}
		second, err := os.ReadFile(base + "/metadata/receipt.json")
		if err != nil {
			finished <- err
			return
		}
		if string(first) == string(second) {
			finished <- fmt.Errorf("crossing reader did not cross receipt rename")
			return
		}
		if committedPair(first, manifest, second) == nil {
			finished <- fmt.Errorf("crossing reader accepted torn pair")
			return
		}
		finished <- nil
	}()
	return func() error {
		close(release)
		select {
		case err := <-finished:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}, nil
}

func competingLease(ctx context.Context, value request) (string, error) {
	competitor := leaseFor(value)
	competitor.Transaction += "-competitor"
	competitor.Nonce += "-competitor"
	reply, err := query(ctx, value.Identity, "lease-begin", competitor)
	if err == nil || reply.Identity != value.Identity || reply.Lease != competitor || reply.Error != "lease occupied; renewal refused" {
		return "", fmt.Errorf("competing lease refusal not attributed: %w", err)
	}
	return reply.Error, nil
}

func acceptanceVersions() map[string]string {
	result := map[string]string{}
	for _, name := range cases {
		result[name] = version
	}
	result["version-mismatch"] = "ga-tmgr-expected-version-negative-v0"
	return result
}

func faultPhase(name string) string {
	if strings.HasSuffix(name, "-before") {
		return "receipt-ready"
	}
	if strings.HasSuffix(name, "-after") {
		return "published"
	}
	return ""
}

func waitTerminal(ctx context.Context, descriptor int) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		polls := []unix.PollFd{{Fd: int32(descriptor), Events: unix.POLLIN}}
		count, err := unix.Poll(polls, 50)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if count == 1 && polls[0].Revents&unix.POLLIN != 0 {
			return nil
		}
	}
}

func replacementFixture(ctx context.Context, executable string) (identity, func() error, error) {
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return identity{}, nil, err
	}
	file, err := listener.File()
	_ = listener.Close()
	if err != nil {
		return identity{}, nil, err
	}
	child := command(ctx, []string{executable, "supervisor", "success"})
	child.ExtraFiles = []*os.File{file}
	input, err := child.StdinPipe()
	if err != nil {
		_ = file.Close()
		return identity{}, nil, err
	}
	output, err := child.StdoutPipe()
	if err != nil {
		_ = input.Close()
		_ = file.Close()
		return identity{}, nil, err
	}
	var stderr limitedBuffer
	child.Stderr = &stderr
	if err := child.Start(); err != nil {
		_ = input.Close()
		_ = output.Close()
		_ = file.Close()
		return identity{}, nil, err
	}
	_ = file.Close()
	fd, fdErr := unix.PidfdOpen(child.Process.Pid, 0)
	finish := func() error {
		_ = input.Close()
		waitErr := child.Wait()
		if fdErr != nil {
			return errors.Join(waitErr, fdErr)
		}
		polls := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		count, pollErr := unix.Poll(polls, 0)
		closeErr := unix.Close(fd)
		if count != 1 || polls[0].Revents&unix.POLLIN == 0 {
			return errors.Join(waitErr, pollErr, closeErr, fmt.Errorf("replacement terminal evidence absent"))
		}
		return errors.Join(waitErr, pollErr, closeErr)
	}
	if fdErr != nil {
		return identity{}, nil, finish()
	}
	var actual identity
	if err := receive(output, &actual); err != nil {
		return identity{}, nil, errors.Join(err, finish())
	}
	if actual.PID != child.Process.Pid {
		return identity{}, nil, errors.Join(fmt.Errorf("replacement PID mismatch"), finish())
	}
	return actual, finish, nil
}

func lifecycleFault(ctx context.Context, value request, pidfd int, phase string, proof *acceptanceEvidence) error {
	if faultPhase(value.Case) != phase {
		return nil
	}
	action := strings.TrimSuffix(strings.TrimSuffix(value.Case, "-before"), "-after")
	if action != "exit" && action != "exec" && action != "restart" && action != "expiry" && action != "eof" {
		return nil
	}
	*proof = acceptanceEvidence{Phase: phase, Action: action, Original: value.Identity, Deadline: value.Binding.Deadline}
	if action == "expiry" {
		remaining := value.Binding.Deadline - mono() + 1
		if remaining > 0 {
			timer := time.NewTimer(time.Duration(remaining))
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		proof.ObservedAt = mono()
		if proof.ObservedAt < value.Binding.Deadline {
			return fmt.Errorf("expiry not observed")
		}
		proof.Refusal = "request deadline expired at phase"
		return errors.New(proof.Refusal)
	}
	if action == "eof" {
		proof.ObservedAt = mono()
		proof.Refusal = "fixture channel closed at phase"
		return errors.New(proof.Refusal)
	}
	endpoint := "fixture-exit"
	if action == "exec" {
		endpoint = "fixture-exec"
	}
	if _, err := query(ctx, value.Identity, endpoint, leaseFor(value)); err != nil {
		return err
	}
	if action == "exec" {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			polls := []unix.PollFd{{Fd: int32(pidfd), Events: unix.POLLIN}}
			count, pollErr := unix.Poll(polls, 0)
			if err := requireExecGenerationAlive(count, polls[0].Revents, pollErr); err != nil {
				return err
			}
			reply, err := query(ctx, value.Identity, "health", leaseFor(value))
			if err != nil && reply.Identity.PID == value.Identity.PID && reply.Identity.Instance != "" && reply.Identity.Instance != value.Identity.Instance {
				proof.Replacement = reply.Identity
				break
			}
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	} else {
		if err := waitTerminal(ctx, pidfd); err != nil {
			return err
		}
		proof.Terminal = true
		if action == "restart" {
			replacement, finish, err := replacementFixture(ctx, binaryPath)
			if err != nil {
				return err
			}
			proof.Replacement = replacement
			err = finish()
			proof.ReplacementReaped = err == nil
			if err != nil {
				return err
			}
		}
	}
	proof.ObservedAt = mono()
	err := verifyHost(ctx, value, pidfd, "lease-check")
	if err == nil {
		return fmt.Errorf("generation fault failed to refuse")
	}
	proof.Refusal = err.Error()
	return err
}

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type boundaryReport struct {
	ACL                map[string]attempt `json:"acl"`
	AttacksCompletedAt int64              `json:"attacks_completed_at"`
	Mutations          map[string]attempt `json:"mutations"`
	Attacks            map[string]attempt `json:"attacks"`
	Namespaces         map[string]string  `json:"namespaces"`
	FDs                map[string]string  `json:"fds"`
	ReadLock           bool               `json:"read_lock"`
	VerifierHidden     bool               `json:"verifier_hidden"`
}
type helloEvidence struct {
	Fixture     fixtureIdentity `json:"fixture"`
	VerifierPID int             `json:"verifier_pid"`
}

func leaseFor(value request) leaseRequest {
	return leaseRequest{value.Binding.Transaction, value.Binding.Nonce, value.Binding.Request, value.LeaseDeadline}
}

func frameFor(value request, kind string, sequence int, hash string) frame {
	return frame{Binding: value.Binding, Kind: kind, Sequence: sequence, Digest: hash}
}

func verifyHost(ctx context.Context, value request, pidfd int, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	before, err := procIdentity(value.Identity.PID, value.Identity.Port, value.Identity.Instance)
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if err != nil {
		return err
	}
	if before != value.Identity {
		return fmt.Errorf("host identity drift")
	}
	polls := []unix.PollFd{{Fd: int32(pidfd), Events: unix.POLLIN}}
	count, err := unix.Poll(polls, 0)
	if err != nil || count != 0 {
		return fmt.Errorf("supervisor lost")
	}
	_, queryErr := query(ctx, value.Identity, path, leaseFor(value))
	after, err := procIdentity(value.Identity.PID, value.Identity.Port, value.Identity.Instance)
	if err != nil || after != before {
		return fmt.Errorf("host identity changed during API check")
	}
	count, err = unix.Poll(polls, 0)
	if err != nil || count != 0 {
		return fmt.Errorf("supervisor lost after API check")
	}
	return queryErr
}

func runVerifier(name string) (returnErr error) {
	if !knownCase(name) {
		return fmt.Errorf("unknown case")
	}
	value, err := readRequest(runRoot + "/" + name + "/request.json")
	if err != nil {
		return err
	}
	for space, expected := range hostSpaces {
		actual, err := os.Readlink("/proc/self/ns/" + space)
		if err != nil || actual != expected {
			return fmt.Errorf("verifier not in host context")
		}
	}
	if _, err := fdInventory(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	pidfd, err := unix.PidfdOpen(value.Identity.PID, 0)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(pidfd) }()
	if err := verifyHost(ctx, value, pidfd, "health"); err != nil {
		return err
	}
	versionArgv := observedVersionArgv(value.Identity.PID)
	output, versionErr := boundedCommand(ctx, versionArgv, nil)
	if err := verifyHost(ctx, value, pidfd, "health"); err != nil {
		return err
	}
	versionProof := versionEvidence{Executable: versionArgv[0], Expected: value.ExpectedVersion, Observed: string(output), Process: value.Identity}
	if err := compareVersion(value.ExpectedVersion, output, versionErr); err != nil {
		var mismatch *versionMismatch
		if name == "version-mismatch" && errors.As(err, &mismatch) {
			return send(os.Stdout, outcome{Status: "aborted_preserved", Error: err.Error(), WriterState: "writer_not_started", WriterExit: -1, LastStep: "version", Version: versionProof})
		}
		return err
	}
	if err := verifyHost(ctx, value, pidfd, "lease-begin"); err != nil {
		var refusal *apiStatusError
		if name == "unsupported" && errors.As(err, &refusal) && absentLeaseRefusal(refusal) {
			return send(os.Stdout, outcome{Status: "aborted_preserved", Error: err.Error(), WriterState: "writer_not_started", WriterExit: -1, LastStep: "lease-begin", Version: versionProof, APIRefusal: refusal})
		}
		return err
	}
	if name == "same-build-impostor" || name == "wrong-image" {
		proof, err := identityNegative(ctx, value)
		if err != nil {
			return err
		}
		return send(os.Stdout, outcome{Status: "aborted_preserved", Error: proof.Refusal, WriterState: "writer_not_started", WriterExit: -1, LastStep: "identity", Version: versionProof, Acceptance: proof})
	}
	cmd := command(ctx, writerArgv(name))
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	outputPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = input.Close()
		return err
	}
	var stderr limitedBuffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		_ = input.Close()
		_ = outputPipe.Close()
		return err
	}
	result := outcome{Status: "aborted_preserved", WriterState: "writer_started", WriterPID: cmd.Process.Pid, WriterExit: -1, Version: versionProof}
	writerPIDFD, pidfdErr := unix.PidfdOpen(cmd.Process.Pid, 0)
	defer func() {
		_ = input.Close()
		waitErr := cmd.Wait()
		var exited *exec.ExitError
		if cmd.ProcessState != nil {
			result.WriterWaitPID = cmd.ProcessState.Pid()
			result.WriterWaitObserved = waitErr == nil || errors.As(waitErr, &exited)
			result.WriterExit = cmd.ProcessState.ExitCode()
		}
		if pidfdErr == nil {
			polls := []unix.PollFd{{Fd: int32(writerPIDFD), Events: unix.POLLIN}}
			count, pollErr := unix.Poll(polls, 0)
			result.WriterPIDFDTerminal = pollErr == nil && count == 1 && polls[0].Revents&unix.POLLIN != 0
			if closeErr := unix.Close(writerPIDFD); closeErr != nil {
				returnErr = errors.Join(returnErr, closeErr)
			}
		}
		result.WriterReaped = writerTerminalEvidence(result.WriterPID, result.WriterWaitPID, result.WriterWaitObserved, result.WriterPIDFDTerminal)
		if result.WriterReaped {
			result.WriterState = "writer_terminal"
		} else {
			returnErr = errors.Join(returnErr, fmt.Errorf("writer terminal evidence incomplete"))
		}
		if waitErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("writer exit refused: %w: %s", waitErr, stderr.String()))
		}
		if returnErr != nil {
			result.Status = disposition(result.Committed, returnErr)
			result.Error = returnErr.Error()
		}
		if err := send(os.Stdout, result); err != nil {
			returnErr = errors.Join(returnErr, err)
		}
	}()
	if pidfdErr != nil {
		return fmt.Errorf("writer pidfd unavailable: %w", pidfdErr)
	}
	fixtureData, err := os.ReadFile(runRoot + "/fixture.json")
	if err != nil {
		return err
	}
	var fixture fixtureIdentity
	if err := strictJSON(fixtureData, &fixture); err != nil {
		return err
	}
	hello := frameFor(value, "HELLO", 1, "")
	hello.Evidence = string(jsonBytes(helloEvidence{fixture, os.Getpid()}))
	if err := send(input, hello); err != nil {
		return err
	}
	var observed frame
	if err := receive(outputPipe, &observed); err != nil {
		return err
	}
	if err := checkFrame(observed, value.Binding, "OBSERVED", 2, mono()); err != nil {
		return err
	}
	if err := strictJSON([]byte(observed.Evidence), &result.Boundary); err != nil {
		return err
	}
	if err := validateBoundary(result.Boundary, fixture); err != nil {
		return err
	}
	if strings.HasPrefix(name, "expiry-") {
		fixtureFD, err := unix.PidfdOpen(fixture.PID, 0)
		if err != nil {
			return err
		}
		defer func() { _ = unix.Close(fixtureFD) }()
		if err := fixtureStillLive(fixture, fixtureFD); err != nil {
			return err
		}
		result.PostControlsStartedAt = mono()
		result.PostControls = processAttempts(ctx, fixture)
		result.PostControlsCompletedAt = mono()
		if err := fixtureStillLive(fixture, fixtureFD); err != nil {
			return err
		}
		if err := validateControlRefresh(result, fixture); err != nil {
			return err
		}
	}
	var prepared frame
	if err := receive(outputPipe, &prepared); err != nil {
		return err
	}
	if err := checkFrame(prepared, value.Binding, "PREPARED", 3, mono()); err != nil {
		return err
	}
	manifest, receipt := postimages(value)
	if name == "concurrent" {
		result.Acceptance.OldReaders, err = concurrentReaders(ctx, runRoot+"/"+name, true)
		if err != nil {
			return err
		}
		result.Acceptance.CompetingRefusal, err = competingLease(ctx, value)
		if err != nil {
			return err
		}
	}
	if prepared.Digest != digest(append(append([]byte{}, manifest...), receipt...)) {
		return fmt.Errorf("prepared write set differs")
	}
	if err := verifyHost(ctx, value, pidfd, "lease-check"); err != nil {
		return err
	}
	if err := send(input, frameFor(value, "COMMIT_GRANT", 4, prepared.Digest)); err != nil {
		return err
	}
	var staged frame
	if err := receive(outputPipe, &staged); err != nil {
		return err
	}
	if err := checkFrame(staged, value.Binding, "RECEIPT_READY", 5, mono()); err != nil {
		return err
	}
	if staged.Digest != digest(receipt) {
		return fmt.Errorf("receipt digest")
	}
	result.LastStep = "receipt-ready"
	var finishCrossing func() error
	if name == "concurrent" {
		finishCrossing, err = crossingReader(ctx, runRoot+"/"+name)
		if err != nil {
			return err
		}
		result.Acceptance.MixedReaders, err = concurrentReaders(ctx, runRoot+"/"+name, false)
		if err != nil {
			return err
		}
	}
	if err := lifecycleFault(ctx, value, pidfd, "receipt-ready", &result.Acceptance); err != nil {
		return err
	}
	if err := verifyHost(ctx, value, pidfd, "lease-check"); err != nil {
		return err
	}
	if err := send(input, frameFor(value, "RECEIPT_GRANT", 6, digest(receipt))); err != nil {
		return err
	}
	var published frame
	if err := receive(outputPipe, &published); err != nil {
		return err
	}
	if published.Kind == "FAULT_READY" {
		if err := checkFrame(published, value.Binding, "FAULT_READY", 7, mono()); err != nil {
			return err
		}
		if err := strictJSON([]byte(published.Evidence), &result.Acceptance); err != nil {
			return err
		}
		if result.Acceptance.Phase != faultPhase(name) || (result.Acceptance.Action != "crash" && result.Acceptance.Action != "fsync") {
			return fmt.Errorf("unexpected fault witness")
		}
		result.LastStep = result.Acceptance.Phase
		return errors.New(result.Acceptance.Refusal)
	}
	if published.Kind == "PUBLISHED" && published.Binding == value.Binding && published.Sequence == 7 && published.Digest == digest(receipt) {
		result.Committed = true
	}
	if err := checkFrame(published, value.Binding, "PUBLISHED", 7, mono()); err != nil {
		return err
	}
	if !result.Committed {
		return fmt.Errorf("publication mismatch")
	}
	result.LastStep = "published"
	if name == "concurrent" {
		if err := finishCrossing(); err != nil {
			return err
		}
		result.Acceptance.CrossingRefused = true
		result.Acceptance.NewReaders, err = concurrentReaders(ctx, runRoot+"/"+name, true)
		if err != nil {
			return err
		}
	}
	if err := lifecycleFault(ctx, value, pidfd, "published", &result.Acceptance); err != nil {
		return err
	}
	if err := verifyHost(ctx, value, pidfd, "lease-check"); err != nil {
		return err
	}
	if err := send(input, frameFor(value, "FINAL", 8, digest(receipt))); err != nil {
		return err
	}
	result.Status = "PASS"
	result.LastStep = "final"
	return nil
}

func postimages(value request) ([]byte, []byte) {
	manifest := []byte(`{"schema":"synthetic-manifest.v1","value":"new"}`)
	return manifest, receiptBytes(value.Binding.Transaction, value.Binding.Epoch, digest(manifest))
}

func knownCase(name string) bool {
	for _, candidate := range cases {
		if name == candidate {
			return true
		}
	}
	return false
}

func validateBoundary(report boundaryReport, fixture fixtureIdentity) error {
	if err := validateACL(report.ACL, false); err != nil {
		return err
	}
	if len(report.Mutations) != 13 || len(report.Attacks) != 4 || !report.ReadLock || !report.VerifierHidden {
		return fmt.Errorf("incomplete boundary")
	}
	for _, name := range []string{"create", "overwrite", "truncate", "unlink", "rename", "mkdir", "symlink", "chmod", "utime", "descendant_thread", "descendant_process", "chown", "xattr"} {
		value, ok := report.Mutations[name]
		if !ok || value.OK || (value.Errno != 1 && value.Errno != 13 && value.Errno != 30) {
			return fmt.Errorf("cache negative inconclusive: %s", name)
		}
	}
	if err := validateProcessAttempts(report.Attacks, fixture, false); err != nil {
		return err
	}
	if report.AttacksCompletedAt <= 0 {
		return fmt.Errorf("missing negative observation time")
	}
	for _, name := range []string{"user", "pid", "mnt", "net", "ipc"} {
		if report.Namespaces[name] == "" || report.Namespaces[name] == fixture.Namespaces[name] {
			return fmt.Errorf("namespace not isolated")
		}
	}
	if report.Namespaces["time"] != fixture.Namespaces["time"] {
		return fmt.Errorf("monotonic domain changed")
	}
	for name, target := range report.FDs {
		number, err := strconv.Atoi(name)
		if err != nil || (number > 2 && target != "anon_inode:[eventpoll]" && target != "anon_inode:[eventfd]") {
			return fmt.Errorf("extra FD")
		}
	}
	return nil
}

func checkPreimages(value request) error {
	manifest, err := fileHash("/metadata/manifest.json")
	if err != nil {
		return err
	}
	receipt, err := fileHash("/metadata/receipt.json")
	if err != nil {
		return err
	}
	if manifest != value.PreManifest || receipt != value.PreReceipt {
		return fmt.Errorf("metadata preimage drift")
	}
	for _, path := range []string{"/metadata", "/metadata/manifest.json", "/metadata/receipt.json"} {
		var stat unix.Stat_t
		if err := unix.Lstat(path, &stat); err != nil {
			return err
		}
		if stat.Mode&unix.S_IFMT == unix.S_IFLNK || stat.Nlink > 1 && stat.Mode&unix.S_IFMT != unix.S_IFDIR {
			return fmt.Errorf("aliased metadata")
		}
	}
	return nil
}

func stage(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	written, writeErr := file.Write(data)
	if writeErr == nil && written != len(data) {
		writeErr = io.ErrShortWrite
	}
	modeErr := file.Chmod(0o400)
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(writeErr, modeErr, syncErr, closeErr)
}

func syncDirectory(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}

func runWriter() (returnErr error) {
	value, err := readRequest("/request.json")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 23*time.Second)
	defer cancel()
	var hello frame
	if err := receive(os.Stdin, &hello); err != nil {
		return err
	}
	if err := checkFrame(hello, value.Binding, "HELLO", 1, mono()); err != nil {
		return err
	}
	var evidence helloEvidence
	if err := strictJSON([]byte(hello.Evidence), &evidence); err != nil {
		return err
	}
	fds, err := fdInventory()
	if err != nil {
		return err
	}
	spaces, err := namespaces()
	if err != nil {
		return err
	}
	lock, err := os.Open("/cache/.packman-cache.lock")
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_SH|unix.LOCK_NB); err != nil {
		return err
	}
	report := boundaryReport{Mutations: mutations(ctx, "/cache", "/writer"), Attacks: processAttempts(ctx, evidence.Fixture), Namespaces: spaces, FDs: fds, ReadLock: true}
	report.ACL = aclChanges("/cache")
	report.AttacksCompletedAt = mono()
	_, statErr := os.Stat("/proc/" + strconv.Itoa(evidence.VerifierPID) + "/fd")
	report.VerifierHidden = errors.Is(statErr, os.ErrNotExist)
	if err := validateBoundary(report, evidence.Fixture); err != nil {
		return err
	}
	observed := frameFor(value, "OBSERVED", 2, "")
	observed.Evidence = string(jsonBytes(report))
	if err := send(os.Stdout, observed); err != nil {
		return err
	}
	if err := checkPreimages(value); err != nil {
		return err
	}
	manifest, receipt := postimages(value)
	writeSet := digest(append(append([]byte{}, manifest...), receipt...))
	if err := send(os.Stdout, frameFor(value, "PREPARED", 3, writeSet)); err != nil {
		return err
	}
	var grant frame
	if err := receive(os.Stdin, &grant); err != nil {
		return err
	}
	if err := checkFrame(grant, value.Binding, "COMMIT_GRANT", 4, mono()); err != nil {
		return err
	}
	if grant.Digest != writeSet {
		return fmt.Errorf("write set grant mismatch")
	}
	committed := false
	defer func() {
		if returnErr != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", disposition(committed, returnErr), returnErr)
		}
	}()
	committed, err = publish(transactionOps{Step: func(step string) error {
		if step != "sync" && step != "final" && mono() >= value.Binding.Deadline {
			return fmt.Errorf("grant expired")
		}
		switch step {
		case "stage":
			if err := checkPreimages(value); err != nil {
				return err
			}
			for _, name := range []string{"manifest", "receipt"} {
				data, err := os.ReadFile("/metadata/" + name + ".json")
				if err != nil {
					return err
				}
				if err := stage("/metadata/"+name+".backup", data); err != nil {
					return err
				}
			}
			if err := stage("/metadata/manifest.stage", manifest); err != nil {
				return err
			}
			if err := stage("/metadata/receipt.stage", receipt); err != nil {
				return err
			}
			return syncDirectory("/metadata")
		case "manifest":
			if err := checkPreimages(value); err != nil {
				return err
			}
			if err := os.Rename("/metadata/manifest.stage", "/metadata/manifest.json"); err != nil {
				return err
			}
			return syncDirectory("/metadata")
		case "grant":
			if err := send(os.Stdout, frameFor(value, "RECEIPT_READY", 5, digest(receipt))); err != nil {
				return err
			}
			var final frame
			if err := receive(os.Stdin, &final); err != nil {
				return err
			}
			if err := checkFrame(final, value.Binding, "RECEIPT_GRANT", 6, mono()); err != nil {
				return err
			}
			if final.Digest != digest(receipt) {
				return fmt.Errorf("final grant digest")
			}
			return nil
		case "receipt":
			if err := writerFault(value, "receipt-ready"); err != nil {
				return err
			}
			actual, err := fileHash("/metadata/manifest.json")
			if err != nil || actual != digest(manifest) {
				return fmt.Errorf("manifest drift before commit")
			}
			old, err := fileHash("/metadata/receipt.json")
			if err != nil || old != value.PreReceipt {
				return fmt.Errorf("receipt drift before commit")
			}
			if mono() >= value.Binding.Deadline {
				return fmt.Errorf("deadline at rename")
			}
			return os.Rename("/metadata/receipt.stage", "/metadata/receipt.json")
		case "sync":
			if err := writerFault(value, "published"); err != nil {
				return err
			}
			return syncDirectory("/metadata")
		case "final":
			if err := send(os.Stdout, frameFor(value, "PUBLISHED", 7, digest(receipt))); err != nil {
				return err
			}
			var final frame
			if err := receive(os.Stdin, &final); err != nil {
				return err
			}
			if err := checkFrame(final, value.Binding, "FINAL", 8, mono()); err != nil {
				return err
			}
			if final.Digest != digest(receipt) {
				return fmt.Errorf("final evidence digest")
			}
			if err := stage("/metadata/final-evidence.json", jsonBytes(final)); err != nil {
				return err
			}
			return syncDirectory("/metadata")
		}
		return fmt.Errorf("unknown transaction step")
	}})
	return err
}

func pairOnDisk(base string) (bool, error) {
	first, err := os.ReadFile(base + "/metadata/receipt.json")
	if err != nil {
		return false, err
	}
	manifest, err := os.ReadFile(base + "/metadata/manifest.json")
	if err != nil {
		return false, err
	}
	second, err := os.ReadFile(base + "/metadata/receipt.json")
	if err != nil {
		return false, err
	}
	return committedPair(first, manifest, second) == nil, nil
}

func validateObservation(value observation, hash string) error {
	if value.Kind != "observation-only-requires-outer-exit-zero" || value.Plan != hash || value.Status != "PASS" || len(value.Cases) != len(cases) || !value.FixtureReaped || len(value.ProcessControls) != 4 {
		return fmt.Errorf("incomplete observation")
	}
	if err := validateProcessAttempts(value.ProcessControls, value.Fixture, true); err != nil {
		return err
	}
	for _, name := range cases {
		result, ok := value.Cases[name]
		if !ok || !result.SupervisorReaped || !result.CacheUnchanged {
			return fmt.Errorf("missing terminal/inventory witness")
		}
		if err := validateACL(result.ACLControls, true); err != nil {
			return err
		}
		writerStarted := name != "unsupported" && name != "version-mismatch" && name != "same-build-impostor" && name != "wrong-image"
		if err := validateWriterOutcome(result, writerStarted); err != nil {
			return err
		}
		if result.Version.Expected != expectedVersions[name] || result.Version.Process.PID <= 0 || result.Version.Executable != observedVersionArgv(result.Version.Process.PID)[0] || result.Version.Process.Hash == "" || result.Version.Observed != version+"\n" {
			return fmt.Errorf("version acceptance evidence incomplete")
		}
		if writerStarted {
			if err := validateBoundary(result.Boundary, value.Fixture); err != nil {
				return err
			}
			if err := validateControlRefresh(result, value.Fixture); err != nil {
				return err
			}
		}
		if err := validateAcceptance(name, result); err != nil {
			return err
		}
		switch name {
		case "success", "concurrent":
			if result.Status != "PASS" || !result.Committed || !result.Pair || result.Error != "" || result.LastStep != "final" || result.VerifierExit != 0 || result.WriterExit != 0 {
				return fmt.Errorf("positive transaction failed")
			}
		case "version-mismatch", "unsupported", "before-loss":
			if result.Committed || result.Pair || result.Status != "aborted_preserved" || result.Error == "" {
				return fmt.Errorf("precommit loss accepted")
			}
			if name == "unsupported" && (result.LastStep != "lease-begin" || !absentLeaseRefusal(result.APIRefusal) || result.Error != result.APIRefusal.Error() || result.VerifierExit != 0) {
				return fmt.Errorf("unsupported guard witness absent")
			}
			if name == "version-mismatch" {
				expectedErr := compareVersion(result.Version.Expected, []byte(result.Version.Observed), nil)
				if expectedErr == nil || result.Error != expectedErr.Error() || result.LastStep != "version" || result.VerifierExit != 0 {
					return fmt.Errorf("version mismatch not demonstrated")
				}
			}
			if name == "before-loss" && (result.LastStep != "receipt-ready" || !strings.Contains(result.Error, "lease missing, stale or expired") || result.VerifierExit != 1) {
				return fmt.Errorf("precommit fault not reached")
			}
		case "after-loss":
			if !result.Committed || !result.Pair || result.Status != "committed_evidence_incomplete" || !strings.Contains(result.Error, "lease missing, stale or expired") || result.LastStep != "published" || result.VerifierExit != 1 {
				return fmt.Errorf("postcommit loss incorrectly classified")
			}
		}
	}
	return nil
}

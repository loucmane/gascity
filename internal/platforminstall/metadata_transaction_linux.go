package platforminstall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

var errMetadataOuterDispositionUnknown = errors.New("automatic v2 replay refused: completed outer disposition unavailable; preserve committed evidence for read-only diagnosis")

func metadataNowBefore(binding MetadataBinding) error {
	now, err := metadataMonotonicNow()
	if err != nil {
		return err
	}
	if now >= binding.Deadline {
		return fmt.Errorf("metadata transaction expired")
	}
	return nil
}

func metadataReceive(pipe *os.File, binding MetadataBinding, kind string, sequence int) (metadataFrame, error) {
	var frame metadataFrame
	if err := metadataDecode(pipe, &frame); err != nil {
		return frame, err
	}
	now, err := metadataMonotonicNow()
	if err != nil {
		return frame, err
	}
	return frame, metadataCheckFrame(frame, binding, kind, sequence, now)
}

func metadataSend(pipe *os.File, binding MetadataBinding, kind string, sequence int, digest string, receipt *Receipt, steps []PlanStep) error {
	if err := metadataNowBefore(binding); err != nil {
		return err
	}
	return metadataEncode(pipe, metadataFrame{Binding: binding, Kind: kind, Sequence: sequence, Digest: digest, Receipt: receipt, Steps: steps})
}

func metadataOutputPreimages(manifest Manifest) error {
	for _, pin := range manifest.Metadata.Preimages {
		if err := metadataImageMatches(pin.Path, pin.SHA256, 0); err != nil {
			return fmt.Errorf("metadata preimage %s: %w", pin.Path, err)
		}
	}
	return nil
}

func metadataStage(manifest Manifest, path string) string {
	return filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".metadata-"+manifest.Metadata.Attempt)
}

func metadataOutputLocks(manifest Manifest) (func() error, error) {
	parents := map[string]bool{manifest.Metadata.Evidence: true}
	for _, path := range metadataOutputs(manifest) {
		parents[filepath.Dir(path)] = true
	}
	ordered := make([]string, 0, len(parents))
	for parent := range parents {
		ordered = append(ordered, parent)
	}
	sort.Strings(ordered)
	var files []*os.File
	closeAll := func() error {
		var result error
		for _, file := range files {
			result = errors.Join(result, file.Close())
		}
		files = nil
		return result
	}
	for _, parent := range ordered {
		resolved, err := filepath.EvalSymlinks(parent)
		if err != nil || resolved != parent {
			return nil, errors.Join(fmt.Errorf("metadata parent alias/absence"), err, closeAll())
		}
		descriptor, err := unix.Open(parent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return nil, errors.Join(err, closeAll())
		}
		file := os.NewFile(uintptr(descriptor), parent)
		files = append(files, file)
		if err := unix.Flock(descriptor, unix.LOCK_EX|unix.LOCK_NB); err != nil {
			return nil, errors.Join(fmt.Errorf("competing metadata transaction: %w", err), closeAll())
		}
	}
	return closeAll, nil
}

func metadataPublishFile(installer *installer, manifest Manifest, path string, data []byte, preimage string) (bool, error) {
	publication, err := installer.prepareMetadataPublication(path, metadataStage(manifest, path), data, preimage)
	if err != nil {
		return false, err
	}
	return publication.publish()
}

func metadataRestoreManifest(installer *installer, manifest Manifest, previous []byte, postimage string) (returnErr error) {
	path := DefaultManifestPath(manifest.CityPath)
	if len(previous) > 0 {
		publication, err := installer.prepareMetadataPublication(path, metadataStage(manifest, path)+".rollback", previous, postimage)
		if err != nil {
			return err
		}
		_, err = publication.publish()
		return err
	}
	resolved, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil || resolved != filepath.Dir(path) {
		return fmt.Errorf("rollback parent changed")
	}
	parent, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { returnErr = errors.Join(returnErr, parent.Close()) }()
	anchored := fmt.Sprintf("/proc/self/fd/%d/%s", parent.Fd(), filepath.Base(path))
	if err := metadataImageMatches(anchored, postimage, 0o644); err != nil {
		return err
	}
	if err := unix.Unlinkat(int(parent.Fd()), filepath.Base(path), 0); err != nil {
		return err
	}
	return parent.Sync()
}

func metadataWriterPipes(deadline time.Time) (_ *os.File, _ *os.File, returnErr error) {
	ownedByFiles := false
	defer func() {
		if !ownedByFiles {
			for _, descriptor := range []int{3, 4} {
				if err := unix.Close(descriptor); err != nil && !errors.Is(err, unix.EBADF) {
					returnErr = errors.Join(returnErr, err)
				}
			}
		}
	}()
	// Exec inheritance makes pipes blocking. NewFile must see O_NONBLOCK to
	// register the inherited endpoints with the runtime poller for deadlines.
	for _, descriptor := range []int{3, 4} {
		if err := unix.SetNonblock(descriptor, true); err != nil {
			return nil, nil, fmt.Errorf("initialize metadata pipe %d: %w", descriptor, err)
		}
	}
	input, output := os.NewFile(3, "metadata-request"), os.NewFile(4, "metadata-response")
	ownedByFiles = true
	defer func() {
		if returnErr != nil {
			returnErr = errors.Join(returnErr, input.Close(), output.Close())
		}
	}()
	if err := input.SetReadDeadline(deadline); err != nil {
		return nil, nil, err
	}
	if err := output.SetWriteDeadline(deadline); err != nil {
		return nil, nil, err
	}
	return input, output, nil
}

// MetadataWriterEntrypoint handles the bound request inside the confined writer.
func MetadataWriterEntrypoint() (returnErr error) {
	committed := false
	defer func() { returnErr = metadataCommittedError(returnErr, committed) }()
	deadline := time.Now().Add(25 * time.Second)
	input, output, err := metadataWriterPipes(deadline)
	if err != nil {
		return err
	}
	defer func() { returnErr = errors.Join(returnErr, input.Close(), output.Close()) }()
	var request metadataRequest
	if err := metadataDecode(input, &request); err != nil {
		return err
	}
	manifest, binding := request.Manifest, request.Binding
	if err := metadataRequestPurpose(request); err != nil {
		return err
	}
	if err := validateMetadataLaunch(manifest); err != nil {
		return err
	}
	want, err := ManifestDigest(manifest)
	if err != nil || want != manifest.ManifestSHA256 || binding.Request != want {
		return fmt.Errorf("writer request digest refused")
	}
	if err := validateMetadataBinding(binding); err != nil {
		return err
	}
	now, err := metadataMonotonicNow()
	if err != nil {
		return err
	}
	if now < 0 || binding.Deadline <= now || binding.LeaseEnd-now > 30_000_000_000 {
		return fmt.Errorf("writer grant lifetime refused")
	}
	if binding.Transaction != manifest.Metadata.Transaction || binding.Attempt != manifest.Metadata.Attempt {
		return fmt.Errorf("writer transaction identity differs")
	}
	if err := metadataWriterBoundary(manifest); err != nil {
		return fmt.Errorf("writer boundary validation: %w", err)
	}
	if err := validateRuntimeProof(manifest, request.Proof); err != nil {
		return err
	}
	if _, err := metadataReceive(input, binding, "HELLO", 1); err != nil {
		return err
	}
	if err := metadataSend(output, binding, "OBSERVED", 2, want, nil, nil); err != nil {
		return err
	}
	if _, err := metadataReceive(input, binding, "PREPARE", 3); err != nil {
		return err
	}
	ctx, cancel := context.WithDeadline(context.WithValue(context.Background(), metadataInspectionKey{}, true), deadline)
	defer cancel()
	return withMetadataOnlyCacheLock(func() (returnErr error) {
		unlock, err := metadataOutputLocks(manifest)
		if err != nil {
			return err
		}
		defer func() { returnErr = errors.Join(returnErr, unlock()) }()
		state, err := preflightManifest(manifest)
		if err != nil {
			return err
		}
		if err := metadataReplayDisposition(state.noopReceipt); err != nil {
			return err
		}
		if !state.coreAlreadyInstalled {
			return fmt.Errorf("metadata writer refuses core installation")
		}
		if err := preflightMetadataOnlyArtifacts(manifest, state); err != nil {
			return err
		}
		if report := inspectPinnedIntegrity(ctx, manifest); len(report.Drifts) != 0 {
			return fmt.Errorf("metadata integrity drift: %+v", report.Drifts)
		}
		if err := checkCandidatePackCacheReadOnlyContext(ctx, manifest, state); err != nil {
			return err
		}
		if err := validateMetadataInputs(manifest, false); err != nil {
			return fmt.Errorf("writer prepare input validation: %w", err)
		}
		if request.PlanOnly {
			steps, err := adoptPlan(ctx, manifest, true)
			if err != nil {
				return err
			}
			if err := metadataSend(output, binding, "PLANNED", 4, want, nil, steps); err != nil {
				return err
			}
			_, err = metadataReceive(input, binding, "FINAL", 5)
			return err
		}
		if err := metadataOutputPreimages(manifest); err != nil {
			return err
		}
		previous, _, err := readOptionalRegularFile(DefaultManifestPath(manifest.CityPath), "precommit manifest")
		if err != nil {
			return err
		}
		receipt := Receipt{Schema: metadataReceiptSchema, ReleaseID: manifest.ReleaseID, ManifestSHA256: want, ArtifactSHA256: manifest.Core.SHA256, PreviousSHA256: manifest.PreviousSHA256, ManagedFiles: receiptManagedFiles(manifest.ManagedFiles), Activation: &request.Proof, Result: ResultInstalled, Metadata: &MetadataCommit{Binding: binding, Host: manifest.Metadata.Host}}
		if err := finalizeReceipt(&receipt); err != nil {
			return err
		}
		if err := metadataSend(output, binding, "PREPARED", 4, receipt.ReceiptSHA256, &receipt, nil); err != nil {
			return err
		}
		grant, err := metadataReceive(input, binding, "COMMIT_GRANT", 5)
		if err != nil {
			return err
		}
		if grant.Digest != receipt.ReceiptSHA256 {
			return fmt.Errorf("prepare grant receipt mismatch")
		}
		if err := metadataOutputPreimages(manifest); err != nil {
			return err
		}
		installer := newInstaller()
		journal, err := json.Marshal(request)
		if err != nil {
			return err
		}
		if _, err := metadataPublishFile(installer, manifest, filepath.Join(manifest.Metadata.Evidence, "intent.json"), journal, ""); err != nil {
			return err
		}
		if state.previousMetadata != nil {
			for index, item := range []struct {
				path  string
				data  []byte
				reuse bool
			}{{manifest.PreviousMetadata.ManifestBackupPath, state.previousMetadata.manifest, state.previousMetadata.reuseManifestBackup}, {manifest.PreviousMetadata.ReceiptBackupPath, state.previousMetadata.receipt, state.previousMetadata.reuseReceiptBackup}} {
				if !item.reuse {
					if _, err := metadataPublishFile(installer, manifest, item.path, item.data, manifest.Metadata.Preimages[index+2].SHA256); err != nil {
						return err
					}
				}
			}
		}
		manifestPublished := false
		defer func() {
			if returnErr != nil && !committed && manifestPublished {
				returnErr = errors.Join(returnErr, metadataRestoreManifest(installer, manifest, previous, sha256Hex(state.manifest)))
			}
		}()
		if !state.reuseManifest {
			manifestPublished, err = metadataPublishFile(installer, manifest, DefaultManifestPath(manifest.CityPath), state.manifest, manifest.Metadata.Preimages[0].SHA256)
			if err != nil {
				return err
			}
		}
		receiptData, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		publication, err := installer.prepareMetadataPublication(manifest.ReceiptPath, metadataStage(manifest, manifest.ReceiptPath), receiptData, manifest.Metadata.Preimages[1].SHA256)
		if err != nil {
			return err
		}
		defer func() { returnErr = errors.Join(returnErr, publication.close()) }()
		if err := metadataSend(output, binding, "PUBLISHED", 6, receipt.ReceiptSHA256, &receipt, nil); err != nil {
			return err
		}
		finalGrant, err := metadataReceive(input, binding, "RECEIPT_GRANT", 7)
		if err != nil {
			return err
		}
		if finalGrant.Digest != receipt.ReceiptSHA256 {
			return fmt.Errorf("final receipt grant mismatch")
		}
		if err := validateMetadataInputs(manifest, false); err != nil {
			return fmt.Errorf("writer publication input validation: %w", err)
		}
		if err := metadataNowBefore(binding); err != nil {
			return err
		}
		committed, err = publication.publish()
		if err != nil {
			return err
		}
		if err := metadataSend(output, binding, "COMMITTED", 8, receipt.ReceiptSHA256, &receipt, nil); err != nil {
			return err
		}
		final, err := metadataReceive(input, binding, "FINAL", 9)
		if err != nil {
			return err
		}
		finalData, err := json.Marshal(final)
		if err != nil {
			return err
		}
		if _, err := metadataPublishFile(installer, manifest, filepath.Join(manifest.Metadata.Evidence, "final.json"), finalData, ""); err != nil {
			return err
		}
		_, _, err = ReadCommittedPair(manifest)
		return err
	})
}

// RunMetadataTransaction verifies the host and coordinates one confined metadata transaction.
func RunMetadataTransaction(ctx context.Context, manifest Manifest, planOnly bool) (_ Receipt, _ []PlanStep, returnErr error) {
	var session *metadataHostSession
	committed := false
	defer func() {
		if returnErr != nil && session != nil && !committed {
			if _, receipt, err := ReadCommittedPair(manifest); err == nil && receipt.Metadata != nil && receipt.Metadata.Binding == session.binding {
				committed = true
			}
		}
		returnErr = metadataCommittedError(returnErr, committed)
	}()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := metadataCheckFDs(false); err != nil {
		return Receipt{}, nil, err
	}
	argv, err := metadataSandboxArgv(manifest)
	if err != nil {
		return Receipt{}, nil, err
	}
	if err := validateMetadataInputs(manifest, true); err != nil {
		return Receipt{}, nil, fmt.Errorf("initial host input validation: %w", err)
	}
	session, err = openMetadataHost(ctx, manifest)
	if err != nil {
		return Receipt{}, nil, err
	}
	defer func() { returnErr = errors.Join(returnErr, session.close()) }()
	if err := session.begin(ctx, planOnly); err != nil {
		return Receipt{}, nil, err
	}
	if err := validateMetadataInputs(manifest, true); err != nil {
		return Receipt{}, nil, fmt.Errorf("post-begin host input validation: %w", err)
	}
	childInput, parentOutput, err := os.Pipe()
	if err != nil {
		return Receipt{}, nil, err
	}
	closeChildInput := metadataCloseOnce(childInput.Close)
	closeParentOutput := metadataCloseOnce(parentOutput.Close)
	defer func() { returnErr = errors.Join(returnErr, closeChildInput(), closeParentOutput()) }()
	parentInput, childOutput, err := os.Pipe()
	if err != nil {
		return Receipt{}, nil, err
	}
	closeParentInput := metadataCloseOnce(parentInput.Close)
	closeChildOutput := metadataCloseOnce(childOutput.Close)
	defer func() { returnErr = errors.Join(returnErr, closeParentInput(), closeChildOutput()) }()
	deadline := time.Now().Add(25 * time.Second)
	if err := parentInput.SetReadDeadline(deadline); err != nil {
		return Receipt{}, nil, err
	}
	if err := parentOutput.SetWriteDeadline(deadline); err != nil {
		return Receipt{}, nil, err
	}
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Env = []string{"GODEBUG=containermaxprocs=0"}
	command.ExtraFiles = []*os.File{childInput, childOutput}
	var stdout, stderr metadataBoundedOutput
	command.Stdout = &stdout
	command.Stderr = &stderr
	command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
	command.WaitDelay = 5 * time.Second
	if err := command.Start(); err != nil {
		return Receipt{}, nil, err
	}
	writerPIDFD, pidErr := unix.PidfdOpen(command.Process.Pid, 0)
	defer func() {
		returnErr = metadataFinishChild(returnErr, closeParentOutput, cancel, func() error {
			if err := command.Wait(); err != nil {
				return fmt.Errorf("confined writer failed: %w: %s", err, stderr.String())
			}
			return nil
		}, func() error {
			if pidErr != nil {
				return pidErr
			}
			polls := []unix.PollFd{{Fd: int32(writerPIDFD), Events: unix.POLLIN}}
			count, pollErr := unix.Poll(polls, 0)
			var terminalErr error
			if pollErr != nil || count != 1 || polls[0].Revents&unix.POLLIN == 0 {
				terminalErr = errors.Join(fmt.Errorf("writer terminal evidence missing"), pollErr)
			}
			return errors.Join(terminalErr, unix.Close(writerPIDFD))
		})
		if returnErr == nil {
			returnErr = metadataNowBefore(session.binding)
		}
		if returnErr == nil && !session.binding.Observation {
			returnErr = metadataWaitLeaseExpiry(ctx, session.binding.LeaseEnd, metadataMonotonicNow, metadataWait)
		}
	}()
	if err := errors.Join(closeChildInput(), closeChildOutput()); err != nil {
		return Receipt{}, nil, err
	}
	if pidErr != nil {
		return Receipt{}, nil, pidErr
	}
	binding := session.binding
	if err := metadataEncode(parentOutput, metadataRequest{Manifest: manifest, Binding: binding, Proof: session.proof, PlanOnly: planOnly}); err != nil {
		return Receipt{}, nil, err
	}
	if err := metadataSend(parentOutput, binding, "HELLO", 1, binding.Request, nil, nil); err != nil {
		return Receipt{}, nil, err
	}
	if _, err := metadataReceive(parentInput, binding, "OBSERVED", 2); err != nil {
		return Receipt{}, nil, err
	}
	if err := session.check(ctx); err != nil {
		return Receipt{}, nil, err
	}
	if err := metadataSend(parentOutput, binding, "PREPARE", 3, binding.Request, nil, nil); err != nil {
		return Receipt{}, nil, err
	}
	var prepared metadataFrame
	if err := metadataDecode(parentInput, &prepared); err != nil {
		return Receipt{}, nil, err
	}
	now, err := metadataMonotonicNow()
	if err != nil {
		return Receipt{}, nil, err
	}
	if err := metadataCheckFrame(prepared, binding, prepared.Kind, 4, now); err != nil {
		return Receipt{}, nil, err
	}
	if prepared.Kind == "NOOP" {
		return Receipt{}, nil, errMetadataOuterDispositionUnknown
	}
	if prepared.Kind == "PLANNED" && planOnly {
		if err := session.check(ctx); err != nil {
			return Receipt{}, nil, err
		}
		if err := metadataSend(parentOutput, binding, "FINAL", 5, binding.Request, nil, nil); err != nil {
			return Receipt{}, nil, err
		}
		return Receipt{}, prepared.Steps, nil
	}
	if planOnly || prepared.Kind != "PREPARED" || prepared.Receipt == nil || !receiptMatchesManifest(*prepared.Receipt, manifest) || prepared.Receipt.Metadata == nil || prepared.Receipt.Metadata.Binding != binding || prepared.Digest != prepared.Receipt.ReceiptSHA256 {
		return Receipt{}, nil, fmt.Errorf("prepared receipt identity refused")
	}
	receipt := *prepared.Receipt
	digest, err := receiptDigest(receipt)
	if err != nil || digest != receipt.ReceiptSHA256 {
		return Receipt{}, nil, fmt.Errorf("prepared receipt digest refused")
	}
	if err := session.check(ctx); err != nil {
		return Receipt{}, nil, err
	}
	if err := metadataSend(parentOutput, binding, "COMMIT_GRANT", 5, digest, nil, nil); err != nil {
		return Receipt{}, nil, err
	}
	published, err := metadataReceive(parentInput, binding, "PUBLISHED", 6)
	if err != nil {
		return Receipt{}, nil, err
	}
	if published.Digest != digest {
		return Receipt{}, nil, fmt.Errorf("publication digest drift")
	}
	if err := session.check(ctx); err != nil {
		return Receipt{}, nil, err
	}
	if err := metadataSend(parentOutput, binding, "RECEIPT_GRANT", 7, digest, nil, nil); err != nil {
		return Receipt{}, nil, err
	}
	commitFrame, err := metadataReceive(parentInput, binding, "COMMITTED", 8)
	if err != nil {
		return Receipt{}, nil, err
	}
	if commitFrame.Digest != digest {
		return Receipt{}, nil, fmt.Errorf("commit digest drift")
	}
	committed = true
	if err := session.check(ctx); err != nil {
		return Receipt{}, nil, err
	}
	if err := metadataSend(parentOutput, binding, "FINAL", 9, digest, nil, nil); err != nil {
		return Receipt{}, nil, err
	}
	return receipt, nil, nil
}

func metadataRequestPurpose(request metadataRequest) error {
	if request.Binding.Observation != request.PlanOnly {
		return fmt.Errorf("observation cannot authorize metadata mutation")
	}
	return nil
}

func metadataReplayDisposition(receipt *Receipt) error {
	if receipt == nil {
		return nil
	}
	return errMetadataOuterDispositionUnknown
}

func metadataWait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func metadataWaitLeaseExpiry(ctx context.Context, deadline int64, now func() (int64, error), wait func(context.Context, time.Duration) error) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := now()
		if err != nil {
			return err
		}
		if current < 0 || deadline <= 0 || deadline-current > 30_000_000_000 {
			return fmt.Errorf("invalid lease expiry wait")
		}
		if current >= deadline {
			return nil
		}
		if err := wait(ctx, time.Duration(deadline-current)); err != nil {
			return err
		}
	}
}

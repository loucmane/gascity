//go:build linux

// Command m5probe preserves the synthetic namespace diagnostic harness.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"syscall"
)

type traceOps struct {
	attach func() error
	wait   func() (bool, error)
	detach func() error
	pause  func(context.Context) error
}

type traceFailure struct {
	stage string
	cause error
}

func (failure *traceFailure) Error() string { return failure.stage + ": " + failure.cause.Error() }
func (failure *traceFailure) Unwrap() error { return failure.cause }

func processDenial(name string, result attempt) bool {
	if result.OK || (result.Errno != 1 && result.Errno != 2 && result.Errno != 3 && result.Errno != 13) {
		return false
	}
	return name != "ptrace" || result.FailureKind == "attach_denied"
}

func finishMetadata(setXattr, makeReadOnly func() error) error {
	if err := setXattr(); err != nil {
		return err
	}
	return makeReadOnly()
}

func traceUntilStopped(ctx context.Context, ops traceOps) (returnErr error) {
	if err := ctx.Err(); err != nil {
		return &traceFailure{stage: "pre_attach", cause: err}
	}
	if err := ops.attach(); err != nil {
		return &traceFailure{stage: "attach_denied", cause: err}
	}
	defer func() {
		if err := ops.detach(); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("ptrace detach: %w", err))
		}
		if returnErr != nil {
			returnErr = &traceFailure{stage: "post_attach", cause: returnErr}
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		stopped, err := ops.wait()
		if err != nil && !errors.Is(err, syscall.EINTR) {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if stopped && err == nil {
			return nil
		}
		if err := ops.pause(ctx); err != nil {
			return err
		}
	}
}

type observationStage interface {
	io.WriteCloser
	Sync() error
}

type publicationOps struct {
	create        func() (observationStage, error)
	rename        func() error
	syncDirectory func() error
}

type publicationState struct {
	Renamed         bool
	DirectorySynced bool
}

func publishObservation(data []byte, ops publicationOps) (publicationState, error) {
	state := publicationState{}
	stage, err := ops.create()
	if err != nil {
		return state, fmt.Errorf("create retained stage: %w", err)
	}
	count, writeErr := stage.Write(data)
	if writeErr == nil && count != len(data) {
		writeErr = io.ErrShortWrite
	}
	var syncErr error
	if writeErr == nil {
		syncErr = stage.Sync()
	}
	closeErr := stage.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return state, fmt.Errorf("retained stage incomplete: %w", err)
	}
	if err := ops.rename(); err != nil {
		return state, fmt.Errorf("publish retained stage: %w", err)
	}
	state.Renamed = true
	if err := ops.syncDirectory(); err != nil {
		return state, fmt.Errorf("renamed observation durability uncertain: %w", err)
	}
	state.DirectorySynced = true
	return state, nil
}

func decodeAttempt(data []byte, commandErr error) attempt {
	if commandErr != nil {
		return attempt{Error: commandErr.Error(), FailureKind: "subprocess"}
	}
	var result attempt
	if err := strictJSON(data, &result); err != nil {
		return attempt{Error: err.Error(), FailureKind: "output"}
	}
	return result
}

package main

import (
	"errors"
	"fmt"
	"runtime"

	"golang.org/x/sys/unix"
)

type listenerOwner interface {
	Fd() uintptr
	Close() error
}

type listenerExecOps struct {
	getFlags func(uintptr) (int, error)
	setFlags func(uintptr, int) error
	exec     func() error
}

func withListenerOwner(owner listenerOwner, run func() error) (returnErr error) {
	if owner.Fd() != 3 {
		return fmt.Errorf("inherited listener must own fd3")
	}
	defer func() {
		returnErr = errors.Join(returnErr, owner.Close())
		runtime.KeepAlive(owner)
	}()
	return run()
}

func execWithListener(owner listenerOwner, ops listenerExecOps) error {
	defer runtime.KeepAlive(owner)
	if owner.Fd() != 3 {
		return fmt.Errorf("exec listener ownership changed")
	}
	flags, err := ops.getFlags(owner.Fd())
	if err != nil {
		return fmt.Errorf("listener inheritance read: %w", err)
	}
	inherited := flags &^ unix.FD_CLOEXEC
	if err := ops.setFlags(owner.Fd(), inherited); err != nil {
		return fmt.Errorf("listener inheritance set: %w", err)
	}
	actual, err := ops.getFlags(owner.Fd())
	if err != nil {
		return fmt.Errorf("listener inheritance readback: %w", err)
	}
	if actual != inherited {
		return fmt.Errorf("listener inheritance flags differ")
	}
	if err := ops.exec(); err != nil {
		return err
	}
	return fmt.Errorf("exec returned without replacing image")
}

func requireExecGenerationAlive(count int, events int16, err error) error {
	if err != nil {
		return fmt.Errorf("exec generation pidfd unavailable: %w", err)
	}
	if count != 0 || events != 0 {
		return fmt.Errorf("supervisor terminal during exec generation wait")
	}
	return nil
}

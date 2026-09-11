package main

import (
	"errors"
	"reflect"
	"testing"

	"golang.org/x/sys/unix"
)

type testListenerOwner struct {
	fd       uintptr
	closed   bool
	events   *[]string
	closeErr error
}

func (owner *testListenerOwner) Fd() uintptr { return owner.fd }
func (owner *testListenerOwner) Close() error {
	owner.closed = true
	*owner.events = append(*owner.events, "close-owner")
	return owner.closeErr
}

func TestListenerOwnerSurvivesImportAndExecError(t *testing.T) {
	events := []string{}
	owner := &testListenerOwner{fd: 3, events: &events}
	execErr := errors.New("exec refused")
	flags := unix.FD_CLOEXEC | 8
	err := withListenerOwner(owner, func() error {
		events = append(events, "import-duplicate", "http-active")
		return execWithListener(owner, listenerExecOps{
			getFlags: func(fd uintptr) (int, error) {
				if fd != 3 || owner.closed {
					t.Fatal("original listener not owned")
				}
				events = append(events, "get-flags")
				return flags, nil
			},
			setFlags: func(fd uintptr, value int) error {
				if fd != 3 || value != 8 || owner.closed {
					t.Fatal("wrong slot or inheritance flags")
				}
				events = append(events, "set-flags")
				flags = value
				return nil
			},
			exec: func() error {
				if owner.closed || flags&unix.FD_CLOEXEC != 0 {
					t.Fatal("listener closed or noninheritable at exec")
				}
				events = append(events, "exec")
				return execErr
			},
		})
	})
	if !errors.Is(err, execErr) || !owner.closed {
		t.Fatal(err, owner.closed)
	}
	want := []string{"import-duplicate", "http-active", "get-flags", "set-flags", "get-flags", "exec", "close-owner"}
	if !reflect.DeepEqual(events, want) {
		t.Fatal(events)
	}
}

func TestListenerHandoffErrorsCloseOnlyOwnedDescriptor(t *testing.T) {
	for _, failure := range []string{"import", "get", "set", "readback", "exec", "close"} {
		events := []string{}
		injected := errors.New(failure)
		owner := &testListenerOwner{fd: 3, events: &events}
		if failure == "close" {
			owner.closeErr = injected
		}
		reads := 0
		err := withListenerOwner(owner, func() error {
			if failure == "import" {
				return injected
			}
			return execWithListener(owner, listenerExecOps{
				getFlags: func(uintptr) (int, error) {
					reads++
					if failure == "get" || (failure == "readback" && reads == 2) {
						return 0, injected
					}
					if reads == 1 {
						return unix.FD_CLOEXEC, nil
					}
					return 0, nil
				},
				setFlags: func(uintptr, int) error {
					if failure == "set" {
						return injected
					}
					return nil
				},
				exec: func() error {
					if failure == "exec" {
						return injected
					}
					return errors.New("exec unexpectedly returned")
				},
			})
		})
		if !owner.closed || len(events) != 1 || !errors.Is(err, injected) {
			t.Fatal(failure, events, err)
		}
	}
	events := []string{}
	unknown := &testListenerOwner{fd: 9, events: &events}
	if err := withListenerOwner(unknown, func() error { t.Fatal("unknown owner used"); return nil }); err == nil || unknown.closed {
		t.Fatal("unknown descriptor touched")
	}
}

func TestExecGenerationTerminalIsRefusal(t *testing.T) {
	if err := requireExecGenerationAlive(0, 0, nil); err != nil {
		t.Fatal(err)
	}
	for _, value := range []struct {
		count  int
		events int16
		err    error
	}{{1, unix.POLLIN, nil}, {1, unix.POLLERR, nil}, {0, 0, errors.New("poll unavailable")}} {
		if requireExecGenerationAlive(value.count, value.events, value.err) == nil {
			t.Fatal("terminal/unavailable pidfd accepted")
		}
	}
}

func TestListenerExecRefusesUnknownOwnerAndFlagDrift(t *testing.T) {
	events := []string{}
	owner := &testListenerOwner{fd: 9, events: &events}
	if execWithListener(owner, listenerExecOps{}) == nil || owner.closed {
		t.Fatal("unknown fd slot used or closed")
	}
	owner.fd = 3
	reads := 0
	err := withListenerOwner(owner, func() error {
		return execWithListener(owner, listenerExecOps{
			getFlags: func(uintptr) (int, error) { reads++; return unix.FD_CLOEXEC, nil },
			setFlags: func(uintptr, int) error { return nil },
			exec:     func() error { t.Fatal("exec despite noninheritable listener"); return nil },
		})
	})
	if err == nil || reads != 2 || !owner.closed {
		t.Fatal(err, reads, owner.closed)
	}
}

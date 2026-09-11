//go:build linux

package main

import (
	"context"
	"errors"
	"io"
	"reflect"
	"syscall"
	"testing"
)

func TestTraceWaitAlwaysDetachesAfterAttach(t *testing.T) {
	for _, scenario := range []string{"stopped", "interrupted", "wait-error", "canceled", "deadline", "detach-error", "attach-error"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			waits, detaches := 0, 0
			fault := errors.New("injected")
			ops := traceOps{
				attach: func() error {
					if scenario == "attach-error" {
						return fault
					}
					return nil
				},
				wait: func() (bool, error) {
					waits++
					switch scenario {
					case "interrupted":
						if waits == 1 {
							return false, syscall.EINTR
						}
					case "wait-error":
						return false, fault
					case "canceled":
						cancel()
						return false, nil
					case "deadline":
						return false, nil
					}
					return true, nil
				},
				detach: func() error {
					detaches++
					if scenario == "detach-error" {
						return fault
					}
					return nil
				},
				pause: func(ctx context.Context) error {
					if scenario == "deadline" {
						return context.DeadlineExceeded
					}
					return ctx.Err()
				},
			}
			err := traceUntilStopped(ctx, ops)
			if scenario == "stopped" || scenario == "interrupted" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("failure accepted")
			}
			want := 1
			if scenario == "attach-error" {
				want = 0
			}
			if detaches != want {
				t.Fatalf("detach attempts %d, want %d", detaches, want)
			}
			if scenario == "deadline" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
			if scenario == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if scenario == "interrupted" && waits != 2 {
				t.Fatal(waits)
			}
		})
	}
}

func TestTraceCancellationBeforeAttachAndJoinedDetachError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := traceUntilStopped(ctx, traceOps{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	waitErr, detachErr := errors.New("wait"), errors.New("detach")
	err := traceUntilStopped(context.Background(), traceOps{attach: func() error { return nil }, wait: func() (bool, error) { return false, waitErr }, detach: func() error { return detachErr }})
	if !errors.Is(err, waitErr) || !errors.Is(err, detachErr) {
		t.Fatalf("lost error: %v", err)
	}
}

type injectedStage struct {
	events *[]string
	fail   string
}

func (stage injectedStage) Write(data []byte) (int, error) {
	*stage.events = append(*stage.events, "write")
	if stage.fail == "write" {
		return 0, errors.New("write")
	}
	if stage.fail == "short" {
		return len(data) - 1, nil
	}
	return len(data), nil
}

func (stage injectedStage) Sync() error {
	*stage.events = append(*stage.events, "sync")
	if stage.fail == "sync" {
		return errors.New("sync")
	}
	return nil
}

func (stage injectedStage) Close() error {
	*stage.events = append(*stage.events, "close")
	if stage.fail == "close" {
		return errors.New("close")
	}
	return nil
}

func TestObservationPublicationFaultsRetainStages(t *testing.T) {
	for _, fault := range []string{"", "create", "write", "short", "sync", "close", "rename", "directory"} {
		t.Run(fault, func(t *testing.T) {
			var events []string
			state, err := publishObservation([]byte("observation"), publicationOps{
				create: func() (observationStage, error) {
					events = append(events, "create")
					if fault == "create" {
						return nil, errors.New("create")
					}
					return injectedStage{&events, fault}, nil
				},
				rename: func() error {
					events = append(events, "rename")
					if fault == "rename" {
						return errors.New("rename")
					}
					return nil
				},
				syncDirectory: func() error {
					events = append(events, "directory")
					if fault == "directory" {
						return errors.New("directory")
					}
					return nil
				},
			})
			if fault == "" {
				if err != nil || !state.Renamed || !state.DirectorySynced {
					t.Fatalf("%+v %v", state, err)
				}
				if !reflect.DeepEqual(events, []string{"create", "write", "sync", "close", "rename", "directory"}) {
					t.Fatal(events)
				}
			} else if err == nil {
				t.Fatal("publication failure accepted")
			}
			if state.Renamed != (fault == "" || fault == "directory") {
				t.Fatalf("wrong rename state %+v", state)
			}
			if fault == "short" && !errors.Is(err, io.ErrShortWrite) {
				t.Fatal(err)
			}
			if fault == "write" || fault == "short" || fault == "sync" {
				if events[len(events)-1] != "close" {
					t.Fatal("stage not closed", events)
				}
			}
		})
	}
}

func TestDescendantRejectsNonzeroExitDespiteValidJSON(t *testing.T) {
	data := []byte(`{"ok":false,"errno":30}`)
	result := decodeAttempt(data, nil)
	if result.Errno != 30 || result.Error != "" {
		t.Fatalf("encoded denial lost: %+v", result)
	}
	result = decodeAttempt(data, errors.New("exit 1"))
	if result.OK || result.Errno != 0 || result.Error == "" {
		t.Fatalf("failed child JSON accepted: %+v", result)
	}
	if decodeAttempt([]byte("invalid"), nil).Error == "" {
		t.Fatal("invalid JSON accepted")
	}
}

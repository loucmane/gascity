package main

import (
	"context"
	"errors"
	"reflect"
	"syscall"
	"testing"
)

func TestMetadataXattrPrecedesReadOnlyMode(t *testing.T) {
	for _, fail := range []string{"", "xattr", "chmod"} {
		var calls []string
		err := finishMetadata(func() error {
			calls = append(calls, "xattr")
			if fail == "xattr" {
				return syscall.EACCES
			}
			return nil
		}, func() error {
			calls = append(calls, "chmod")
			if fail == "chmod" {
				return syscall.EPERM
			}
			return nil
		})
		want := []string{"xattr", "chmod"}
		if fail == "xattr" {
			want = want[:1]
		}
		if !reflect.DeepEqual(calls, want) || (err != nil) != (fail != "") {
			t.Fatalf("%s: %v %v", fail, calls, err)
		}
	}
}

func TestPtraceDenialRequiresFailedAttach(t *testing.T) {
	for _, stage := range []string{"attach", "wait", "detach"} {
		for _, errno := range []syscall.Errno{syscall.EPERM, syscall.ENOENT, syscall.ESRCH, syscall.EACCES} {
			result := observe(func() error {
				return traceUntilStopped(context.Background(), traceOps{
					attach: func() error {
						if stage == "attach" {
							return errno
						}
						return nil
					},
					wait: func() (bool, error) {
						if stage == "wait" {
							return false, errno
						}
						return true, nil
					},
					detach: func() error {
						if stage == "detach" {
							return errno
						}
						return nil
					},
				})
			})
			want := "post_attach"
			if stage == "attach" {
				want = "attach_denied"
			}
			if result.FailureKind != want || result.Errno != int(errno) || processDenial("ptrace", result) != (stage == "attach") {
				t.Fatalf("%s: %+v", stage, result)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := observe(func() error { return traceUntilStopped(ctx, traceOps{}) })
	if processDenial("ptrace", result) || result.FailureKind != "pre_attach" {
		t.Fatalf("%+v", result)
	}
	if processDenial("ptrace", attempt{Errno: 3, FailureKind: "syscall"}) {
		t.Fatal("missing stage accepted")
	}
	if !processDenial("memory", attempt{Errno: 3}) {
		t.Fatal("other denial set changed")
	}
	if processDenial("ptrace", observe(func() error { return errors.New("unknown") })) {
		t.Fatal("unknown error accepted")
	}
}

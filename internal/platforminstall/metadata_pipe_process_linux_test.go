//go:build integration

package platforminstall

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestMetadataInheritedPipeDeadlines owns the real exec/anonymous-pipe boundary.
// No supervisor, namespace, network, manifest or metadata transaction is started.
func TestMetadataInheritedPipeDeadlines(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"exchange", "expired-read", "expired-write", "missing-input", "missing-output", "regular-input", "regular-output"} {
		t.Run(mode, func(t *testing.T) {
			childInput, parentOutput, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = childInput.Close(); _ = parentOutput.Close() })
			parentInput, childOutput, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = parentInput.Close(); _ = childOutput.Close() })
			extra := []*os.File{childInput, childOutput}
			if mode == "regular-input" || mode == "regular-output" {
				file, err := os.OpenFile(filepath.Join(t.TempDir(), "not-a-pipe"), os.O_CREATE|os.O_RDWR, 0o600)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = file.Close() })
				index := 0
				if mode == "regular-output" {
					index = 1
				}
				extra[index] = file
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, executable, "-test.run=^TestMetadataInheritedPipeProcess$", "-test.count=1", "-test.timeout=4s")
			command.Env = []string{"GC_METADATA_PIPE_TEST=" + mode}
			command.ExtraFiles = extra
			command.WaitDelay = time.Second
			var output bytes.Buffer
			command.Stdout, command.Stderr = &output, &output
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			waited := false
			t.Cleanup(func() {
				cancel()
				if !waited {
					_ = command.Wait()
				}
			})
			if err := errors.Join(childInput.Close(), childOutput.Close()); err != nil {
				t.Fatal(err)
			}
			var exchangeErr error
			if mode == "exchange" {
				deadline := time.Now().Add(3 * time.Second)
				if err := errors.Join(parentOutput.SetWriteDeadline(deadline), parentInput.SetReadDeadline(deadline)); err != nil {
					t.Fatal(err)
				}
				_, writeErr := parentOutput.Write([]byte("ping"))
				var reply [4]byte
				_, readErr := io.ReadFull(parentInput, reply[:])
				exchangeErr = errors.Join(writeErr, readErr)
				if exchangeErr == nil && string(reply[:]) != "pong" {
					t.Errorf("unexpected reply %q", reply)
				}
			}
			err = command.Wait()
			waited = true
			if err != nil || exchangeErr != nil {
				t.Fatalf("child %s: wait=%v exchange=%v\n%s", mode, err, exchangeErr, output.String())
			}
		})
	}
}

func TestMetadataInheritedPipeProcess(t *testing.T) {
	mode := os.Getenv("GC_METADATA_PIPE_TEST")
	if mode == "" {
		return
	}
	for _, fd := range []int{3, 4} {
		flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
		if err != nil || flags&unix.O_NONBLOCK != 0 {
			t.Fatalf("exec did not deliver blocking fd%d: flags=%x err=%v", fd, flags, err)
		}
	}
	if mode == "missing-input" || mode == "missing-output" {
		fd := 3
		if mode == "missing-output" {
			fd = 4
		}
		if err := unix.Close(fd); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	if mode == "expired-read" || mode == "expired-write" {
		deadline = time.Unix(1, 0)
	}
	input, output, err := metadataWriterPipes(deadline)
	if mode == "missing-input" || mode == "missing-output" || mode == "regular-input" || mode == "regular-output" {
		if err == nil {
			_ = input.Close()
			_ = output.Close()
			t.Fatal("invalid inherited channel accepted")
		}
		for _, fd := range []int{3, 4} {
			if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0); !errors.Is(err, unix.EBADF) {
				t.Errorf("failed initialization retained fd%d: %v", fd, err)
			}
		}
		return
	}
	if err != nil {
		t.Fatalf("initialize inherited pipes: %v", err)
	}
	defer func() {
		if err := errors.Join(input.Close(), output.Close()); err != nil {
			t.Error(err)
		}
	}()
	switch mode {
	case "exchange":
		var request [4]byte
		if _, err := io.ReadFull(input, request[:]); err != nil || string(request[:]) != "ping" {
			t.Fatalf("request=%q err=%v", request, err)
		}
		if _, err := output.Write([]byte("pong")); err != nil {
			t.Fatal(err)
		}
	case "expired-read":
		_, err := input.Read(make([]byte, 1))
		if !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Fatalf("expired read: got %v, want deadline exceeded", err)
		}
	case "expired-write":
		_, err := output.Write([]byte("must not be written"))
		if !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Fatalf("expired write: got %v, want deadline exceeded", err)
		}
	default:
		t.Fatalf("unknown child mode %q", mode)
	}
}

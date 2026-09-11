package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

type attempt struct {
	Target      *attackTarget `json:"target,omitempty"`
	OK          bool          `json:"ok"`
	Errno       int           `json:"errno"`
	Error       string        `json:"error,omitempty"`
	FailureKind string        `json:"failure_kind,omitempty"`
}
type fixtureIdentity struct {
	Boot          string            `json:"boot"`
	Start         string            `json:"start"`
	ExecutableSHA string            `json:"executable_sha"`
	Marker        string            `json:"marker"`
	Memory        string            `json:"memory"`
	PID           int               `json:"pid"`
	FD            int               `json:"fd"`
	Address       uint64            `json:"address"`
	Namespaces    map[string]string `json:"namespaces"`
	PtracerPolicy string            `json:"ptracer_policy"`
}
type fileImage struct {
	Stat   unix.Stat_t       `json:"stat"`
	SHA256 string            `json:"sha256"`
	Xattrs map[string]string `json:"xattrs"`
}

func namespaces() (map[string]string, error) {
	result := map[string]string{}
	for _, name := range []string{"user", "pid", "mnt", "net", "ipc", "time"} {
		value, err := os.Readlink("/proc/self/ns/" + name)
		if err != nil {
			return nil, err
		}
		result[name] = value
	}
	return result, nil
}

func observe(operation func() error) attempt {
	err := operation()
	if err == nil {
		return attempt{OK: true}
	}
	var errno syscall.Errno
	var traceErr *traceFailure
	if errors.As(err, &traceErr) {
		errors.As(err, &errno)
		return attempt{Errno: int(errno), Error: err.Error(), FailureKind: traceErr.stage}
	}
	if !errors.As(err, &errno) {
		return attempt{Error: err.Error(), FailureKind: "non_errno"}
	}
	return attempt{Errno: int(errno), Error: err.Error(), FailureKind: "syscall"}
}

func boundedCommand(ctx context.Context, argv []string, input []byte) ([]byte, error) {
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Env = childEnvironment()
	command.Stdin = bytes.NewReader(input)
	var stdout, stderr limitedBuffer
	command.Stdout, command.Stderr = &stdout, &stderr
	command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
	command.WaitDelay = 10 * time.Second
	err := command.Run()
	if err != nil {
		return stdout.Bytes(), fmt.Errorf("%s: %w: %s", argv[0], err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func createFixture(root string) error {
	if err := os.Mkdir(root, 0o700); err != nil {
		return err
	}
	for _, name := range []string{"overwrite", "truncate", "unlink", "rename", "chmod", "utime", "chown", "xattr", ".packman-cache.lock"} {
		if err := os.WriteFile(root+"/"+name, []byte("synthetic-cache\n"), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func mutations(ctx context.Context, root, helper string) map[string]attempt {
	result := map[string]attempt{}
	operations := map[string]func() error{
		"create":    func() error { return os.WriteFile(root+"/created", []byte("new"), 0o600) },
		"overwrite": func() error { return os.WriteFile(root+"/overwrite", []byte("changed"), 0o600) },
		"truncate":  func() error { return os.Truncate(root+"/truncate", 0) },
		"unlink":    func() error { return os.Remove(root + "/unlink") },
		"rename":    func() error { return os.Rename(root+"/rename", root+"/renamed") },
		"mkdir":     func() error { return os.Mkdir(root+"/directory", 0o700) },
		"symlink":   func() error { return os.Symlink("overwrite", root+"/link") },
		"chmod":     func() error { return os.Chmod(root+"/chmod", 0o400) },
		"utime":     func() error { return os.Chtimes(root+"/utime", time.Unix(1, 0), time.Unix(1, 0)) },
		"chown":     func() error { return os.Chown(root+"/chown", os.Getuid(), os.Getgid()) },
		"xattr":     func() error { return unix.Setxattr(root+"/xattr", "user.m5", []byte("synthetic"), 0) },
	}
	for name, operation := range operations {
		result[name] = observe(operation)
	}
	completion := make(chan attempt, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		completion <- observe(func() error { return os.WriteFile(root+"/thread-write", []byte("thread"), 0o600) })
	}()
	select {
	case result["descendant_thread"] = <-completion:
	case <-ctx.Done():
		result["descendant_thread"] = attempt{Error: ctx.Err().Error()}
	}
	data, err := boundedCommand(ctx, []string{helper, "descendant", root}, nil)
	result["descendant_process"] = decodeAttempt(data, err)
	return result
}

func snapshot(root string) (map[string]fileImage, error) {
	result := map[string]fileImage{}
	var walk func(string) error
	walk = func(path string) error {
		var image fileImage
		if err := unix.Lstat(path, &image.Stat); err != nil {
			return err
		}
		image.Xattrs = map[string]string{}
		names := make([]byte, 8192)
		count, err := unix.Listxattr(path, names)
		if err != nil {
			return err
		}
		for _, name := range strings.Split(string(names[:count]), "\x00") {
			if name != "" {
				value := make([]byte, 8192)
				size, err := unix.Getxattr(path, name, value)
				if err != nil {
					return err
				}
				image.Xattrs[name] = hex.EncodeToString(value[:size])
			}
		}
		fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NOATIME, 0)
		if err != nil {
			return err
		}
		file := os.NewFile(uintptr(fd), path)
		defer func() { _ = file.Close() }()
		switch image.Stat.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
			names, err := file.Readdirnames(-1)
			if err != nil {
				return err
			}
			for _, name := range names {
				if err := walk(filepath.Join(path, name)); err != nil {
					return err
				}
			}
		case unix.S_IFREG:
			data, err := io.ReadAll(io.LimitReader(file, 65537))
			if err != nil || len(data) > 65536 {
				return fmt.Errorf("invalid inventory file")
			}
			image.SHA256 = digest(data)
		default:
			return fmt.Errorf("unexpected fixture object: %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[relative] = image
		return nil
	}
	err := walk(root)
	return result, err
}

func serveFixture() (returnErr error) {
	self, err := os.Executable()
	if err != nil || self != binaryPath {
		return fmt.Errorf("invalid fixture executable")
	}
	marker, err := os.OpenFile(runRoot+"/fixture-marker", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = marker.Close() }()
	if _, err := marker.WriteString(fixtureMarker); err != nil {
		return err
	}
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 1, 0, 0, 0); err != nil {
		return err
	}
	if err := unix.Prctl(unix.PR_SET_PTRACER, uintptr(unix.PR_SET_PTRACER_ANY), 0, 0, 0); err != nil {
		return err
	}
	data, err := unix.Mmap(-1, 0, 4096, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANON)
	if err != nil {
		return err
	}
	defer func() { returnErr = errors.Join(returnErr, unix.Munmap(data)) }()
	copy(data, []byte(fixtureMemory))
	if err := unix.Mprotect(data, unix.PROT_READ); err != nil {
		return err
	}
	spaces, err := namespaces()
	if err != nil {
		return err
	}
	boot, start, hash, err := fixtureProcessIdentity(os.Getpid())
	if err != nil {
		return err
	}
	identity := fixtureIdentity{PID: os.Getpid(), FD: int(marker.Fd()), Address: uint64(uintptr(unsafe.Pointer(&data[0]))), Namespaces: spaces, PtracerPolicy: "fixture-only-any-same-uid", Boot: boot, Start: start, ExecutableSHA: hash, Marker: fixtureMarker, Memory: fixtureMemory}
	if err := json.NewEncoder(os.Stdout).Encode(identity); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, io.LimitReader(os.Stdin, 1)); close(done) }()
	select {
	case <-done:
	case <-time.After(360 * time.Second):
		return fmt.Errorf("fixture deadline")
	}
	return nil
}

func processAttempts(ctx context.Context, identity fixtureIdentity) map[string]attempt {
	if !validFixture(identity) {
		return map[string]attempt{}
	}
	base := "/proc/" + strconv.Itoa(identity.PID)
	result := map[string]attempt{
		"signal": observe(func() error { return unix.Kill(identity.PID, 0) }),
		"memory": observe(func() error {
			file, err := os.Open(base + "/mem")
			if err != nil {
				return err
			}
			defer func() { _ = file.Close() }()
			data := make([]byte, len(identity.Memory))
			_, err = file.ReadAt(data, int64(identity.Address))
			if err != nil {
				return err
			}
			if string(data) != identity.Memory {
				return fmt.Errorf("wrong fixture memory")
			}
			return nil
		}),
		"fd": observe(func() error {
			data, err := os.ReadFile(base + "/fd/" + strconv.Itoa(identity.FD))
			if err != nil {
				return err
			}
			if string(data) != identity.Marker {
				return fmt.Errorf("wrong fixture descriptor")
			}
			return nil
		}),
		"ptrace": observe(func() error {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			return traceUntilStopped(ctx, traceOps{
				attach: func() error { return unix.PtraceAttach(identity.PID) },
				wait: func() (bool, error) {
					var status unix.WaitStatus
					pid, err := unix.Wait4(identity.PID, &status, unix.WNOHANG, nil)
					if err != nil || pid == 0 {
						return false, err
					}
					if pid != identity.PID || !status.Stopped() {
						return false, fmt.Errorf("fixture did not enter ptrace stop")
					}
					return true, nil
				},
				detach: func() error { return unix.PtraceDetach(identity.PID) },
				pause: func(ctx context.Context) error {
					timer := time.NewTimer(10 * time.Millisecond)
					defer timer.Stop()
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-timer.C:
						return nil
					}
				},
			})
		}),
	}
	for name, value := range result {
		target := targetForFixture(identity)
		value.Target = &target
		result[name] = value
	}
	return result
}

func fdInventory() (map[string]string, error) {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, entry := range entries {
		value, err := os.Readlink("/proc/self/fd/" + entry.Name())
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result[entry.Name()] = value
		fd, err := strconv.Atoi(entry.Name())
		if err != nil {
			return nil, err
		}
		if fd > 2 && value != "anon_inode:[eventpoll]" && value != "anon_inode:[eventfd]" {
			return nil, fmt.Errorf("unexpected descriptor %d: %s", fd, value)
		}
	}
	return result, nil
}

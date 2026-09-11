package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

type fixtureIdentity struct {
	PID           int               `json:"pid"`
	FD            int               `json:"fd"`
	Address       uint64            `json:"address"`
	Namespaces    map[string]string `json:"namespaces"`
	PtracerPolicy string            `json:"ptracer_policy"`
}

type childReport struct {
	Namespaces map[string]string  `json:"namespaces"`
	FDs        map[string]string  `json:"fds"`
	Mutations  map[string]attempt `json:"mutations"`
	Attacks    map[string]attempt `json:"attacks"`
	Metadata   bool               `json:"metadata"`
	ReadLock   bool               `json:"read_lock"`
}

type fileImage struct {
	Stat   unix.Stat_t       `json:"stat"`
	SHA256 string            `json:"sha256"`
	Xattrs map[string]string `json:"xattrs"`
}

type runReport struct {
	EvidenceKind          string               `json:"evidence_kind"`
	Status                string               `json:"status"`
	Error                 string               `json:"error,omitempty"`
	PlanSHA256            string               `json:"plan_sha256"`
	Fixture               fixtureIdentity      `json:"fixture"`
	Control               map[string]attempt   `json:"control"`
	ProcessControl        map[string]attempt   `json:"process_control"`
	Child                 childReport          `json:"child"`
	Before                map[string]fileImage `json:"before"`
	After                 map[string]fileImage `json:"after"`
	ZeroProcessResidue    bool                 `json:"zero_process_residue"`
	FixtureReaped         bool                 `json:"fixture_reaped"`
	FixturePIDFDTerminal  bool                 `json:"fixture_pidfd_terminal"`
	ConfinedPID1Completed bool                 `json:"confined_pid1_completed"`
}

type limitedBuffer struct{ bytes.Buffer }

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	if buffer.Len()+len(data) > 65536 {
		return 0, fmt.Errorf("bounded output exceeded")
	}
	return buffer.Buffer.Write(data)
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func fileDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return digest(data), nil
}

func main() {
	if err := dispatch(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func dispatch(args []string) error {
	if os.Getuid() == 0 || os.Geteuid() == 0 {
		return fmt.Errorf("root refused")
	}
	if runtime.GOARCH != "amd64" {
		return fmt.Errorf("reviewed Linux amd64 only")
	}
	if len(args) == 4 && args[0] == "validate-result" {
		exitCode, err := strconv.Atoi(args[3])
		if err != nil {
			return err
		}
		file, err := os.Open(args[1])
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, 65537))
		closeErr := file.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			return err
		}
		return validateObservation(data, args[2], exitCode)
	}
	if len(args) == 1 && args[0] == "freeze" {
		value := plan{Schema: 1, RunRoot: runRoot, Binary: binaryPath, TimeoutSeconds: 120, Argv: probeArgv()}
		for _, path := range dependencyPaths {
			hash, err := fileDigest(path)
			if err != nil {
				return err
			}
			value.Pins = append(value.Pins, pin{Path: path, SHA256: hash})
		}
		if err := validateRuntime(value); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(value)
	}
	if len(args) == 3 && args[0] == "execute-reviewed" {
		data, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		if digest(data) != args[2] {
			return fmt.Errorf("reviewed plan digest mismatch")
		}
		value, err := decodePlan(data)
		if err != nil {
			return err
		}
		if err := validateRuntime(value); err != nil {
			return err
		}
		return executeProbe(value, args[2])
	}
	if len(args) == 1 && args[0] == "fixture" {
		return serveFixture()
	}
	if len(args) == 1 && args[0] == "child" {
		return childProbe()
	}
	if len(args) == 2 && args[0] == "descendant" {
		if args[1] != "/cache" && args[1] != runRoot+"/control" {
			return fmt.Errorf("non-fixture descendant path")
		}
		return json.NewEncoder(os.Stdout).Encode(observe(func() error { return os.WriteFile(args[1]+"/process-write", []byte("descendant"), 0o600) }))
	}
	return fmt.Errorf("expected freeze or execute-reviewed PLAN SHA256; internal modes are fixture-only")
}

func validateObservation(data []byte, planHash string, exitCode int) error {
	if exitCode != 0 {
		return fmt.Errorf("outer execution failed; observation is not acceptance")
	}
	decoded, err := hex.DecodeString(planHash)
	if err != nil || len(decoded) != 32 {
		return fmt.Errorf("invalid reviewed plan hash")
	}
	var report runReport
	if err := strictJSON(data, &report); err != nil {
		return err
	}
	if report.EvidenceKind != "observation-only-requires-outer-exit-zero" || report.PlanSHA256 != planHash || report.Status != "PASS" || report.Error != "" {
		return fmt.Errorf("observation identity/outcome refused")
	}
	if !report.ZeroProcessResidue || !report.FixtureReaped || !report.FixturePIDFDTerminal || !report.ConfinedPID1Completed {
		return fmt.Errorf("process terminal witnesses incomplete")
	}
	if len(report.Before) == 0 || !reflect.DeepEqual(report.Before, report.After) || !report.Child.Metadata || !report.Child.ReadLock || verdict(report.Control, report.Child.Mutations) != "PASS" {
		return fmt.Errorf("capability observations incomplete")
	}
	if report.Fixture.PID <= 1 || report.Fixture.PtracerPolicy != "fixture-only-any-same-uid" {
		return fmt.Errorf("fixture identity incomplete")
	}
	for _, name := range []string{"user", "pid", "mnt", "net", "ipc"} {
		if report.Fixture.Namespaces[name] == "" || report.Child.Namespaces[name] == "" || report.Fixture.Namespaces[name] == report.Child.Namespaces[name] {
			return fmt.Errorf("namespace witness missing: %s", name)
		}
	}
	if report.Fixture.Namespaces["time"] == "" || report.Fixture.Namespaces["time"] != report.Child.Namespaces["time"] {
		return fmt.Errorf("monotonic domain witness missing")
	}
	if len(report.ProcessControl) != 4 || len(report.Child.Attacks) != 4 {
		return fmt.Errorf("process matrix incomplete")
	}
	for _, name := range []string{"signal", "memory", "fd", "ptrace"} {
		positive, exists := report.ProcessControl[name]
		negative, negativeExists := report.Child.Attacks[name]
		if !exists || !positive.OK || !negativeExists || !processDenial(name, negative) {
			return fmt.Errorf("process witness refused: %s", name)
		}
	}
	for _, name := range []string{"0", "1", "2"} {
		if report.Child.FDs[name] == "" {
			return fmt.Errorf("standard descriptor witness missing")
		}
	}
	for name, value := range report.Child.FDs {
		fd, err := strconv.Atoi(name)
		if err != nil || fd < 0 {
			return fmt.Errorf("invalid FD observation")
		}
		if fd > 2 && value != "anon_inode:[eventpoll]" && value != "anon_inode:[eventfd]" {
			return fmt.Errorf("unexpected FD observation")
		}
	}
	return nil
}

func validateRuntime(value plan) error {
	if err := validatePlan(value, os.Geteuid()); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil || self != binaryPath {
		return fmt.Errorf("unexpected executable location")
	}
	if _, err := os.Lstat(runRoot); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("run root must be absent: %w", err)
	}
	if _, err := os.Lstat("/etc/ld.so.preload"); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("ambient loader preload configuration refused")
	}
	resolved, err := filepath.EvalSymlinks("/tmp")
	if err != nil || resolved != "/tmp" {
		return fmt.Errorf("aliased /tmp")
	}
	for _, item := range value.Pins {
		info, err := os.Stat(item.Path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
			return fmt.Errorf("unsafe dependency mode: %s", item.Path)
		}
		hash, err := fileDigest(item.Path)
		if err != nil || hash != item.SHA256 {
			return fmt.Errorf("dependency drift: %s", item.Path)
		}
		image, err := elf.Open(item.Path)
		if err != nil {
			return err
		}
		libs, libErr := image.ImportedLibraries()
		rpaths, rpathErr := image.DynString(elf.DT_RPATH)
		runpaths, runpathErr := image.DynString(elf.DT_RUNPATH)
		_ = image.Close()
		if rpathErr != nil || runpathErr != nil || len(rpaths) != 0 || len(runpaths) != 0 {
			return fmt.Errorf("unexpected loader search path: %s", item.Path)
		}
		if libErr != nil {
			return libErr
		}
		if item.Path == binaryPath && len(libs) != 0 {
			return fmt.Errorf("helper must be static")
		}
		for _, lib := range libs {
			if lib != "libcap.so.2" && lib != "libc.so.6" && lib != "ld-linux-x86-64.so.2" && lib != "libselinux.so.1" && lib != "libpcre2-8.so.0" {
				return fmt.Errorf("unreviewed ELF dependency: %s", lib)
			}
		}
	}
	return nil
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
	command.Env = []string{}
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
	if _, err := marker.WriteString("m5-public-synthetic-marker"); err != nil {
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
	copy(data, []byte("m5-public-synthetic-memory"))
	if err := unix.Mprotect(data, unix.PROT_READ); err != nil {
		return err
	}
	spaces, err := namespaces()
	if err != nil {
		return err
	}
	identity := fixtureIdentity{PID: os.Getpid(), FD: int(marker.Fd()), Address: uint64(uintptr(unsafe.Pointer(&data[0]))), Namespaces: spaces, PtracerPolicy: "fixture-only-any-same-uid"}
	if err := json.NewEncoder(os.Stdout).Encode(identity); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, io.LimitReader(os.Stdin, 1)); close(done) }()
	select {
	case <-done:
	case <-time.After(120 * time.Second):
		return fmt.Errorf("fixture deadline")
	}
	return nil
}

func processAttempts(ctx context.Context, identity fixtureIdentity) map[string]attempt {
	base := "/proc/" + strconv.Itoa(identity.PID)
	return map[string]attempt{
		"signal": observe(func() error { return unix.Kill(identity.PID, 0) }),
		"memory": observe(func() error {
			file, err := os.Open(base + "/mem")
			if err != nil {
				return err
			}
			defer func() { _ = file.Close() }()
			data := make([]byte, len("m5-public-synthetic-memory"))
			_, err = file.ReadAt(data, int64(identity.Address))
			if err != nil {
				return err
			}
			if string(data) != "m5-public-synthetic-memory" {
				return fmt.Errorf("wrong fixture memory")
			}
			return nil
		}),
		"fd": observe(func() error {
			data, err := os.ReadFile(base + "/fd/" + strconv.Itoa(identity.FD))
			if err != nil {
				return err
			}
			if string(data) != "m5-public-synthetic-marker" {
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

func childProbe() error {
	self, err := os.Executable()
	if err != nil || self != "/probe" {
		return fmt.Errorf("child outside private mount")
	}
	if os.Getpid() != 1 {
		return fmt.Errorf("probe must own private PID namespace init")
	}
	var identity fixtureIdentity
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 65537))
	if err != nil {
		return err
	}
	if err := strictJSON(data, &identity); err != nil {
		return err
	}
	report := childReport{}
	report.Namespaces, err = namespaces()
	if err != nil {
		return err
	}
	for _, name := range []string{"user", "pid", "mnt", "net", "ipc"} {
		if identity.Namespaces[name] == "" || report.Namespaces[name] == identity.Namespaces[name] {
			return fmt.Errorf("namespace not private: %s", name)
		}
	}
	if report.Namespaces["time"] != identity.Namespaces["time"] {
		return fmt.Errorf("different monotonic domain")
	}
	if identity.PID <= 1 {
		return fmt.Errorf("invalid synthetic host identity")
	}
	if _, err := os.Stat("/proc/" + strconv.Itoa(identity.PID)); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("host PID visible or collides in child; inconclusive")
	}
	report.FDs, err = fdInventory()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report.Attacks = processAttempts(ctx, identity)
	lock, err := os.Open("/cache/.packman-cache.lock")
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_SH|unix.LOCK_NB); err != nil {
		return err
	}
	report.ReadLock = true
	report.Mutations = mutations(ctx, "/cache", "/probe")
	metadata := "/metadata/staged"
	if err := os.WriteFile(metadata, []byte("synthetic-metadata"), 0o600); err != nil {
		return err
	}
	file, err := os.OpenFile(metadata, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	err = file.Sync()
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(metadata, "/metadata/receipt"); err != nil {
		return err
	}
	if err := finishMetadata(
		func() error { return unix.Setxattr("/metadata/receipt", "user.m5", []byte("metadata"), 0) },
		func() error { return os.Chmod("/metadata/receipt", 0o400) },
	); err != nil {
		return err
	}
	directory, err := os.Open("/metadata")
	if err != nil {
		return err
	}
	err = directory.Sync()
	closeErr = directory.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	report.Metadata = true
	return json.NewEncoder(os.Stdout).Encode(report)
}

func executeProbe(value plan, planHash string) (returnErr error) {
	if err := os.Mkdir(runRoot, 0o700); err != nil {
		return err
	}
	report := runReport{Status: "INCONCLUSIVE", PlanSHA256: planHash, EvidenceKind: "observation-only-requires-outer-exit-zero"}
	defer func() {
		if returnErr != nil {
			report.Error = returnErr.Error()
			if report.Status == "PASS" {
				report.Status = "INCONCLUSIVE"
			}
		}
		data, err := json.MarshalIndent(report, "", "  ")
		if err == nil {
			state, publishErr := publishObservation(append(data, '\n'), publicationOps{
				create: func() (observationStage, error) {
					return os.OpenFile(runRoot+"/result.stage", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
				},
				rename: func() error {
					return unix.Renameat2(unix.AT_FDCWD, runRoot+"/result.stage", unix.AT_FDCWD, runRoot+"/result.json", unix.RENAME_NOREPLACE)
				},
				syncDirectory: func() error {
					directory, err := os.Open(runRoot)
					if err != nil {
						return err
					}
					return errors.Join(directory.Sync(), directory.Close())
				},
			})
			if publishErr != nil {
				err = fmt.Errorf("observation publication renamed=%t directory_synced=%t: %w", state.Renamed, state.DirectorySynced, publishErr)
			}
		}
		if err != nil {
			returnErr = errors.Join(returnErr, err)
		}
	}()
	for _, name := range []string{"cache", "control"} {
		if err := createFixture(runRoot + "/" + name); err != nil {
			return err
		}
	}
	if err := os.Mkdir(runRoot+"/metadata", 0o700); err != nil {
		return err
	}
	var err error
	report.Before, err = snapshot(runRoot + "/cache")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(value.TimeoutSeconds)*time.Second)
	defer cancel()
	fixture := exec.CommandContext(ctx, binaryPath, "fixture")
	fixture.Env = []string{}
	input, err := fixture.StdinPipe()
	if err != nil {
		return err
	}
	output, err := fixture.StdoutPipe()
	if err != nil {
		return err
	}
	var fixtureErrors limitedBuffer
	fixture.Stderr = &fixtureErrors
	fixture.Cancel = func() error { return fixture.Process.Signal(syscall.SIGTERM) }
	fixture.WaitDelay = 10 * time.Second
	if err := fixture.Start(); err != nil {
		return err
	}
	pidfd, pidErr := unix.PidfdOpen(fixture.Process.Pid, 0)
	defer func() {
		if err := input.Close(); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("fixture stdin close: %w", err))
		}
		waitErr := fixture.Wait()
		if waitErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("fixture teardown: %w: %s", waitErr, fixtureErrors.String()))
		}
		if fixture.ProcessState == nil || !fixture.ProcessState.Exited() {
			returnErr = errors.Join(returnErr, fmt.Errorf("fixture not terminal"))
		}
		report.FixtureReaped = waitErr == nil && fixture.ProcessState != nil && fixture.ProcessState.Exited()
		if pidErr == nil {
			polls := []unix.PollFd{{Fd: int32(pidfd), Events: unix.POLLIN}}
			count, err := unix.Poll(polls, 0)
			if err != nil || count != 1 || polls[0].Revents&unix.POLLIN == 0 {
				returnErr = errors.Join(returnErr, fmt.Errorf("fixture pidfd not terminal"))
			}
			report.FixturePIDFDTerminal = err == nil && count == 1 && polls[0].Revents&unix.POLLIN != 0
			if err := unix.Close(pidfd); err != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("pidfd close: %w", err))
			}
		}
		report.ZeroProcessResidue = returnErr == nil && report.FixtureReaped && report.FixturePIDFDTerminal && report.ConfinedPID1Completed
	}()
	if pidErr != nil {
		return pidErr
	}
	if err := json.NewDecoder(io.LimitReader(output, 65536)).Decode(&report.Fixture); err != nil {
		return err
	}
	if report.Fixture.PID != fixture.Process.Pid {
		return fmt.Errorf("fixture identity mismatch")
	}
	report.ProcessControl = processAttempts(ctx, report.Fixture)
	for name, result := range report.ProcessControl {
		if !result.OK {
			return fmt.Errorf("INCONCLUSIVE: process control %s: %s", name, result.Error)
		}
	}
	report.Control = mutations(ctx, runRoot+"/control", binaryPath)
	for name, result := range report.Control {
		if !result.OK {
			return fmt.Errorf("INCONCLUSIVE: writable control %s: %s", name, result.Error)
		}
	}
	request, err := json.Marshal(report.Fixture)
	if err != nil {
		return err
	}
	childOutput, childErr := boundedCommand(ctx, value.Argv, request)
	if err := os.WriteFile(runRoot+"/child-output.json", childOutput, 0o600); err != nil {
		return err
	}
	report.After, err = snapshot(runRoot + "/cache")
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(report.Before, report.After) {
		report.Status = "FAIL"
		return fmt.Errorf("protected cache inventory changed")
	}
	if childErr != nil {
		return childErr
	}
	if err := strictJSON(childOutput, &report.Child); err != nil {
		return err
	}
	report.ConfinedPID1Completed = report.Child.Namespaces["pid"] != ""
	for name, positive := range report.ProcessControl {
		negative, exists := report.Child.Attacks[name]
		if !positive.OK || !exists || !processDenial(name, negative) {
			return fmt.Errorf("process boundary not demonstrated: %s", name)
		}
	}
	if !report.Child.ReadLock || !report.Child.Metadata {
		return fmt.Errorf("required positive absent")
	}
	report.Status = verdict(report.Control, report.Child.Mutations)
	if report.Status != "PASS" {
		return fmt.Errorf("cache mutation verdict %s", report.Status)
	}
	return nil
}

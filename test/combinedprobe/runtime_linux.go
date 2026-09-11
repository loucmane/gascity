package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"debug/elf"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const (
	runRoot      = "/tmp/ga-tmgr-combined-run-r6-20260910"
	binaryPath   = "/tmp/ga-tmgr-combined-candidate-r6-20260910/combinedprobe"
	impostorPath = "/tmp/ga-tmgr-combined-candidate-r6-20260910/impostor"
)

var imageVariant = "genuine"

const (
	version  = "ga-tmgr-synthetic-supervisor-v1"
	bwrapSHA = "52231e1caf55bcbc667b269f49c63599a6f7db4767ae6a039580d0ff853db712"
)

var (
	cases            = []string{"success", "version-mismatch", "unsupported", "before-loss", "after-loss", "exit-before", "exit-after", "exec-before", "exec-after", "restart-before", "restart-after", "same-build-impostor", "wrong-image", "expiry-before", "expiry-after", "crash-before", "crash-after", "eof-before", "eof-after", "fsync-before", "fsync-after", "concurrent"}
	expectedVersions = acceptanceVersions()
	dependencies     = []string{"/usr/bin/bwrap", binaryPath, "/usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2", "/usr/lib/x86_64-linux-gnu/libcap.so.2", "/usr/lib/x86_64-linux-gnu/libc.so.6", "/usr/lib/x86_64-linux-gnu/libselinux.so.1", "/usr/lib/x86_64-linux-gnu/libpcre2-8.so.0", impostorPath}
	hostSpaces       = map[string]string{"user": "user:[4026531837]", "pid": "pid:[4026532220]", "mnt": "mnt:[4026532218]", "net": "net:[4026531840]", "ipc": "ipc:[4026532207]", "time": "time:[4026531834]"}
)

type pin struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type plan struct {
	Cases            []string            `json:"cases"`
	ChildEnvironment []string            `json:"child_environment"`
	ExpectedVersions map[string]string   `json:"expected_versions"`
	Schema           string              `json:"schema"`
	Root             string              `json:"root"`
	Binary           string              `json:"binary"`
	Pins             []pin               `json:"pins"`
	Argv             map[string][]string `json:"argv"`
	Timeout          int                 `json:"timeout"`
	Host             map[string]string   `json:"host"`
}
type identity struct {
	PID      int    `json:"pid"`
	Boot     string `json:"boot"`
	Start    string `json:"start"`
	Exe      string `json:"exe"`
	Hash     string `json:"hash"`
	Instance string `json:"instance"`
	Port     int    `json:"port"`
	Inode    string `json:"inode"`
	Version  string `json:"version"`
}
type request struct {
	ExpectedVersion string   `json:"expected_version"`
	Case            string   `json:"case"`
	Identity        identity `json:"identity"`
	Binding         binding  `json:"binding"`
	LeaseDeadline   int64    `json:"lease_deadline"`
	PreManifest     string   `json:"pre_manifest"`
	PreReceipt      string   `json:"pre_receipt"`
}
type outcome struct {
	ACLControls             map[string]attempt `json:"acl_controls"`
	Acceptance              acceptanceEvidence `json:"acceptance"`
	WriterState             string             `json:"writer_state"`
	WriterPID               int                `json:"writer_pid"`
	WriterWaitPID           int                `json:"writer_wait_pid"`
	WriterWaitObserved      bool               `json:"writer_wait_observed"`
	WriterPIDFDTerminal     bool               `json:"writer_pidfd_terminal"`
	WriterExit              int                `json:"writer_exit"`
	Version                 versionEvidence    `json:"version"`
	APIRefusal              *apiStatusError    `json:"api_refusal,omitempty"`
	PostControls            map[string]attempt `json:"post_controls,omitempty"`
	PostControlsStartedAt   int64              `json:"post_controls_started_at"`
	PostControlsCompletedAt int64              `json:"post_controls_completed_at"`
	LastStep                string             `json:"last_step"`
	VerifierExit            int                `json:"verifier_exit"`
	Status                  string             `json:"status"`
	Error                   string             `json:"error,omitempty"`
	Committed               bool               `json:"committed"`
	Pair                    bool               `json:"pair"`
	CacheUnchanged          bool               `json:"cache_unchanged"`
	WriterReaped            bool               `json:"writer_reaped"`
	SupervisorReaped        bool               `json:"supervisor_reaped"`
	Boundary                boundaryReport     `json:"boundary"`
}
type observation struct {
	Fixture         fixtureIdentity    `json:"fixture"`
	ProcessControls map[string]attempt `json:"process_controls"`
	FixtureReaped   bool               `json:"fixture_reaped"`
	Kind            string             `json:"kind"`
	Plan            string             `json:"plan"`
	Cases           map[string]outcome `json:"cases"`
	Status          string             `json:"status"`
}

func mono() int64 {
	var value unix.Timespec
	if unix.ClockGettime(unix.CLOCK_MONOTONIC, &value) != nil {
		return -1
	}
	return value.Nano()
}

func randomID() (string, error) {
	data := make([]byte, 32)
	_, err := rand.Read(data)
	return hex.EncodeToString(data), err
}
func fileHash(path string) (string, error) { data, err := os.ReadFile(path); return digest(data), err }
func jsonBytes(value any) []byte           { data, _ := json.Marshal(value); return data }

func writerArgv(name string) []string {
	base := runRoot + "/" + name
	return []string{dependencies[2], "--inhibit-cache", "--glibc-hwcaps-mask", "", "--library-path", "/usr/lib/x86_64-linux-gnu", "/usr/bin/bwrap", "--unshare-user", "--unshare-pid", "--as-pid-1", "--unshare-net", "--unshare-ipc", "--unshare-uts", "--die-with-parent", "--new-session", "--cap-drop", "ALL", "--clearenv", "--setenv", "GODEBUG", "containermaxprocs=0", "--ro-bind", binaryPath, "/writer", "--ro-bind", base + "/cache", "/cache", "--ro-bind", base + "/request.json", "/request.json", "--bind", base + "/metadata", "/metadata", "--proc", "/proc", "--remount-ro", "/proc", "--remount-ro", "/", "--chdir", "/metadata", "--", "/writer", "writer"}
}

func freeze() (plan, error) {
	value := plan{Schema: "combined-synthetic.v1", Root: runRoot, Binary: binaryPath, Timeout: 360, Host: hostSpaces, Argv: map[string][]string{"writer_template": writerArgv("CASE")}, Cases: cases, ExpectedVersions: expectedVersions, ChildEnvironment: childEnvironment()}
	for _, path := range dependencies {
		hash, err := fileHash(path)
		if err != nil {
			return value, err
		}
		value.Pins = append(value.Pins, pin{path, hash})
	}
	return value, validatePlan(value, false)
}

func validatePlan(value plan, execution bool) error {
	if os.Getuid() != 1000 || os.Geteuid() != 1000 || runtime.GOARCH != "amd64" {
		return fmt.Errorf("UID1000 Linux amd64 required")
	}
	self, err := os.Executable()
	if err != nil || self != binaryPath {
		return fmt.Errorf("wrong helper")
	}
	if value.Schema != "combined-synthetic.v1" || value.Root != runRoot || value.Binary != binaryPath || value.Timeout != 360 || !reflect.DeepEqual(value.Host, hostSpaces) || !reflect.DeepEqual(value.ExpectedVersions, expectedVersions) || len(value.Pins) != len(dependencies) || !reflect.DeepEqual(value.Cases, cases) {
		return fmt.Errorf("plan identity mismatch")
	}
	if _, err := os.Lstat(runRoot); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("runroot must be absent")
	}
	if target, err := filepath.EvalSymlinks("/tmp"); err != nil || target != "/tmp" {
		return fmt.Errorf("aliased tmp")
	}
	if _, err := os.Lstat("/etc/ld.so.preload"); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("ambient preload refused")
	}
	if err := validateChildLaunch(value); err != nil {
		return err
	}
	for index, item := range value.Pins {
		if item.Path != dependencies[index] {
			return fmt.Errorf("dependency path drift")
		}
		hash, err := fileHash(item.Path)
		if err != nil || hash != item.SHA256 {
			return fmt.Errorf("dependency hash drift")
		}
		info, err := os.Stat(item.Path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
			return fmt.Errorf("unsafe runtime mode")
		}
		image, err := elf.Open(item.Path)
		if err != nil {
			return err
		}
		libs, libErr := image.ImportedLibraries()
		rpath, pathErr := image.DynString(elf.DT_RPATH)
		runpath, runErr := image.DynString(elf.DT_RUNPATH)
		_ = image.Close()
		if libErr != nil || pathErr != nil || runErr != nil || len(rpath)+len(runpath) != 0 {
			return fmt.Errorf("unreviewed ELF")
		}
		if (item.Path == binaryPath || item.Path == impostorPath) && len(libs) != 0 {
			return fmt.Errorf("helper not static")
		}
		for _, lib := range libs {
			if lib != "libcap.so.2" && lib != "libc.so.6" && lib != "ld-linux-x86-64.so.2" && lib != "libselinux.so.1" && lib != "libpcre2-8.so.0" {
				return fmt.Errorf("unknown dependency")
			}
		}
	}
	if value.Pins[0].SHA256 != bwrapSHA {
		return fmt.Errorf("bwrap drift")
	}
	if execution {
		for name, expected := range hostSpaces {
			actual, err := os.Readlink("/proc/self/ns/" + name)
			if err != nil || actual != expected {
				return fmt.Errorf("not genuine host namespace: %s", name)
			}
		}
	}
	return nil
}

func send(output io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > maxFrame {
		return fmt.Errorf("oversized frame")
	}
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(data)))
	if count, err := output.Write(size[:]); err != nil {
		return err
	} else if count != len(size) {
		return io.ErrShortWrite
	}
	written, err := output.Write(data)
	if err == nil && written != len(data) {
		err = io.ErrShortWrite
	}
	return err
}

func receive(input io.Reader, target any) error {
	var size [4]byte
	if _, err := io.ReadFull(input, size[:]); err != nil {
		return err
	}
	length := binary.BigEndian.Uint32(size[:])
	if length == 0 || length > maxFrame {
		return fmt.Errorf("oversized frame")
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(input, data); err != nil {
		return err
	}
	return strictJSON(data, target)
}

type limitedBuffer struct{ bytes.Buffer }

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	if buffer.Len()+len(data) > 65536 {
		return 0, fmt.Errorf("output limit")
	}
	return buffer.Buffer.Write(data)
}

func command(ctx context.Context, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = childEnvironment()
	cmd.WaitDelay = time.Second
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	return cmd
}

func procIdentity(pid int, port int, instance string) (identity, error) {
	return procIdentityImage(pid, port, instance, binaryPath)
}

func procIdentityImage(pid int, port int, instance, expectedImage string) (identity, error) {
	var directory *os.File
	observed, inspectionErr := inspectIdentity(pid, port, instance, expectedImage, identityReads{readFile: os.ReadFile, readlink: os.Readlink, fdNames: func(path string) ([]string, error) {
		var err error
		directory, err = os.Open(path)
		if err != nil {
			return nil, err
		}
		return directory.Readdirnames(-1)
	}})
	if directory != nil {
		if err := directory.Close(); err != nil {
			return identity{}, errors.Join(inspectionErr, err)
		}
	}
	return observed, inspectionErr
}

func inspectIdentity(pid int, port int, instance, expectedImage string, reads identityReads) (identity, error) {
	base := "/proc/" + strconv.Itoa(pid)
	stat, err := reads.readFile(base + "/stat")
	if err != nil {
		return identity{}, err
	}
	tail := strings.LastIndex(string(stat), ") ")
	if tail < 0 {
		return identity{}, fmt.Errorf("bad proc stat")
	}
	fields := strings.Fields(string(stat)[tail+2:])
	if len(fields) < 20 {
		return identity{}, fmt.Errorf("short proc stat")
	}
	boot, err := reads.readFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return identity{}, err
	}
	exe, err := reads.readlink(base + "/exe")
	if err != nil {
		return identity{}, err
	}
	image, err := reads.readFile(base + "/exe")
	if err != nil {
		return identity{}, err
	}
	observed := identity{PID: pid, Boot: strings.TrimSpace(string(boot)), Start: fields[19], Exe: exe, Hash: digest(image), Instance: instance, Port: port, Version: version}
	if pid <= 0 || observed.Boot == "" || observed.Start == "" || exe == "" || len(image) == 0 {
		return identity{}, fmt.Errorf("incomplete process identity")
	}
	if exe != expectedImage {
		return identity{}, &identityMismatch{Kind: "executable-image", Inspected: observed, ExpectedImage: expectedImage}
	}
	entries, err := reads.fdNames(base + "/fd")
	if err != nil {
		return identity{}, err
	}
	sockets := map[string]bool{}
	for _, entry := range entries {
		target, err := reads.readlink(base + "/fd/" + entry)
		if err != nil {
			return identity{}, err
		}
		if strings.HasPrefix(target, "socket:[") {
			sockets[strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")] = true
		}
	}
	table, err := reads.readFile(base + "/net/tcp")
	if err != nil {
		return identity{}, err
	}
	inode := ""
	address := fmt.Sprintf("0100007F:%04X", port)
	for _, line := range strings.Split(string(table), "\n") {
		columns := strings.Fields(line)
		if len(columns) == 0 || columns[0] == "sl" {
			continue
		}
		if len(columns) < 10 {
			return identity{}, fmt.Errorf("incomplete listener table")
		}
		if len(columns) > 9 && columns[1] == address && columns[3] == "0A" {
			if inode != "" {
				return identity{}, fmt.Errorf("ambiguous listener")
			}
			inode = columns[9]
			if number, err := strconv.ParseUint(inode, 10, 64); err != nil || number == 0 {
				return identity{}, fmt.Errorf("incomplete listener inode")
			}
		}
	}
	if inode == "" {
		return identity{}, fmt.Errorf("listener not owned by supervisor")
	}
	observed.Inode = inode
	if !sockets[inode] {
		owned := make([]string, 0, len(sockets))
		for socket := range sockets {
			owned = append(owned, socket)
		}
		sort.Strings(owned)
		return identity{}, &identityMismatch{Kind: "listener-ownership", Inspected: observed, ExpectedImage: expectedImage, OwnedSockets: owned}
	}
	return observed, nil
}

type apiReply struct {
	Identity identity     `json:"identity"`
	Lease    leaseRequest `json:"lease"`
	Error    string       `json:"error,omitempty"`
}

func serveSupervisor(name string) error {
	listenerFile := os.NewFile(3, "synthetic-listener")
	if listenerFile == nil {
		return fmt.Errorf("missing listener")
	}
	return withListenerOwner(listenerFile, func() error { return serveSupervisorListener(name, listenerFile) })
}

func serveSupervisorListener(name string, listenerFile *os.File) error {
	listener, err := net.FileListener(listenerFile)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	instance, err := randomID()
	if err != nil {
		return err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	self, err := os.Executable()
	if err != nil {
		return err
	}
	actual, err := procIdentityImage(os.Getpid(), port, instance, self)
	if err != nil {
		return err
	}
	slot := leaseSlot{Instance: instance}
	var mutex sync.Mutex
	checks := 0
	transition := make(chan string, 1)
	handler := http.HandlerFunc(func(output http.ResponseWriter, input *http.Request) {
		mutex.Lock()
		defer mutex.Unlock()
		if input.Method != "POST" {
			http.Error(output, "method", http.StatusMethodNotAllowed)
			return
		}
		data, err := io.ReadAll(io.LimitReader(input.Body, maxFrame+1))
		if err != nil {
			http.Error(output, "body", 400)
			return
		}
		var lease leaseRequest
		if err := strictJSON(data, &lease); err != nil {
			http.Error(output, "body", 400)
			return
		}
		switch input.URL.Path {
		case "/health":
		case "/fixture-exit", "/fixture-exec":
			if !strings.HasPrefix(name, "exit-") && !strings.HasPrefix(name, "restart-") && !strings.HasPrefix(name, "exec-") {
				err = fmt.Errorf("fixture transition not declared")
			}
		case "/lease-begin":
			err = slot.begin(lease, mono())
		case "/lease-check":
			checks++
			if (name == "before-loss" && checks == 2) || (name == "after-loss" && checks == 3) {
				slot.Instance = "lost-" + instance
				slot.Active = false
			}
			err = slot.check(lease, instance, mono())
		default:
			err = fmt.Errorf("unsupported endpoint")
		}
		reply := apiReply{Identity: actual, Lease: lease}
		if err != nil {
			reply.Error = err.Error()
		}
		output.Header().Set("Content-Type", "application/json")
		if encodeErr := json.NewEncoder(output).Encode(reply); encodeErr != nil {
			return
		}
		if err == nil && (input.URL.Path == "/fixture-exit" || input.URL.Path == "/fixture-exec") {
			if flusher, ok := output.(http.Flusher); ok {
				flusher.Flush()
			}
			select {
			case transition <- input.URL.Path:
			default:
			}
		}
	})
	server := &http.Server{Handler: supervisorMux(name, handler), ReadHeaderTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: time.Second, MaxHeaderBytes: 4096}
	if err := send(os.Stdout, actual); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	stop := make(chan struct{})
	go func() { var one [1]byte; _, _ = os.Stdin.Read(one[:]); close(stop) }()
	select {
	case <-stop:
	case action := <-transition:
		if action == "/fixture-exec" {
			return execWithListener(listenerFile, listenerExecOps{
				getFlags: func(fd uintptr) (int, error) { return unix.FcntlInt(fd, unix.F_GETFD, 0) },
				setFlags: func(fd uintptr, flags int) error { _, err := unix.FcntlInt(fd, unix.F_SETFD, flags); return err },
				exec: func() error {
					return syscall.Exec(binaryPath, []string{binaryPath, "supervisor", "success"}, childEnvironment())
				},
			})
		}
	case <-time.After(35 * time.Second):
	case err := <-done:
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

func query(ctx context.Context, expected identity, path string, lease leaseRequest) (apiReply, error) {
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("redirect refused") }}
	input, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://127.0.0.1:%d/%s", expected.Port, path), bytes.NewReader(jsonBytes(lease)))
	if err != nil {
		return apiReply{}, err
	}
	response, err := client.Do(input)
	if err != nil {
		return apiReply{}, err
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxFrame+1))
	if err != nil {
		return apiReply{}, err
	}
	return decodeAPIResponse(expected, path, lease, response.StatusCode, data)
}

func readRequest(path string) (request, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return request{}, err
	}
	var value request
	if err := strictJSON(data, &value); err != nil {
		return value, err
	}
	saved := value.Binding.Request
	value.Binding.Request = ""
	if digest(jsonBytes(value)) != saved {
		return value, fmt.Errorf("request digest")
	}
	value.Binding.Request = saved
	if !knownCase(value.Case) || value.ExpectedVersion != expectedVersions[value.Case] {
		return value, fmt.Errorf("unreviewed expected version/case")
	}
	if value.Binding.Protocol != domain || value.Binding.Deadline <= 0 || value.LeaseDeadline-value.Binding.Deadline != 2_000_000_000 {
		return value, fmt.Errorf("request deadline/domain")
	}
	return value, nil
}

func main() {
	if err := dispatch(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func dispatch(args []string) error {
	if imageVariant != "genuine" && imageVariant != "r4-impostor" {
		return fmt.Errorf("unknown image variant")
	}
	if os.Getuid() == 0 || os.Geteuid() == 0 {
		return fmt.Errorf("root refused")
	}
	if len(args) == 1 && args[0] == "version" {
		fmt.Println(version)
		return nil
	}
	if len(args) == 1 && args[0] == "fixture" {
		return serveFixture()
	}
	if len(args) == 1 && args[0] == "freeze" {
		value, err := freeze()
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(value)
	}
	if len(args) == 2 && args[0] == "supervisor" {
		return serveSupervisor(args[1])
	}
	if len(args) == 2 && args[0] == "verifier" {
		return runVerifier(args[1])
	}
	if len(args) == 1 && args[0] == "writer" {
		return runWriter()
	}
	if len(args) == 2 && args[0] == "descendant" {
		if args[1] != "/cache" && !strings.HasPrefix(args[1], runRoot+"/") {
			return fmt.Errorf("fixture-only")
		}
		return json.NewEncoder(os.Stdout).Encode(observe(func() error { return os.WriteFile(args[1]+"/process-write", []byte("synthetic"), 0o600) }))
	}
	if len(args) == 3 && args[0] == "execute-reviewed" {
		data, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		if digest(data) != args[2] {
			return fmt.Errorf("plan digest")
		}
		var value plan
		if err := strictJSON(data, &value); err != nil {
			return err
		}
		if err := validatePlan(value, true); err != nil {
			return err
		}
		return execute(value, args[2])
	}
	if len(args) == 4 && args[0] == "validate-result" {
		if args[3] != "0" {
			return fmt.Errorf("outer execution failed")
		}
		data, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		value, err := decodeObservation(data)
		if err != nil {
			return err
		}
		return validateObservation(value, args[2])
	}
	return fmt.Errorf("reviewed synthetic modes only")
}

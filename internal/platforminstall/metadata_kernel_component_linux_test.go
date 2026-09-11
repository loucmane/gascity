//go:build integration

package platforminstall

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

const metadataKernelRoot = "/tmp/ga-tmgr-kernel-component-run-r1-20260911"

type metadataKernelReply struct {
	Kind  string
	PID   int
	NetNS string
}

type metadataKernelChild struct {
	command *exec.Cmd
	control *os.File
	replies *os.File
	decoder *json.Decoder
	log     *os.File
	cancel  context.CancelFunc
	pidfd   int
	waited  bool
}

func metadataKernelStart(t *testing.T, ctx context.Context, listener *os.File, name string, isolatedNet bool) *metadataKernelChild {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	childInput, parentOutput, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	parentInput, childOutput, err := os.Pipe()
	if err != nil {
		childInput.Close()
		parentOutput.Close()
		t.Fatal(err)
	}
	log, err := os.OpenFile(filepath.Join(metadataKernelRoot, name+".log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		childInput.Close()
		parentOutput.Close()
		parentInput.Close()
		childOutput.Close()
		t.Fatal(err)
	}
	childCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
	command := exec.CommandContext(childCtx, executable, "-test.run=^TestMetadataKernelFixtureProcess$", "-test.count=1", "-test.timeout=40s")
	command.Env = []string{"GODEBUG=containermaxprocs=0", "HOME=/nonexistent", "PATH=/usr/bin:/bin", "GC_METADATA_KERNEL_CHILD=owned-listener-v1"}
	command.ExtraFiles = []*os.File{listener, childInput, childOutput}
	command.Stdout, command.Stderr = log, log
	command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
	command.WaitDelay = 2 * time.Second
	if isolatedNet {
		command.SysProcAttr = &syscall.SysProcAttr{
			Cloneflags:                 unix.CLONE_NEWUSER | unix.CLONE_NEWNET,
			UidMappings:                []syscall.SysProcIDMap{{ContainerID: os.Getuid(), HostID: os.Getuid(), Size: 1}},
			GidMappingsEnableSetgroups: false,
			GidMappings:                []syscall.SysProcIDMap{{ContainerID: os.Getgid(), HostID: os.Getgid(), Size: 1}},
		}
	}
	if err := command.Start(); err != nil {
		cancel()
		childInput.Close()
		parentOutput.Close()
		parentInput.Close()
		childOutput.Close()
		log.Close()
		t.Fatalf("INCONCLUSIVE: owned fixture start unavailable: %v", err)
	}
	childInput.Close()
	childOutput.Close()
	child := &metadataKernelChild{command: command, control: parentOutput, replies: parentInput, decoder: json.NewDecoder(io.LimitReader(parentInput, 4096)), log: log, cancel: cancel, pidfd: -1}
	t.Cleanup(func() {
		child.stop(t)
		if err := errors.Join(child.control.Close(), child.replies.Close(), child.log.Close()); err != nil {
			t.Errorf("fixture descriptor teardown: %v", err)
		}
		if child.pidfd >= 0 {
			if err := unix.Close(child.pidfd); err != nil {
				t.Error(err)
			}
		}
		child.cancel()
	})
	record, err := os.OpenFile(filepath.Join(metadataKernelRoot, "owned-pids.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	err = json.NewEncoder(record).Encode(struct {
		Name        string
		PID         int
		IsolatedNet bool
	}{name, command.Process.Pid, isolatedNet})
	if err := errors.Join(err, record.Sync(), record.Close()); err != nil {
		t.Fatal(err)
	}
	child.pidfd, err = unix.PidfdOpen(command.Process.Pid, 0)
	if err != nil {
		t.Fatalf("INCONCLUSIVE: fixture pidfd unavailable: %v", err)
	}
	child.receive(t, "ready")
	return child
}

func (child *metadataKernelChild) send(command string) error {
	if err := child.control.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	return json.NewEncoder(child.control).Encode(command)
}

func (child *metadataKernelChild) receive(t *testing.T, kind string) metadataKernelReply {
	t.Helper()
	if err := child.replies.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var reply metadataKernelReply
	if err := child.decoder.Decode(&reply); err != nil {
		t.Fatalf("INCONCLUSIVE: fixture reply %s: %v", kind, err)
	}
	if reply.Kind != kind || reply.PID != child.command.Process.Pid || reply.NetNS == "" {
		t.Fatalf("fixture reply mismatch: %+v", reply)
	}
	return reply
}

func (child *metadataKernelChild) stop(t *testing.T) {
	t.Helper()
	if child.waited {
		return
	}
	if err := child.send("stop"); err != nil {
		child.cancel()
	}
	err := child.command.Wait()
	child.waited = true
	if err != nil {
		t.Errorf("INCONCLUSIVE: fixture termination: %v", err)
	}
	if child.pidfd >= 0 {
		polls := []unix.PollFd{{Fd: int32(child.pidfd), Events: unix.POLLIN}}
		count, err := unix.Poll(polls, 0)
		if err != nil || count != 1 || polls[0].Revents&unix.POLLIN == 0 {
			t.Errorf("fixture terminal pidfd unavailable: %d %v", count, err)
		}
	}
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", child.command.Process.Pid)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("INCONCLUSIVE: owned PID absence not established: %v", err)
	}
}

func metadataKernelHost(t *testing.T, pid int, address, listener string) (MetadataHost, string) {
	t.Helper()
	base := fmt.Sprintf("/proc/%d", pid)
	stat, err := os.ReadFile(base + "/stat")
	if err != nil {
		t.Fatal(err)
	}
	separator := strings.LastIndex(string(stat), ") ")
	if separator < 0 {
		t.Fatal("invalid owned stat")
	}
	fields := strings.Fields(string(stat)[separator+2:])
	if len(fields) < 20 {
		t.Fatal("incomplete owned stat")
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Readlink(base + "/exe")
	if err != nil {
		t.Fatal(err)
	}
	image, err := os.ReadFile(base + "/exe")
	if err != nil {
		t.Fatal(err)
	}
	return MetadataHost{PID: pid, Boot: strings.TrimSpace(string(boot)), Start: fields[19], Executable: executable, Address: address, Listener: listener, City: "fixture"}, sha256Hex(image)
}

func TestMetadataKernelComponents(t *testing.T) {
	if os.Getenv("GC_METADATA_KERNEL_TESTS") != "1" {
		t.Skip("opt-in kernel component test not executed")
	}
	if os.Getuid() == 0 || os.Getuid() != os.Geteuid() {
		t.Fatal("root/effective identity refused")
	}
	if err := os.Mkdir(metadataKernelRoot, 0o700); err != nil {
		t.Fatalf("exact absent kernel runroot required: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("INCONCLUSIVE: private listener unavailable: %v", err)
	}
	defer listener.Close()
	listenerFile, err := listener.File()
	if err != nil {
		t.Fatal(err)
	}
	defer listenerFile.Close()
	var listenerStat unix.Stat_t
	if err := unix.Fstat(int(listenerFile.Fd()), &listenerStat); err != nil {
		t.Fatal(err)
	}
	listenerInode := strconv.FormatUint(listenerStat.Ino, 10)
	target := metadataKernelStart(t, ctx, listenerFile, "target", false)
	other := metadataKernelStart(t, ctx, listenerFile, "other-holder", false)
	if err := target.send("accept"); err != nil {
		t.Fatal(err)
	}
	client, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp4", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	acceptedReply := target.receive(t, "accepted")
	targetHost, imageSHA := metadataKernelHost(t, target.command.Process.Pid, listener.Addr().String(), listenerInode)
	otherHost, otherSHA := metadataKernelHost(t, other.command.Process.Pid, listener.Addr().String(), listenerInode)
	reads := productionMetadataHostReads()
	if err := inspectMetadataHost(ctx, targetHost, imageSHA, reads); err != nil {
		t.Fatalf("INCONCLUSIVE target identity: %v", err)
	}
	if err := inspectMetadataHost(ctx, otherHost, otherSHA, reads); err != nil {
		t.Fatalf("INCONCLUSIVE shared-listener holder identity: %v", err)
	}
	socket, err := metadataReadSocket(client)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := metadataAcceptedSocket(ctx, targetHost, acceptedReply.NetNS, socket, "", reads)
	if err != nil {
		t.Fatal(err)
	}
	if accepted == listenerInode {
		t.Fatal("accepted socket confused with listener")
	}
	if _, err := metadataAcceptedSocket(ctx, otherHost, acceptedReply.NetNS, socket, "", reads); err == nil || !strings.Contains(err.Error(), "target accepted-socket custody missing or ambiguous") {
		t.Fatalf("wrong-accepter refusal not attributed: %v", err)
	}
	connection := &metadataConnection{conn: client, socket: socket, accepted: accepted, netns: acceptedReply.NetNS}
	session := &metadataHostSession{manifest: Manifest{Core: Artifact{SHA256: imageSHA}, Metadata: &MetadataLaunch{Host: targetHost}}, pidfd: target.pidfd, connection: connection}
	if err := session.inspect(ctx); err != nil {
		t.Fatal(err)
	}
	if err := target.send("serve-two"); err != nil {
		t.Fatal(err)
	}
	for exchange := 0; exchange < 2; exchange++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+targetHost.Address+"/health", nil)
		if err != nil {
			t.Fatal(err)
		}
		body, err := connection.exchange(ctx, request, session.inspect)
		if err != nil || string(body) != "{}" {
			t.Fatalf("retained connection exchange: %q %v", body, err)
		}
	}
	target.receive(t, "served-two")
	isolated := metadataKernelStart(t, ctx, listenerFile, "isolated-net-holder", true)
	if err := isolated.send("observe"); err != nil {
		t.Fatal(err)
	}
	namespaceReply := isolated.receive(t, "observed")
	if namespaceReply.NetNS == acceptedReply.NetNS {
		t.Fatal("new network namespace not established")
	}
	isolatedHost, _ := metadataKernelHost(t, isolated.command.Process.Pid, targetHost.Address, listenerInode)
	if _, err := metadataAcceptedSocket(ctx, isolatedHost, acceptedReply.NetNS, socket, "", reads); err == nil || !strings.Contains(err.Error(), "connection network namespace drift") {
		t.Fatalf("actual namespace refusal not attributed: %v", err)
	}
	if err := target.send("close"); err != nil {
		t.Fatal(err)
	}
	target.receive(t, "closed")
	if err := client.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var trailing [1]byte
	if count, err := client.Read(trailing[:]); count != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("fixture FIN not observed: %d %v", count, err)
	}
	if _, err := metadataReadSocket(client); err == nil {
		t.Fatal("closed connection accepted")
	}
	if err := connection.inspect(ctx, targetHost); err == nil {
		t.Fatal("closed lifetime accepted")
	}
	target.stop(t)
	if err := session.inspect(ctx); err == nil {
		t.Fatal("terminal supervisor accepted")
	}
	other.stop(t)
	isolated.stop(t)
	if t.Failed() {
		return
	}
	result := struct {
		ComponentOnly bool
		Target        MetadataHost
		Other         MetadataHost
		Client        metadataSocket
		Accepted      string
		HostNetNS     string
		IsolatedNetNS string
	}{true, targetHost, otherHost, socket, accepted, acceptedReply.NetNS, namespaceReply.NetNS}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadataKernelRoot, "component-observations.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMetadataKernelFixtureProcess(t *testing.T) {
	if os.Getenv("GC_METADATA_KERNEL_CHILD") != "owned-listener-v1" {
		t.Skip("owned subprocess only")
	}
	if os.Getuid() == 0 || os.Getuid() != os.Geteuid() {
		t.Fatal("fixture root/effective identity refused")
	}
	if err := unix.SetNonblock(4, true); err != nil {
		t.Fatal(err)
	}
	if err := unix.SetNonblock(5, true); err != nil {
		t.Fatal(err)
	}
	listenerFile := os.NewFile(3, "owned-listener")
	control := os.NewFile(4, "owned-control")
	replies := os.NewFile(5, "owned-replies")
	defer listenerFile.Close()
	defer control.Close()
	defer replies.Close()
	listener, err := net.FileListener(listenerFile)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	tcpListener, ok := listener.(*net.TCPListener)
	if !ok {
		t.Fatal("non-TCP fixture listener")
	}
	if err := control.SetReadDeadline(time.Now().Add(35 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := replies.SetWriteDeadline(time.Now().Add(35 * time.Second)); err != nil {
		t.Fatal(err)
	}
	netns, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	reply := func(kind string) {
		if err := json.NewEncoder(replies).Encode(metadataKernelReply{kind, os.Getpid(), netns}); err != nil {
			t.Fatal(err)
		}
	}
	reply("ready")
	decoder := json.NewDecoder(io.LimitReader(control, 256))
	var accepted net.Conn
	defer func() {
		if accepted != nil {
			accepted.Close()
		}
	}()
	for count := 0; count < 8; count++ {
		var command string
		if err := decoder.Decode(&command); err != nil {
			t.Fatal(err)
		}
		switch command {
		case "accept":
			if accepted != nil {
				t.Fatal("duplicate accept")
			}
			if err := tcpListener.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
				t.Fatal(err)
			}
			accepted, err = tcpListener.Accept()
			if err != nil {
				t.Fatal(err)
			}
			reply("accepted")
		case "serve-two":
			if accepted == nil {
				t.Fatal("accepted connection absent")
			}
			if err := accepted.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
				t.Fatal(err)
			}
			reader := bufio.NewReader(io.LimitReader(accepted, 8192))
			for exchange := 0; exchange < 2; exchange++ {
				request, err := http.ReadRequest(reader)
				if err != nil {
					t.Fatal(err)
				}
				if request.Method != http.MethodGet || request.RequestURI != "/health" || request.ContentLength != 0 {
					t.Fatal("fixture request refused")
				}
				if err := request.Body.Close(); err != nil {
					t.Fatal(err)
				}
				if _, err := io.WriteString(accepted, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\n{}"); err != nil {
					t.Fatal(err)
				}
			}
			reply("served-two")
		case "observe":
			reply("observed")
		case "close":
			if accepted == nil {
				t.Fatal("accepted connection absent")
			}
			if err := accepted.Close(); err != nil {
				t.Fatal(err)
			}
			accepted = nil
			reply("closed")
		case "stop":
			return
		default:
			t.Fatal("unknown fixture command")
		}
	}
	t.Fatal("fixture command limit")
}

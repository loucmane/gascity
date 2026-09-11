package platforminstall

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type metadataSocket struct {
	Local  string
	Remote string
	Inode  uint64
	Cookie uint64
}

const metadataTCPEstablished = 1

func metadataConnectionRoute(city, action string) (string, string, error) {
	if len(city) == 0 || len(city) > 128 {
		return "", "", fmt.Errorf("metadata connection city is not a canonical route segment")
	}
	for _, character := range city {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '-' && character != '_' {
			return "", "", fmt.Errorf("metadata connection city is not a canonical route segment")
		}
	}
	switch action {
	case "health":
		return http.MethodGet, "/health", nil
	case "begin", "check":
		return http.MethodPost, "/v0/city/" + city + "/platform/metadata-lease/" + action, nil
	default:
		return "", "", fmt.Errorf("metadata connection route refused")
	}
}

func metadataSocketAddress(address unix.Sockaddr) (string, error) {
	inet, ok := address.(*unix.SockaddrInet4)
	if !ok || inet.Addr != [4]byte{127, 0, 0, 1} || inet.Port < 1 {
		return "", fmt.Errorf("metadata connection is not direct IPv4 loopback")
	}
	return fmt.Sprintf("0100007F:%04X", inet.Port), nil
}

func metadataReadSocket(connection net.Conn) (metadataSocket, error) {
	var observation metadataSocket
	owner, ok := connection.(syscall.Conn)
	if !ok {
		return observation, fmt.Errorf("metadata connection lacks a retained kernel socket")
	}
	raw, err := owner.SyscallConn()
	if err != nil {
		return observation, err
	}
	var inspectErr error
	err = raw.Control(func(descriptor uintptr) {
		fd := int(descriptor)
		var stat unix.Stat_t
		if inspectErr = unix.Fstat(fd, &stat); inspectErr != nil {
			return
		}
		if stat.Mode&unix.S_IFMT != unix.S_IFSOCK || stat.Ino == 0 {
			inspectErr = fmt.Errorf("metadata connection descriptor is not a socket")
			return
		}
		observation.Inode = stat.Ino
		observation.Cookie, inspectErr = unix.GetsockoptUint64(fd, unix.SOL_SOCKET, unix.SO_COOKIE)
		if inspectErr != nil {
			return
		}
		local, localErr := unix.Getsockname(fd)
		remote, remoteErr := unix.Getpeername(fd)
		if inspectErr = errors.Join(localErr, remoteErr); inspectErr != nil {
			return
		}
		observation.Local, inspectErr = metadataSocketAddress(local)
		if inspectErr != nil {
			return
		}
		observation.Remote, inspectErr = metadataSocketAddress(remote)
		if inspectErr != nil {
			return
		}
		info, infoErr := unix.GetsockoptTCPInfo(fd, unix.IPPROTO_TCP, unix.TCP_INFO)
		if infoErr != nil {
			inspectErr = infoErr
			return
		}
		if info.State != metadataTCPEstablished || observation.Cookie == 0 {
			inspectErr = fmt.Errorf("metadata connection is no longer established")
			return
		}
		var pending [1]byte
		_, _, peekErr := unix.Recvfrom(fd, pending[:], unix.MSG_PEEK|unix.MSG_DONTWAIT)
		if peekErr != unix.EAGAIN && peekErr != unix.EWOULDBLOCK {
			if peekErr == nil {
				inspectErr = fmt.Errorf("metadata connection closed or has unsolicited data")
			} else {
				inspectErr = fmt.Errorf("metadata connection closed or has unsolicited data: %w", peekErr)
			}
		}
	})
	return observation, errors.Join(err, inspectErr)
}

func metadataAcceptedSocket(ctx context.Context, host MetadataHost, netns string, socket metadataSocket, expected string, reads metadataHostReads) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if netns == "" || socket.Inode == 0 || socket.Cookie == 0 || socket.Local == socket.Remote {
		return "", fmt.Errorf("incomplete connection identity")
	}
	base := "/proc/" + strconv.Itoa(host.PID)
	for _, target := range []string{"/proc/self/ns/net", base + "/ns/net"} {
		actual, err := reads.link(target)
		if err != nil {
			return "", err
		}
		if actual != netns {
			return "", fmt.Errorf("connection network namespace drift")
		}
	}
	port, err := metadataEndpoint(host.Address)
	if err != nil {
		return "", err
	}
	if socket.Remote != fmt.Sprintf("0100007F:%04X", port) {
		return "", fmt.Errorf("connection endpoint drift")
	}
	table, err := reads.read(base + "/net/tcp")
	if err != nil {
		return "", err
	}
	accepted, forwardCount, reverseCount := "", 0, 0
	for _, line := range strings.Split(string(table), "\n") {
		columns := strings.Fields(line)
		if len(columns) == 0 || columns[0] == "sl" {
			continue
		}
		if len(columns) < 10 {
			return "", fmt.Errorf("incomplete connection table")
		}
		forward := columns[1] == socket.Local && columns[2] == socket.Remote
		reverse := columns[1] == socket.Remote && columns[2] == socket.Local
		if !forward && !reverse {
			continue
		}
		inode, parseErr := strconv.ParseUint(columns[9], 10, 64)
		if parseErr != nil || inode == 0 || columns[3] != "01" {
			return "", fmt.Errorf("connection row not live")
		}
		if forward {
			forwardCount++
			if inode != socket.Inode {
				return "", fmt.Errorf("client socket inode drift")
			}
		} else {
			reverseCount++
			accepted = columns[9]
			if accepted == host.Listener || inode == socket.Inode || expected != "" && accepted != expected {
				return "", fmt.Errorf("accepted socket identity drift")
			}
		}
	}
	if forwardCount != 1 || reverseCount != 1 {
		return "", fmt.Errorf("missing or ambiguous connection tuple")
	}
	descriptors, err := reads.fds(base + "/fd")
	if err != nil {
		return "", err
	}
	owned := 0
	for _, descriptor := range descriptors {
		target, err := reads.link(base + "/fd/" + descriptor)
		if err != nil {
			return "", err
		}
		if target == "socket:["+accepted+"]" {
			owned++
		}
	}
	if owned != 1 {
		return "", fmt.Errorf("target accepted-socket custody missing or ambiguous")
	}
	for _, target := range []string{"/proc/self/ns/net", base + "/ns/net"} {
		actual, err := reads.link(target)
		if err != nil {
			return "", err
		}
		if actual != netns {
			return "", fmt.Errorf("connection network namespace changed during inspection")
		}
	}
	return accepted, ctx.Err()
}

func validateMetadataSocketContinuity(expected, actual metadataSocket) error {
	if expected.Inode == 0 || expected.Cookie == 0 || expected != actual {
		return fmt.Errorf("retained metadata socket changed")
	}
	return nil
}

type metadataConnection struct {
	mu       sync.Mutex
	conn     net.Conn
	socket   metadataSocket
	accepted string
	netns    string
	failed   bool
}

func (connection *metadataConnection) inspect(ctx context.Context, host MetadataHost) error {
	if connection.failed || connection.conn == nil {
		return fmt.Errorf("metadata connection is permanently unavailable")
	}
	before, err := metadataReadSocket(connection.conn)
	if err != nil {
		return err
	}
	if err := validateMetadataSocketContinuity(connection.socket, before); err != nil {
		return err
	}
	_, err = metadataAcceptedSocket(ctx, host, connection.netns, before, connection.accepted, productionMetadataHostReads())
	if err != nil {
		return err
	}
	after, err := metadataReadSocket(connection.conn)
	if err != nil {
		return err
	}
	if err := validateMetadataSocketContinuity(before, after); err != nil {
		return err
	}
	return ctx.Err()
}

func (connection *metadataConnection) exchange(ctx context.Context, request *http.Request, inspect func(context.Context) error) (_ []byte, returnErr error) {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	if connection.failed || connection.conn == nil {
		return nil, fmt.Errorf("metadata connection cannot reconnect or retry")
	}
	defer func() {
		if returnErr != nil {
			connection.failed = true
			returnErr = errors.Join(returnErr, connection.conn.Close())
		}
	}()
	return metadataHTTPExchange(ctx, connection.conn, request, inspect)
}

func metadataHTTPExchange(ctx context.Context, connection net.Conn, request *http.Request, inspect func(context.Context) error) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	deadline, _ := ctx.Deadline()
	if err := connection.SetDeadline(deadline); err != nil {
		return nil, err
	}
	interrupted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = connection.Close(); close(interrupted) })
	defer func() {
		if !stop() {
			<-interrupted
		}
	}()
	if err := inspect(ctx); err != nil {
		return nil, err
	}
	if err := request.Write(connection); err != nil {
		return nil, err
	}
	reader := bufio.NewReaderSize(io.LimitReader(connection, 8192+metadataFrameLimit+1), 4096)
	var header bytes.Buffer
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if header.Len()+len(line) > 8192 {
			return nil, fmt.Errorf("metadata response headers exceed 8192 bytes")
		}
		header.WriteString(line)
		if line == "\r\n" {
			break
		}
	}
	responseReader := bufio.NewReader(io.MultiReader(bytes.NewReader(header.Bytes()), reader))
	response, err := http.ReadResponse(responseReader, request)
	if err != nil {
		return nil, err
	}
	if response.ProtoMajor != 1 || response.ProtoMinor != 1 || response.Close || response.StatusCode != http.StatusOK || response.Header.Get("Upgrade") != "" || response.Header.Get("Connection") != "" || response.ContentLength < 0 || response.ContentLength > metadataFrameLimit || len(response.TransferEncoding) != 0 {
		return nil, errors.Join(fmt.Errorf("metadata response protocol/status/framing refused: %d", response.StatusCode), response.Body.Close())
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, metadataFrameLimit+1))
	if readErr != nil || int64(len(data)) != response.ContentLength || responseReader.Buffered() != 0 || reader.Buffered() != 0 {
		return nil, errors.Join(fmt.Errorf("metadata response incomplete or unsolicited"), readErr, response.Body.Close())
	}
	if request.Method == http.MethodPost && response.Header.Get("Cache-Control") != "no-store" {
		return nil, errors.Join(fmt.Errorf("lease response cache policy refused"), response.Body.Close())
	}
	inspectErr := inspect(ctx)
	if err := errors.Join(inspectErr, response.Body.Close(), ctx.Err()); err != nil {
		return nil, err
	}
	return data, nil
}

package platforminstall

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/api/genclient"
	"golang.org/x/sys/unix"
)

type metadataHostReads struct {
	read func(string) ([]byte, error)
	link func(string) (string, error)
	fds  func(string) ([]string, error)
}

func metadataEndpoint(address string) (int, error) {
	host, portText, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		return 0, fmt.Errorf("metadata verifier requires exact IPv4 loopback TCP endpoint")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 || strconv.Itoa(port) != portText {
		return 0, fmt.Errorf("noncanonical metadata TCP port")
	}
	return port, nil
}

func inspectMetadataHost(ctx context.Context, expected MetadataHost, imageSHA string, reads metadataHostReads) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	port, err := metadataEndpoint(expected.Address)
	if err != nil {
		return err
	}
	if expected.PID <= 1 {
		return fmt.Errorf("invalid supervisor PID")
	}
	base := "/proc/" + strconv.Itoa(expected.PID)
	stat, err := reads.read(base + "/stat")
	if err != nil {
		return err
	}
	separator := strings.LastIndex(string(stat), ") ")
	if separator < 0 {
		return fmt.Errorf("invalid supervisor stat")
	}
	fields := strings.Fields(string(stat)[separator+2:])
	if len(fields) < 20 || fields[19] != expected.Start || fields[0] == "Z" || fields[0] == "X" {
		return fmt.Errorf("supervisor start/state drift")
	}
	boot, err := reads.read("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(boot)) != expected.Boot || expected.Boot == "" {
		return fmt.Errorf("supervisor boot drift")
	}
	executable, err := reads.link(base + "/exe")
	if err != nil {
		return err
	}
	if executable != expected.Executable {
		return fmt.Errorf("supervisor executable path drift")
	}
	image, err := reads.read(base + "/exe")
	if err != nil {
		return err
	}
	if len(image) == 0 || sha256Hex(image) != imageSHA {
		return fmt.Errorf("supervisor executable digest drift")
	}
	descriptors, err := reads.fds(base + "/fd")
	if err != nil {
		return err
	}
	owned := false
	for _, descriptor := range descriptors {
		target, err := reads.link(base + "/fd/" + descriptor)
		if err != nil {
			return err
		}
		if target == "socket:["+expected.Listener+"]" {
			owned = true
		}
	}
	if !owned {
		return fmt.Errorf("supervisor does not own frozen listener")
	}
	table, err := reads.read(base + "/net/tcp")
	if err != nil {
		return err
	}
	wanted := fmt.Sprintf("0100007F:%04X", port)
	count := 0
	for _, line := range strings.Split(string(table), "\n") {
		columns := strings.Fields(line)
		if len(columns) == 0 || columns[0] == "sl" {
			continue
		}
		if len(columns) < 10 {
			return fmt.Errorf("incomplete TCP ownership evidence")
		}
		if columns[1] == wanted && columns[3] == "0A" {
			count++
			if columns[9] != expected.Listener {
				return fmt.Errorf("TCP listener identity drift")
			}
		}
	}
	if count != 1 {
		return fmt.Errorf("missing or shared TCP listener")
	}
	return ctx.Err()
}

func productionMetadataHostReads() metadataHostReads {
	return metadataHostReads{read: os.ReadFile, link: os.Readlink, fds: func(path string) ([]string, error) {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		return names, nil
	}}
}

type metadataHostSession struct {
	manifest   Manifest
	pidfd      int
	connection *metadataConnection
	binding    MetadataBinding
	proof      RuntimeProof
}

func metadataRandomID() (string, error) {
	var value [32]byte
	_, err := rand.Read(value[:])
	return hex.EncodeToString(value[:]), err
}

func (session *metadataHostSession) close() error {
	var connectionErr error
	if session.connection != nil {
		connectionErr = session.connection.conn.Close()
		session.connection.failed = true
	}
	if session.pidfd < 0 {
		return connectionErr
	}
	err := unix.Close(session.pidfd)
	session.pidfd = -1
	return errors.Join(connectionErr, err)
}

func (session *metadataHostSession) inspect(ctx context.Context) error {
	if session.pidfd < 0 {
		return fmt.Errorf("supervisor reference closed")
	}
	for _, phase := range []string{"before", "after"} {
		var owner unix.Stat_t
		if err := unix.Stat(fmt.Sprintf("/proc/%d", session.manifest.Metadata.Host.PID), &owner); err != nil {
			return err
		}
		if owner.Uid != uint32(os.Geteuid()) {
			return fmt.Errorf("supervisor owner differs")
		}
		polls := []unix.PollFd{{Fd: int32(session.pidfd), Events: unix.POLLIN}}
		count, err := unix.Poll(polls, 0)
		if err != nil || count != 0 {
			return fmt.Errorf("supervisor terminal/unavailable %s inspection: %w", phase, err)
		}
		if phase == "before" {
			if err := inspectMetadataHost(ctx, session.manifest.Metadata.Host, session.manifest.Core.SHA256, productionMetadataHostReads()); err != nil {
				return err
			}
			if session.connection != nil {
				if err := session.connection.inspect(ctx, session.manifest.Metadata.Host); err != nil {
					return err
				}
				if err := inspectMetadataHost(ctx, session.manifest.Metadata.Host, session.manifest.Core.SHA256, productionMetadataHostReads()); err != nil {
					return err
				}
			}
		}
	}
	return ctx.Err()
}

type metadataBoundedOutput struct{ buffer bytes.Buffer }

func (output *metadataBoundedOutput) Len() int       { return output.buffer.Len() }
func (output *metadataBoundedOutput) String() string { return output.buffer.String() }

func (output *metadataBoundedOutput) Write(data []byte) (int, error) {
	if output.Len()+len(data) > 8192 {
		return 0, fmt.Errorf("metadata helper output exceeded 8192 bytes")
	}
	return output.buffer.Write(data)
}

func (session *metadataHostSession) exchange(ctx context.Context, action string, input, output any) (returnErr error) {
	defer func() {
		if returnErr != nil && session.connection != nil {
			session.connection.failed = true
			returnErr = errors.Join(returnErr, session.connection.conn.Close())
		}
	}()
	method, path, err := metadataConnectionRoute(session.manifest.Metadata.Host.City, action)
	if err != nil {
		return err
	}
	switch action {
	case "health":
		if input != nil {
			return fmt.Errorf("health request body refused")
		}
	case "begin":
		if _, ok := input.(MetadataLeaseRequest); !ok {
			return fmt.Errorf("begin request type refused")
		}
	case "check":
		if _, ok := input.(MetadataLeaseCheck); !ok {
			return fmt.Errorf("check request type refused")
		}
	}
	if session.connection == nil {
		return fmt.Errorf("retained metadata connection absent")
	}
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://"+session.manifest.Metadata.Host.Address+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Cache-Control", "no-store")
	request.Header.Set("X-GC-Request", "metadata-verifier")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	data, err := session.connection.exchange(ctx, request, session.inspect)
	if err != nil {
		return err
	}
	if err := rejectDuplicateMetadataFields(data, output); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func validateMetadataLeaseReply(proof MetadataLeaseProof, request MetadataLeaseRequest, instance string, before, after int64) error {
	if proof.Request != request || proof.Instance != instance || before < 0 || after < before || proof.Observed < before || proof.Observed > after || after >= request.Deadline {
		return fmt.Errorf("metadata lease request/instance/observation/deadline drift")
	}
	return validateSHA256("lease instance", proof.Instance)
}

func (session *metadataHostSession) leaseRequest() MetadataLeaseRequest {
	return MetadataLeaseRequest{Transaction: session.binding.Transaction, RequestSHA256: session.binding.Request, Nonce: session.binding.Nonce, Deadline: session.binding.LeaseEnd, Observation: session.binding.Observation}
}

func (session *metadataHostSession) check(ctx context.Context) error {
	before, err := metadataMonotonicNow()
	if err != nil {
		return err
	}
	if before >= session.binding.Deadline {
		return fmt.Errorf("metadata grant expired")
	}
	var proof MetadataLeaseProof
	if err := session.exchange(ctx, "check", MetadataLeaseCheck{session.leaseRequest(), session.binding.Instance}, &proof); err != nil {
		return err
	}
	after, err := metadataMonotonicNow()
	if err != nil {
		return err
	}
	if after >= session.binding.Deadline {
		return fmt.Errorf("metadata grant expired during check")
	}
	if err := validateMetadataLeaseReply(proof, session.leaseRequest(), session.binding.Instance, before, after); err != nil {
		return err
	}
	// Recheck the protected host layout at every existing grant checkpoint.
	if err := metadataProtectedHostCheckpoint(session.manifest); err != nil {
		return err
	}
	if session.manifest.Metadata != nil && len(session.manifest.Metadata.ProtectedTrees) > 0 {
		return metadataNowBefore(session.binding)
	}
	return nil
}

func openMetadataHost(ctx context.Context, manifest Manifest) (_ *metadataHostSession, returnErr error) {
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}
	if manifest.Metadata == nil {
		return nil, fmt.Errorf("metadata verifier requires versioned launch authority")
	}
	want, err := ManifestDigest(manifest)
	if err != nil || want != manifest.ManifestSHA256 {
		return nil, fmt.Errorf("metadata manifest digest refused")
	}
	if os.Geteuid() == 0 || os.Getuid() != os.Geteuid() {
		return nil, fmt.Errorf("metadata verifier refuses root or changed effective identity")
	}
	for space, expected := range manifest.Metadata.Namespaces {
		actual, err := os.Readlink("/proc/self/ns/" + space)
		if err != nil || actual != expected {
			return nil, fmt.Errorf("verifier namespace %s differs", space)
		}
	}
	if _, err := metadataEndpoint(manifest.Metadata.Host.Address); err != nil {
		return nil, err
	}
	if _, _, err := metadataConnectionRoute(manifest.Metadata.Host.City, "health"); err != nil {
		return nil, err
	}
	if err := metadataCustodyImage(manifest.Core.SHA256); err != nil {
		return nil, err
	}
	var owner unix.Stat_t
	if err := unix.Stat(fmt.Sprintf("/proc/%d", manifest.Metadata.Host.PID), &owner); err != nil {
		return nil, err
	}
	if owner.Uid != uint32(os.Geteuid()) {
		return nil, fmt.Errorf("supervisor owner differs")
	}
	pidfd, err := unix.PidfdOpen(manifest.Metadata.Host.PID, 0)
	if err != nil {
		return nil, err
	}
	session := &metadataHostSession{manifest: manifest, pidfd: pidfd}
	defer func() {
		if returnErr != nil {
			returnErr = errors.Join(returnErr, session.close())
		}
	}()
	if err := session.inspect(ctx); err != nil {
		return nil, err
	}
	versionCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(versionCtx, fmt.Sprintf("/proc/%d/exe", manifest.Metadata.Host.PID), "version")
	command.Env = []string{"GODEBUG=containermaxprocs=0"}
	command.WaitDelay = time.Second
	var version metadataBoundedOutput
	command.Stdout = &version
	command.Stderr = &version
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("observed supervisor version failed: %w", err)
	}
	if err := session.inspect(ctx); err != nil {
		return nil, err
	}
	connection, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp4", manifest.Metadata.Host.Address)
	if err != nil {
		return nil, err
	}
	session.connection = &metadataConnection{conn: connection, netns: manifest.Metadata.Namespaces["net"]}
	session.connection.socket, err = metadataReadSocket(connection)
	if err != nil {
		return nil, err
	}
	session.connection.accepted, err = metadataAcceptedSocket(ctx, manifest.Metadata.Host, session.connection.netns, session.connection.socket, "", productionMetadataHostReads())
	if err != nil {
		return nil, err
	}
	var health genclient.SupervisorHealthOutputBody
	if err := session.exchange(ctx, "health", nil, &health); err != nil {
		return nil, err
	}
	if health.BuildId == nil {
		return nil, fmt.Errorf("supervisor API lacks BuildID")
	}
	session.proof = RuntimeProof{manifest.Core.SHA256, *health.BuildId, strings.TrimSpace(version.String())}
	if err := validateRuntimeProof(manifest, session.proof); err != nil {
		return nil, err
	}
	if health.Status != "ok" {
		return nil, fmt.Errorf("supervisor health refused")
	}
	return session, nil
}

func (session *metadataHostSession) begin(ctx context.Context, observation bool) error {
	if session.binding.Protocol != "" {
		return fmt.Errorf("lease cannot be renewed")
	}
	start, err := metadataMonotonicNow()
	if err != nil {
		return err
	}
	nonce, err := metadataRandomID()
	if err != nil {
		return err
	}
	channel, err := metadataRandomID()
	if err != nil {
		return err
	}
	session.binding = MetadataBinding{Protocol: metadataProtocol, Transaction: session.manifest.Metadata.Transaction, Attempt: session.manifest.Metadata.Attempt, Nonce: nonce, Channel: channel, Request: session.manifest.ManifestSHA256, Deadline: start + 25_000_000_000, LeaseEnd: start + 28_000_000_000, Observation: observation}
	var proof MetadataLeaseProof
	if err := session.exchange(ctx, "begin", session.leaseRequest(), &proof); err != nil {
		return err
	}
	after, err := metadataMonotonicNow()
	if err != nil {
		return err
	}
	if err := validateMetadataLeaseReply(proof, session.leaseRequest(), proof.Instance, start, after); err != nil {
		return err
	}
	session.binding.Instance = proof.Instance
	return validateMetadataBinding(session.binding)
}

// VerifyMetadataRuntime verifies current host identity without publishing metadata.
func VerifyMetadataRuntime(ctx context.Context, manifest Manifest) (_ RuntimeProof, returnErr error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	session, err := openMetadataHost(ctx, manifest)
	if err != nil {
		return RuntimeProof{}, err
	}
	defer func() { returnErr = errors.Join(returnErr, session.close()) }()
	if err := session.begin(ctx, true); err != nil {
		return RuntimeProof{}, err
	}
	if err := session.check(ctx); err != nil {
		return RuntimeProof{}, err
	}
	return session.proof, nil
}

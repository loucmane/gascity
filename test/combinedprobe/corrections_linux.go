package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

type observationHeader struct {
	Schema      string      `json:"schema"`
	Observation observation `json:"observation"`
}
type observationCase struct {
	Name    string  `json:"name"`
	Outcome outcome `json:"outcome"`
}

func encodeObservation(value observation) ([]byte, error) {
	var output bytes.Buffer
	header := value
	header.Cases = nil
	if len(value.Cases) > len(cases) {
		return nil, fmt.Errorf("extra observation cases")
	}
	if err := send(&output, observationHeader{"combined-observation.frames.v1", header}); err != nil {
		return nil, err
	}
	for _, name := range cases {
		if err := send(&output, observationCase{name, value.Cases[name]}); err != nil {
			return nil, err
		}
	}
	return output.Bytes(), nil
}

func decodeObservation(data []byte) (observation, error) {
	if len(data) > (len(cases)+1)*(maxFrame+4) {
		return observation{}, fmt.Errorf("oversized observation stream")
	}
	input := bytes.NewReader(data)
	var header observationHeader
	if err := receive(input, &header); err != nil {
		return observation{}, err
	}
	if header.Schema != "combined-observation.frames.v1" || header.Observation.Cases != nil {
		return observation{}, fmt.Errorf("invalid observation header")
	}
	value := header.Observation
	value.Cases = map[string]outcome{}
	for _, name := range cases {
		var record observationCase
		if err := receive(input, &record); err != nil {
			return value, err
		}
		if record.Name != name {
			return value, fmt.Errorf("observation case order/identity mismatch")
		}
		value.Cases[name] = record.Outcome
	}
	if input.Len() != 0 {
		return value, fmt.Errorf("trailing observation frames")
	}
	return value, nil
}

const (
	controlFreshnessNS int64 = 10_000_000_000
	fixtureMarker            = "m5-public-synthetic-marker"
	fixtureMemory            = "m5-public-synthetic-memory"
)

type versionMismatch struct{ Expected, Observed string }

func (mismatch *versionMismatch) Error() string {
	return fmt.Sprintf("observed version mismatch: expected %q, got %q", mismatch.Expected, mismatch.Observed)
}

type versionEvidence struct {
	Executable string   `json:"executable"`
	Expected   string   `json:"expected"`
	Observed   string   `json:"observed"`
	Process    identity `json:"process"`
}

func observedVersionArgv(pid int) []string {
	return []string{"/proc/" + strconv.Itoa(pid) + "/exe", "version"}
}

func compareVersion(expected string, output []byte, commandErr error) error {
	if commandErr != nil {
		return fmt.Errorf("observed image version execution failed: %w", commandErr)
	}
	if expected == "" {
		return fmt.Errorf("expected version absent")
	}
	if string(output) != expected+"\n" {
		return &versionMismatch{expected, string(output)}
	}
	return nil
}

func writerTerminalEvidence(pid, waitedPID int, waitObserved, pidfdTerminal bool) bool {
	return pid > 0 && waitedPID == pid && waitObserved && pidfdTerminal
}

func validateWriterOutcome(value outcome, started bool) error {
	if !started {
		if value.WriterState != "writer_not_started" || value.WriterPID != 0 || value.WriterWaitPID != 0 || value.WriterWaitObserved || value.WriterPIDFDTerminal || value.WriterReaped || value.WriterExit != -1 {
			return fmt.Errorf("unstarted writer has fabricated terminal evidence")
		}
		return nil
	}
	if value.WriterState != "writer_terminal" || !value.WriterReaped || !writerTerminalEvidence(value.WriterPID, value.WriterWaitPID, value.WriterWaitObserved, value.WriterPIDFDTerminal) {
		return fmt.Errorf("writer terminal evidence incomplete")
	}
	return nil
}

type apiStatusError struct {
	Path   string `json:"path"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

func (failure *apiStatusError) Error() string {
	return fmt.Sprintf("API /%s HTTP %d: %q", failure.Path, failure.Status, failure.Detail)
}

func absentLeaseRefusal(failure *apiStatusError) bool {
	return failure != nil && failure.Path == "lease-begin" && failure.Status == http.StatusNotFound && failure.Detail == "404 page not found\n"
}

func supervisorMux(name string, handler http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/health", handler)
	if strings.HasPrefix(name, "exit-") || strings.HasPrefix(name, "exec-") || strings.HasPrefix(name, "restart-") {
		mux.Handle("/fixture-exit", handler)
		mux.Handle("/fixture-exec", handler)
	}
	if name != "unsupported" {
		mux.Handle("/lease-begin", handler)
		mux.Handle("/lease-check", handler)
	}
	return mux
}

func decodeAPIResponse(expected identity, path string, lease leaseRequest, status int, data []byte) (apiReply, error) {
	if len(data) > maxFrame {
		return apiReply{}, fmt.Errorf("oversized API response")
	}
	if status != http.StatusOK {
		return apiReply{}, &apiStatusError{Path: path, Status: status, Detail: string(data)}
	}
	var reply apiReply
	if err := strictJSON(data, &reply); err != nil {
		return reply, err
	}
	if reply.Error != "" || reply.Identity != expected || reply.Lease != lease {
		return reply, fmt.Errorf("API lease/instance refusal: %s", reply.Error)
	}
	return reply, nil
}

type attackTarget struct {
	PID           int    `json:"pid"`
	FD            int    `json:"fd"`
	Address       uint64 `json:"address"`
	Boot          string `json:"boot"`
	Start         string `json:"start"`
	ExecutableSHA string `json:"executable_sha"`
	MarkerSHA     string `json:"marker_sha"`
	MemorySHA     string `json:"memory_sha"`
}

func targetForFixture(value fixtureIdentity) attackTarget {
	return attackTarget{value.PID, value.FD, value.Address, value.Boot, value.Start, value.ExecutableSHA, digest([]byte(value.Marker)), digest([]byte(value.Memory))}
}

func validFixture(value fixtureIdentity) bool {
	return value.PID > 1 && value.FD > 2 && value.Address != 0 && value.Boot != "" && value.Start != "" && value.ExecutableSHA != "" && value.Marker == fixtureMarker && value.Memory == fixtureMemory
}

func validateProcessAttempts(values map[string]attempt, fixture fixtureIdentity, positive bool) error {
	if !validFixture(fixture) || len(values) != 4 {
		return fmt.Errorf("missing/invalid fixture control identity")
	}
	expected := targetForFixture(fixture)
	for _, name := range []string{"signal", "memory", "fd", "ptrace"} {
		value, exists := values[name]
		if !exists || value.Target == nil || *value.Target != expected {
			return fmt.Errorf("unbound process target: %s", name)
		}
		if positive {
			if !value.OK || value.Errno != 0 || value.FailureKind != "" {
				return fmt.Errorf("positive fixture unavailable: %s", name)
			}
		} else if !processDenial(name, value) {
			return fmt.Errorf("negative process control inconclusive: %s", name)
		}
	}
	return nil
}

func validateControlRefresh(value outcome, fixture fixtureIdentity) error {
	negative := value.Boundary.AttacksCompletedAt
	if negative <= 0 || value.PostControlsStartedAt < negative || value.PostControlsCompletedAt < value.PostControlsStartedAt || value.PostControlsCompletedAt-negative > controlFreshnessNS {
		return fmt.Errorf("stale/out-of-order refreshed fixture controls")
	}
	return validateProcessAttempts(value.PostControls, fixture, true)
}

func fixtureProcessIdentity(pid int) (boot, start, hash string, err error) {
	base := "/proc/" + strconv.Itoa(pid)
	image, err := os.Readlink(base + "/exe")
	if err != nil || image != binaryPath {
		return "", "", "", fmt.Errorf("fixture image identity changed")
	}
	data, err := os.ReadFile(base + "/stat")
	if err != nil {
		return "", "", "", err
	}
	tail := strings.LastIndex(string(data), ") ")
	if tail < 0 {
		return "", "", "", fmt.Errorf("invalid fixture proc stat")
	}
	fields := strings.Fields(string(data)[tail+2:])
	if len(fields) < 20 {
		return "", "", "", fmt.Errorf("short fixture proc stat")
	}
	bootData, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", "", "", err
	}
	hash, err = fileHash(base + "/exe")
	return strings.TrimSpace(string(bootData)), fields[19], hash, err
}

func fixtureStillLive(value fixtureIdentity, pidfd int) error {
	if !validFixture(value) {
		return fmt.Errorf("invalid fixture")
	}
	polls := []unix.PollFd{{Fd: int32(pidfd), Events: unix.POLLIN}}
	count, err := unix.Poll(polls, 0)
	if err != nil || count != 0 {
		return fmt.Errorf("fixture no longer live")
	}
	boot, start, hash, err := fixtureProcessIdentity(value.PID)
	if err != nil || boot != value.Boot || start != value.Start || hash != value.ExecutableSHA {
		return fmt.Errorf("fixture generation changed")
	}
	count, err = unix.Poll(polls, 0)
	if err != nil || count != 0 {
		return fmt.Errorf("fixture lost during identity check")
	}
	return nil
}

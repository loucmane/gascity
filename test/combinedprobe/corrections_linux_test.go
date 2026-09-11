package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriterTerminalEvidenceRequiresActualWaitAndPIDFD(t *testing.T) {
	for _, scenario := range []struct {
		pid, waitedPID   int
		waited, terminal bool
		want             bool
	}{
		{42, 42, true, true, true},
		{42, 0, false, true, false},
		{42, 43, true, true, false},
		{42, 42, true, false, false},
		{0, 0, true, true, false},
	} {
		if got := writerTerminalEvidence(scenario.pid, scenario.waitedPID, scenario.waited, scenario.terminal); got != scenario.want {
			t.Fatal(scenario, got)
		}
	}
	notStarted := outcome{WriterState: "writer_not_started", WriterExit: -1}
	if err := validateWriterOutcome(notStarted, false); err != nil {
		t.Fatal(err)
	}
	notStarted.WriterReaped = true
	if validateWriterOutcome(notStarted, false) == nil {
		t.Fatal("unstarted writer called reaped")
	}
	terminal := outcome{WriterState: "writer_terminal", WriterPID: 42, WriterWaitPID: 42, WriterWaitObserved: true, WriterPIDFDTerminal: true, WriterReaped: true, WriterExit: 1}
	if err := validateWriterOutcome(terminal, true); err != nil {
		t.Fatal(err)
	}
	terminal.WriterPIDFDTerminal = false
	if validateWriterOutcome(terminal, true) == nil {
		t.Fatal("missing terminal witness accepted")
	}
}

func completeR2Observation() observation {
	fixture := controlFixture()
	fixture.PID = 1234567
	fixture.Boot = "386902d4-a7d2-4056-a6c3-aa592cffe444"
	fixture.Start = "1234567890"
	fixture.ExecutableSHA = strings.Repeat("a", 64)
	fixture.Namespaces = hostSpaces
	value := observation{Kind: "observation-only-requires-outer-exit-zero", Plan: strings.Repeat("b", 64), Status: "PASS", Fixture: fixture, FixtureReaped: true, ProcessControls: boundAttempts(fixture, true), Cases: map[string]outcome{}}
	for _, name := range cases {
		result := outcome{Status: "PASS", LastStep: "final", Committed: true, Pair: true, CacheUnchanged: true, SupervisorReaped: true, WriterState: "writer_terminal", WriterPID: 1234568, WriterWaitPID: 1234568, WriterWaitObserved: true, WriterPIDFDTerminal: true, WriterReaped: true}
		result.ACLControls = map[string]attempt{"access": {OK: true}, "descendant_default": {OK: true}}
		result.Version = versionEvidence{Executable: "/proc/1234569/exe", Expected: expectedVersions[name], Observed: version + "\n", Process: identity{PID: 1234569, Boot: fixture.Boot, Start: fixture.Start, Exe: binaryPath, Hash: fixture.ExecutableSHA, Instance: strings.Repeat("c", 64), Port: 12345, Inode: "1234567890", Version: version}}
		result.Boundary = boundaryReport{Mutations: map[string]attempt{}, Attacks: boundAttempts(fixture, false), AttacksCompletedAt: 100, Namespaces: map[string]string{}, FDs: map[string]string{"0": "pipe:[123]", "1": "pipe:[124]", "2": "pipe:[125]"}, ReadLock: true, VerifierHidden: true}
		result.Boundary.ACL = map[string]attempt{"access": {Errno: 30}, "descendant_default": {Errno: 30}}
		for key, space := range hostSpaces {
			result.Boundary.Namespaces[key] = space + "-child"
		}
		result.Boundary.Namespaces["time"] = fixture.Namespaces["time"]
		for _, operation := range []string{"create", "overwrite", "truncate", "unlink", "rename", "mkdir", "symlink", "chmod", "utime", "descendant_thread", "descendant_process", "chown", "xattr"} {
			result.Boundary.Mutations[operation] = attempt{Errno: 30, FailureKind: "syscall", Error: "read-only file system"}
		}
		result.PostControls = boundAttempts(fixture, true)
		result.PostControlsStartedAt = 101
		result.PostControlsCompletedAt = 102
		switch name {
		case "version-mismatch", "unsupported":
			result.WriterState = "writer_not_started"
			result.WriterPID = 0
			result.WriterWaitPID = 0
			result.WriterWaitObserved = false
			result.WriterPIDFDTerminal = false
			result.WriterReaped = false
			result.WriterExit = -1
			result.Committed = false
			result.Pair = false
			result.Status = "aborted_preserved"
			result.Boundary = boundaryReport{}
			result.PostControls = nil
			result.PostControlsStartedAt = 0
			result.PostControlsCompletedAt = 0
			if name == "version-mismatch" {
				result.LastStep = "version"
				result.Error = compareVersion(result.Version.Expected, []byte(result.Version.Observed), nil).Error()
			} else {
				result.LastStep = "lease-begin"
				result.APIRefusal = &apiStatusError{Path: "lease-begin", Status: 404, Detail: "404 page not found\n"}
				result.Error = result.APIRefusal.Error()
			}
		case "before-loss", "after-loss":
			result.VerifierExit = 1
			result.WriterExit = 1
			result.Error = "API lease/instance refusal: lease missing, stale or expired"
			if name == "before-loss" {
				result.Committed = false
				result.Pair = false
				result.Status = "aborted_preserved"
				result.LastStep = "receipt-ready"
			} else {
				result.Status = "committed_evidence_incomplete"
				result.LastStep = "published"
			}
		}
		acceptanceFixture(name, &result)
		value.Cases[name] = result
	}
	return value
}

func TestCompleteR2ObservationAndFaultRefusal(t *testing.T) {
	value := completeR2Observation()
	if err := validateObservation(value, value.Plan); err != nil {
		t.Fatal(err)
	}
	data, err := encodeObservation(value)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeObservation(data)
	if err != nil {
		t.Fatalf("complete observation bytes=%d: %v", len(data), err)
	}
	if err := validateObservation(decoded, value.Plan); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeObservation(append(append([]byte{}, data...), 0)); err == nil {
		t.Fatal("trailing result data accepted")
	}
	if _, err := decodeObservation(data[:len(data)-1]); err == nil {
		t.Fatal("truncated result accepted")
	}
	for _, scenario := range []string{"unstarted-reaped", "missing-pidfd", "wrong-version", "wrong-status", "wrong-target", "stale-control"} {
		changed := completeR2Observation()
		name := "success"
		if scenario == "unstarted-reaped" || scenario == "wrong-status" {
			name = "unsupported"
		}
		result := changed.Cases[name]
		switch scenario {
		case "unstarted-reaped":
			result.WriterReaped = true
		case "missing-pidfd":
			result.WriterPIDFDTerminal = false
		case "wrong-version":
			result.Version.Expected = version + "wrong"
		case "wrong-status":
			result.APIRefusal.Status = 500
		case "wrong-target":
			result.Boundary.Attacks["ptrace"].Target.PID++
		case "stale-control":
			result.PostControlsCompletedAt = controlFreshnessNS + 101
		}
		changed.Cases[name] = result
		if validateObservation(changed, changed.Plan) == nil {
			t.Fatal(scenario)
		}
	}
}

func TestVersionExpectationIsIndependent(t *testing.T) {
	if err := compareVersion("expected", []byte("expected\n"), nil); err != nil {
		t.Fatal(err)
	}
	var mismatch *versionMismatch
	if err := compareVersion("different", []byte("expected\n"), nil); !errors.As(err, &mismatch) {
		t.Fatal(err)
	}
	if err := compareVersion("expected", []byte("expected\n"), errors.New("exit1")); err == nil || errors.As(err, &mismatch) {
		t.Fatal("failed command treated as mismatch", err)
	}
	if observedVersionArgv(123)[0] != "/proc/123/exe" || observedVersionArgv(123)[1] != "version" {
		t.Fatal("not literal observed image")
	}
}

func TestAbsentLeaseRouteIsAttributed404(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(output http.ResponseWriter, _ *http.Request) { called = true; output.WriteHeader(200) })
	mux := supervisorMux("unsupported", handler)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("POST", "http://127.0.0.1/lease-begin", nil))
	if called || response.Code != 404 || response.Body.String() != "404 page not found\n" {
		t.Fatal("lease handler was not absent", response)
	}
	_, err := decodeAPIResponse(identity{}, "lease-begin", leaseRequest{}, response.Code, response.Body.Bytes())
	var refusal *apiStatusError
	if !errors.As(err, &refusal) || !absentLeaseRefusal(refusal) {
		t.Fatal(err)
	}
	for _, changed := range []apiStatusError{{Path: "health", Status: 404, Detail: "404 page not found\n"}, {Path: "lease-begin", Status: 500, Detail: "404 page not found\n"}, {Path: "lease-begin", Status: 404, Detail: "different"}} {
		if absentLeaseRefusal(&changed) {
			t.Fatal(changed)
		}
	}
	_, err = decodeAPIResponse(identity{}, "lease-begin", leaseRequest{}, 200, []byte(`{"identity":{},"lease":{},"error":"unsupported lease"}`))
	if err == nil || errors.As(err, &refusal) {
		t.Fatal("200 application error accepted as absent handler", err)
	}
}

func controlFixture() fixtureIdentity {
	return fixtureIdentity{PID: 42, FD: 7, Address: 4096, Boot: "boot", Start: "123", ExecutableSHA: "hash", Marker: fixtureMarker, Memory: fixtureMemory, Namespaces: map[string]string{"pid": "host"}}
}

func boundAttempts(fixture fixtureIdentity, positive bool) map[string]attempt {
	result := map[string]attempt{}
	for _, name := range []string{"signal", "memory", "fd", "ptrace"} {
		target := targetForFixture(fixture)
		value := attempt{OK: positive, Target: &target}
		if !positive {
			value.Errno = 3
			value.FailureKind = "attach_denied"
		}
		result[name] = value
	}
	return result
}

func TestAttackTargetsAndRefreshedControlsMustBind(t *testing.T) {
	fixture := controlFixture()
	for _, positive := range []bool{false, true} {
		values := boundAttempts(fixture, positive)
		if err := validateProcessAttempts(values, fixture, positive); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"pid", "fd", "address", "boot", "start", "image", "marker", "memory"} {
			changed := boundAttempts(fixture, positive)
			target := changed["ptrace"].Target
			switch field {
			case "pid":
				target.PID++
			case "fd":
				target.FD++
			case "address":
				target.Address++
			case "boot":
				target.Boot = "other"
			case "start":
				target.Start = "other"
			case "image":
				target.ExecutableSHA = "other"
			case "marker":
				target.MarkerSHA = "other"
			case "memory":
				target.MemorySHA = "other"
			}
			if validateProcessAttempts(changed, fixture, positive) == nil {
				t.Fatal(field, positive)
			}
		}
	}
	result := outcome{Boundary: boundaryReport{AttacksCompletedAt: 100}, PostControls: boundAttempts(fixture, true), PostControlsStartedAt: 101, PostControlsCompletedAt: 102}
	if err := validateControlRefresh(result, fixture); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"before", "stale", "failed", "missing"} {
		changed := result
		switch scenario {
		case "before":
			changed.PostControlsStartedAt = 99
		case "stale":
			changed.PostControlsCompletedAt = 100 + controlFreshnessNS + 1
		case "failed":
			changed.PostControls = boundAttempts(fixture, false)
		case "missing":
			changed.PostControls = nil
		}
		if validateControlRefresh(changed, fixture) == nil {
			t.Fatal(scenario)
		}
	}
}

//go:build linux

package main

import (
	"encoding/binary"
	"strings"
	"testing"
)

func acceptanceFixture(name string, result *outcome) {
	if name == "concurrent" {
		result.Acceptance = acceptanceEvidence{OldReaders: 4, MixedReaders: 4, NewReaders: 4, CrossingRefused: true, CompetingRefusal: "lease occupied; renewal refused"}
		return
	}
	phase := faultPhase(name)
	if phase == "" && name != "same-build-impostor" && name != "wrong-image" {
		return
	}
	action := strings.TrimSuffix(strings.TrimSuffix(name, "-before"), "-after")
	proof := acceptanceEvidence{Phase: phase, Action: action, ObservedAt: 200, Deadline: 199, Original: result.Version.Process, Replacement: result.Version.Process, Refusal: "fixture expected refusal", Terminal: true, ReplacementReaped: true}
	proof.Replacement.Instance = "new-instance"
	if action != "exec" {
		proof.Replacement.PID++
	}
	result.Committed = phase == "published"
	result.Pair = result.Committed
	result.Status = "aborted_preserved"
	if result.Committed {
		result.Status = "committed_evidence_incomplete"
	}
	result.Error = proof.Refusal
	if action == "eof" {
		result.Error += ": EOF"
	}
	result.LastStep = phase
	result.VerifierExit = 1
	result.WriterExit = 1
	if action == "crash" {
		result.WriterExit = 23
	}
	if action == "fsync" {
		proof.FaultErrno = 9
	}
	if name == "same-build-impostor" || name == "wrong-image" {
		proof.Phase = "identity"
		result.LastStep = "identity"
		if name == "wrong-image" {
			proof.Replacement.Hash = strings.Repeat("f", 64)
			proof.Replacement.Exe = impostorPath
		}
		proof.Replacement.Port++
		proof.Replacement.Inode = "1234567891"
		inspected := proof.Original
		inspected.Port = proof.Replacement.Port
		inspected.Inode = proof.Replacement.Inode
		proof.Mismatch = &identityMismatch{Kind: "listener-ownership", Inspected: inspected, ExpectedImage: binaryPath, OwnedSockets: []string{proof.Original.Inode}}
		if name == "wrong-image" {
			inspected = proof.Replacement
			inspected.Inode = ""
			proof.Mismatch = &identityMismatch{Kind: "executable-image", Inspected: inspected, ExpectedImage: binaryPath}
		}
		proof.Refusal = proof.Mismatch.Error()
		result.Error = proof.Refusal
		result.WriterState = "writer_not_started"
		result.WriterPID = 0
		result.WriterWaitPID = 0
		result.WriterWaitObserved = false
		result.WriterPIDFDTerminal = false
		result.WriterReaped = false
		result.WriterExit = -1
		result.VerifierExit = 0
	}
	result.Acceptance = proof
}

func TestAcceptanceFaultClassification(t *testing.T) {
	for _, name := range []string{"exit-before", "exit-after", "exec-before", "exec-after", "expiry-before", "expiry-after", "crash-before", "crash-after", "eof-before", "eof-after", "fsync-before", "fsync-after"} {
		if !knownCase(name) {
			t.Errorf("missing bounded acceptance case %s", name)
		}
	}
}

func TestAcceptanceWitnessesFailClosed(t *testing.T) {
	for _, name := range []string{"exit-before", "exec-after", "restart-before", "wrong-image", "same-build-impostor", "expiry-after", "crash-before", "fsync-after", "concurrent"} {
		original := completeR2Observation().Cases[name]
		if err := validateAcceptance(name, original); err != nil {
			t.Fatal(name, err)
		}
		changed := original
		changed.Acceptance = acceptanceEvidence{}
		if validateAcceptance(name, changed) == nil {
			t.Fatal("missing witness accepted", name)
		}
		changed = original
		changed.Committed = !original.Committed
		if name != "concurrent" && validateAcceptance(name, changed) == nil {
			t.Fatal("wrong commit accepted", name)
		}
	}
	value := completeR2Observation().Cases["exec-before"]
	value.Acceptance.Replacement.Instance = value.Acceptance.Original.Instance
	if validateAcceptance("exec-before", value) == nil {
		t.Fatal("epoch injection without exec-generation witness accepted")
	}
}

func TestIdentityNegativeRejectsArbitraryRefusal(t *testing.T) {
	for _, name := range []string{"same-build-impostor", "wrong-image"} {
		value := completeR2Observation().Cases[name]
		value.Acceptance.Refusal = "unrelated proc read failure"
		value.Error = value.Acceptance.Refusal
		if validateAcceptance(name, value) == nil {
			t.Fatalf("%s accepted arbitrary refusal", name)
		}
	}
}

func TestACLPositiveAndDenialAreDifferent(t *testing.T) {
	data := fixtureACL(4)
	if len(data) != 44 || binary.LittleEndian.Uint32(data) != 2 || binary.LittleEndian.Uint32(data[16:]) != 65534 {
		t.Fatal("invalid fixture ACL")
	}
	for _, errno := range []int{0, 2, 22, 95} {
		values := map[string]attempt{"access": {Errno: errno}, "descendant_default": {Errno: 30}}
		if validateACL(values, false) == nil {
			t.Fatal("unsupported or invalid ACL called denial", errno)
		}
	}
	positive := map[string]attempt{"access": {OK: true}, "descendant_default": {OK: true}}
	if err := validateACL(positive, true); err != nil {
		t.Fatal(err)
	}
	if validateACL(positive, false) == nil {
		t.Fatal("writable ACL accepted as confinement")
	}
}

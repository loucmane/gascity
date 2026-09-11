package main

import (
	"encoding/json"
	"errors"
	"strings"
	"syscall"
	"testing"
)

func TestNonErrnoAndBoundedOutputRemainRefusals(t *testing.T) {
	plain := observe(func() error { return errors.New("fixture mismatch") })
	if plain.OK || plain.Errno != 0 || plain.FailureKind != "non_errno" {
		t.Fatalf("%+v", plain)
	}
	denial := observe(func() error { return syscall.EROFS })
	if denial.OK || denial.Errno != 30 || denial.FailureKind != "syscall" {
		t.Fatalf("%+v", denial)
	}
	var output limitedBuffer
	if _, err := output.Write(make([]byte, 65536)); err != nil {
		t.Fatal(err)
	}
	if count, err := output.Write([]byte("overflow")); err == nil || count != 0 || output.Len() != 65536 {
		t.Fatal("overflow accepted")
	}
}

func TestObservationRequiresOuterExitAndCompleteWitnesses(t *testing.T) {
	hash := strings.Repeat("a", 64)
	complete := func() runReport {
		result := runReport{EvidenceKind: "observation-only-requires-outer-exit-zero", Status: "PASS", PlanSHA256: hash, ZeroProcessResidue: true, FixtureReaped: true, FixturePIDFDTerminal: true, ConfinedPID1Completed: true, Before: map[string]fileImage{".": {}}, After: map[string]fileImage{".": {}}, Control: map[string]attempt{}, ProcessControl: map[string]attempt{}}
		result.Fixture = fixtureIdentity{PID: 42, PtracerPolicy: "fixture-only-any-same-uid", Namespaces: map[string]string{}}
		result.Child = childReport{Metadata: true, ReadLock: true, Namespaces: map[string]string{}, Mutations: map[string]attempt{}, Attacks: map[string]attempt{}, FDs: map[string]string{"0": "pipe:1", "1": "pipe:2", "2": "pipe:3"}}
		for _, name := range []string{"user", "pid", "mnt", "net", "ipc", "time"} {
			result.Fixture.Namespaces[name] = "host-" + name
			result.Child.Namespaces[name] = "child-" + name
		}
		result.Child.Namespaces["time"] = "host-time"
		for _, name := range mutationNames {
			result.Control[name] = attempt{OK: true}
			result.Child.Mutations[name] = attempt{Errno: 30}
		}
		for _, name := range []string{"signal", "memory", "fd", "ptrace"} {
			result.ProcessControl[name] = attempt{OK: true}
			result.Child.Attacks[name] = attempt{Errno: 3}
		}
		result.Child.Attacks["ptrace"] = attempt{Errno: 3, FailureKind: "attach_denied"}
		return result
	}
	for _, scenario := range []string{"complete", "exit-failure", "missing-witness", "wrong-hash", "missing-cache", "unexpected-fd", "errno-zero", "truncated", "ptrace-post-attach", "ptrace-no-provenance"} {
		t.Run(scenario, func(t *testing.T) {
			value := complete()
			exitCode := 0
			switch scenario {
			case "ptrace-post-attach":
				value.Child.Attacks["ptrace"] = attempt{Errno: 3, FailureKind: "post_attach"}
			case "ptrace-no-provenance":
				value.Child.Attacks["ptrace"] = attempt{Errno: 3}
			case "exit-failure":
				exitCode = 1
			case "missing-witness":
				value.FixturePIDFDTerminal = false
			case "wrong-hash":
				value.PlanSHA256 = "wrong"
			case "missing-cache":
				value.Before = nil
			case "unexpected-fd":
				value.Child.FDs["8"] = "anon_inode:[timerfd]"
			case "errno-zero":
				value.Child.Mutations[mutationNames[0]] = attempt{Error: "not errno", FailureKind: "non_errno"}
			}
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "truncated" {
				data = data[:len(data)/2]
			}
			err = validateObservation(data, hash, exitCode)
			if scenario == "complete" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("invalid outer acceptance")
			}
		})
	}
}

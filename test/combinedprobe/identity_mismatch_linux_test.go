package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

type masqueradingMismatch struct {
	value *identityMismatch
}

func (masqueradingMismatch) Error() string { return "unrelated failure with custom As" }
func (failure masqueradingMismatch) As(target any) bool {
	if pointer, ok := target.(**identityMismatch); ok {
		*pointer = failure.value
		return true
	}
	return false
}

func identityInspectionFixture(name string) (identity, identity, identityReads) {
	original := identity{PID: 41, Boot: "fixture-boot", Start: "100", Exe: binaryPath, Hash: digest([]byte("genuine")), Instance: "original", Port: 1234, Inode: "101", Version: version}
	replacement := identity{PID: 42, Boot: original.Boot, Start: "200", Exe: binaryPath, Hash: original.Hash, Instance: "replacement", Port: 1235, Inode: "202", Version: version}
	target := original
	image := "genuine"
	if name == "wrong-image" {
		replacement.Exe = impostorPath
		replacement.Hash = digest([]byte("impostor"))
		target = replacement
		image = "impostor"
	}
	fields := make([]string, 20)
	for index := range fields {
		fields[index] = "0"
	}
	fields[19] = target.Start
	base := fmt.Sprintf("/proc/%d", target.PID)
	reads := identityReads{
		readFile: func(path string) ([]byte, error) {
			switch path {
			case base + "/stat":
				return []byte(fmt.Sprintf("%d (fixture) %s", target.PID, strings.Join(fields, " "))), nil
			case "/proc/sys/kernel/random/boot_id":
				return []byte(target.Boot), nil
			case base + "/exe":
				return []byte(image), nil
			case base + "/net/tcp":
				return []byte(fmt.Sprintf("sl local_address remote_address st tx_queue rx tr tm retr uid\n0: 0100007F:%04X 00000000:0000 0A 0 0 0 0 0 202\n", replacement.Port)), nil
			}
			return nil, fmt.Errorf("unexpected fixture read %s", path)
		},
		readlink: func(path string) (string, error) {
			if path == base+"/exe" {
				return target.Exe, nil
			}
			if path == base+"/fd/3" {
				return "socket:[101]", nil
			}
			return "", fmt.Errorf("unexpected fixture link %s", path)
		},
		fdNames: func(path string) ([]string, error) {
			if path != base+"/fd" {
				return nil, fmt.Errorf("unexpected fixture directory")
			}
			return []string{"3"}, nil
		},
	}
	return original, replacement, reads
}

func inspectNegativeFixture(name string, original, replacement identity, reads identityReads) error {
	target := original
	if name == "wrong-image" {
		target = replacement
	}
	_, err := inspectIdentity(target.PID, replacement.Port, target.Instance, binaryPath, reads)
	return err
}

func TestIdentityInspectionAcceptsOnlyObservedMismatch(t *testing.T) {
	for _, name := range []string{"same-build-impostor", "wrong-image"} {
		original, replacement, reads := identityInspectionFixture(name)
		refusal := inspectNegativeFixture(name, original, replacement, reads)
		mismatch, err := classifyIdentityMismatch(context.Background(), name, original, replacement, refusal)
		if err != nil || mismatch == nil {
			t.Fatal(name, refusal, err)
		}
		for _, unrelated := range []error{nil, (*identityMismatch)(nil), os.ErrNotExist, io.ErrUnexpectedEOF, context.Canceled, context.DeadlineExceeded, errors.New("API decode failure"), errors.Join(refusal, io.ErrUnexpectedEOF), fmt.Errorf("unrelated refusal: %w", refusal), masqueradingMismatch{value: mismatch}} {
			if _, err := classifyIdentityMismatch(context.Background(), name, original, replacement, unrelated); err == nil {
				t.Fatalf("%s accepted %v", name, unrelated)
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := classifyIdentityMismatch(ctx, name, original, replacement, refusal); err == nil {
			t.Fatal("cancellation accepted")
		}
	}
}

func TestIdentityInspectionPositiveStillRequiresOwnedListener(t *testing.T) {
	original, _, reads := identityInspectionFixture("same-build-impostor")
	readFile := reads.readFile
	reads.readFile = func(path string) ([]byte, error) {
		if strings.HasSuffix(path, "/net/tcp") {
			return []byte(fmt.Sprintf("0: 0100007F:%04X 00000000:0000 0A 0 0 0 0 0 101\n", original.Port)), nil
		}
		return readFile(path)
	}
	actual, err := inspectIdentity(original.PID, original.Port, original.Instance, binaryPath, reads)
	if err != nil || actual != original {
		t.Fatal(actual, err)
	}
	reads.fdNames = func(string) ([]string, error) { return nil, os.ErrNotExist }
	if _, err := inspectIdentity(original.PID, original.Port, original.Instance, binaryPath, reads); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("positive FD inventory error hidden", err)
	}
}

func TestIdentityInspectionIOFailuresNeverBecomeNegativePass(t *testing.T) {
	for _, name := range []string{"same-build-impostor", "wrong-image"} {
		for _, operation := range []string{"stat", "boot", "image", "readlink"} {
			original, replacement, reads := identityInspectionFixture(name)
			readFile, readlink := reads.readFile, reads.readlink
			reads.readFile = func(path string) ([]byte, error) {
				if (operation == "stat" && strings.HasSuffix(path, "/stat")) || (operation == "boot" && strings.HasSuffix(path, "/boot_id")) || (operation == "image" && strings.HasSuffix(path, "/exe")) {
					return nil, os.ErrNotExist
				}
				return readFile(path)
			}
			reads.readlink = func(path string) (string, error) {
				if operation == "readlink" {
					return impostorPath, io.ErrUnexpectedEOF
				}
				return readlink(path)
			}
			refusal := inspectNegativeFixture(name, original, replacement, reads)
			identityMismatch := &identityMismatch{}
			if errors.As(refusal, &identityMismatch) {
				t.Fatal("I/O failure conflated with mismatch", name, operation)
			}
			if _, err := classifyIdentityMismatch(context.Background(), name, original, replacement, refusal); err == nil {
				t.Fatal(name, operation)
			}
		}
	}
	for _, operation := range []string{"enumeration", "fd-readlink", "table", "truncated-table", "missing-listener", "empty-inventory"} {
		original, replacement, reads := identityInspectionFixture("same-build-impostor")
		readFile, readlink := reads.readFile, reads.readlink
		if operation == "enumeration" {
			reads.fdNames = func(string) ([]string, error) { return nil, io.ErrUnexpectedEOF }
		}
		if operation == "empty-inventory" {
			reads.fdNames = func(string) ([]string, error) { return nil, nil }
		}
		reads.readlink = func(path string) (string, error) {
			if operation == "fd-readlink" && strings.Contains(path, "/fd/") {
				return "", os.ErrNotExist
			}
			return readlink(path)
		}
		reads.readFile = func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "/net/tcp") {
				switch operation {
				case "table":
					return nil, io.ErrUnexpectedEOF
				case "truncated-table":
					return []byte("0: broken"), nil
				case "missing-listener":
					return []byte(""), nil
				}
			}
			return readFile(path)
		}
		refusal := inspectNegativeFixture("same-build-impostor", original, replacement, reads)
		if _, err := classifyIdentityMismatch(context.Background(), "same-build-impostor", original, replacement, refusal); err == nil {
			t.Fatal(operation)
		}
	}
}

func TestIdentityMismatchObservationBinding(t *testing.T) {
	for _, name := range []string{"same-build-impostor", "wrong-image"} {
		for _, field := range []string{"kind", "pid", "image", "hash", "port", "inode", "expected", "missing", "refusal"} {
			value := completeR2Observation().Cases[name]
			proof := *value.Acceptance.Mismatch
			value.Acceptance.Mismatch = &proof
			switch field {
			case "kind":
				proof.Kind = "proc-read-error"
			case "pid":
				proof.Inspected.PID++
			case "image":
				proof.Inspected.Exe = "/missing"
			case "hash":
				proof.Inspected.Hash = "unrelated"
			case "port":
				proof.Inspected.Port++
			case "inode":
				proof.Inspected.Inode = "unrelated"
			case "expected":
				proof.ExpectedImage = "/unrelated"
			case "missing":
				value.Acceptance.Mismatch = nil
			case "refusal":
				value.Acceptance.Refusal = "API timeout"
				value.Error = value.Acceptance.Refusal
			}
			if field != "refusal" {
				value.Acceptance.Refusal = proof.Error()
				value.Error = value.Acceptance.Refusal
			}
			if validateAcceptance(name, value) == nil {
				t.Fatal(name, field)
			}
		}
	}
}

package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
)

type identityReads struct {
	readFile func(string) ([]byte, error)
	readlink func(string) (string, error)
	fdNames  func(string) ([]string, error)
}

type identityMismatch struct {
	Kind          string   `json:"kind"`
	Inspected     identity `json:"inspected"`
	ExpectedImage string   `json:"expected_image"`
	OwnedSockets  []string `json:"owned_sockets,omitempty"`
}

func (mismatch *identityMismatch) Error() string {
	return fmt.Sprintf("observed %s mismatch: pid=%d expected_image=%q actual_image=%q image_sha256=%s port=%d listener=%q owned=%q", mismatch.Kind, mismatch.Inspected.PID, mismatch.ExpectedImage, mismatch.Inspected.Exe, mismatch.Inspected.Hash, mismatch.Inspected.Port, mismatch.Inspected.Inode, mismatch.OwnedSockets)
}

func expectedIdentityMismatch(name string, original, replacement identity, mismatch *identityMismatch) error {
	if mismatch == nil || !completeInspectedIdentity(original) || !completeInspectedIdentity(replacement) || original.PID == replacement.PID || original.Exe != binaryPath || mismatch.ExpectedImage != binaryPath {
		return fmt.Errorf("identity mismatch evidence incomplete")
	}
	switch name {
	case "wrong-image":
		expected := replacement
		expected.Inode = ""
		if mismatch.Kind != "executable-image" || mismatch.Inspected != expected || replacement.Exe != impostorPath || replacement.Hash == original.Hash || len(mismatch.OwnedSockets) != 0 {
			return fmt.Errorf("wrong-image mismatch is not the inspected replacement")
		}
	case "same-build-impostor":
		expected := original
		expected.Port = replacement.Port
		expected.Inode = replacement.Inode
		if mismatch.Kind != "listener-ownership" || mismatch.Inspected != expected || replacement.Exe != binaryPath || replacement.Hash != original.Hash || replacement.Inode == "" || replacement.Inode == original.Inode || replacement.Port == original.Port || !sort.StringsAreSorted(mismatch.OwnedSockets) {
			return fmt.Errorf("listener mismatch is not the inspected target")
		}
		foundOriginal := false
		for index, inode := range mismatch.OwnedSockets {
			if !positiveIdentityNumber(inode) || inode == replacement.Inode || (index > 0 && inode == mismatch.OwnedSockets[index-1]) {
				return fmt.Errorf("incomplete/conflicting socket inventory")
			}
			foundOriginal = foundOriginal || inode == original.Inode
		}
		if !foundOriginal {
			return fmt.Errorf("original listener missing from inspected FD inventory")
		}
	default:
		return fmt.Errorf("unreviewed identity negative")
	}
	return nil
}

func positiveIdentityNumber(value string) bool {
	number, err := strconv.ParseUint(value, 10, 64)
	return err == nil && number > 0
}

func completeInspectedIdentity(value identity) bool {
	hash, err := hex.DecodeString(value.Hash)
	return value.PID > 1 && value.Boot != "" && positiveIdentityNumber(value.Start) && value.Exe != "" && err == nil && len(hash) == 32 && value.Instance != "" && value.Port > 0 && value.Port <= 65535 && positiveIdentityNumber(value.Inode) && value.Version == version
}

func classifyIdentityMismatch(ctx context.Context, name string, original, replacement identity, refusal error) (*identityMismatch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Only the direct observed mismatch is evidence. A joined/wrapped error
	// may carry an I/O failure, so errors.As alone would incorrectly accept it.
	var mismatch *identityMismatch
	if reflect.TypeOf(refusal) != reflect.TypeFor[*identityMismatch]() || !errors.As(refusal, &mismatch) || mismatch == nil {
		return nil, fmt.Errorf("identity negative lacks observed mismatch: %w", refusal)
	}
	if err := expectedIdentityMismatch(name, original, replacement, mismatch); err != nil {
		return nil, err
	}
	return mismatch, nil
}

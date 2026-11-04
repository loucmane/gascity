package verifiedexec

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExecutableIdentityRefusals(t *testing.T) {
	for _, fault := range []string{"digest", "mode", "resolved", "symlink", "directory"} {
		t.Run(fault, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cli")
			data := []byte("#!/bin/sh\necho approved\n")
			if err := os.WriteFile(path, data, 0o555); err != nil {
				t.Fatal(err)
			}
			resolved, digest := path, fmt.Sprintf("%x", sha256.Sum256(data))
			switch fault {
			case "digest":
				digest = fmt.Sprintf("%x", sha256.Sum256([]byte("different")))
			case "mode":
				if err := os.Chmod(path, 0o755); err != nil {
					t.Fatal(err)
				}
			case "resolved":
				resolved += ".other"
			case "symlink":
				if err := os.Rename(path, path+".target"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".target", path); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0o555); err != nil {
					t.Fatal(err)
				}
			}
			file, err := Open(path, resolved, digest, 0o555)
			if err == nil {
				_ = file.Close()
				t.Fatal("accepted identity mismatch")
			}
		})
	}
}

func TestDescriptorExecKeepsOpenedBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cli")
	data := []byte("#!/bin/sh\necho approved\n")
	if err := os.WriteFile(path, data, 0o555); err != nil {
		t.Fatal(err)
	}
	file, err := Open(path, path, fmt.Sprintf("%x", sha256.Sum256(data)), 0o555)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close() //nolint:errcheck // test read-only descriptor
	if err := os.WriteFile(path+".new", []byte("#!/bin/sh\necho replacement\n"), 0o555); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".new", path); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(context.Background(), path)
	Bind(cmd, file)
	data, err = cmd.Output()
	if err != nil || string(data) != "approved\n" {
		t.Fatalf("ran %q: %v", data, err)
	}
}

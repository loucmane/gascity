package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/gastownhall/gascity/internal/managedworker"
)

func TestProbeManagedWorkerToolchainIgnoresSuccessfulVersionStderr(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "go")
	executableBytes := []byte("#!/bin/sh\nprintf 'toolchain warning\\n' >&2\nprintf 'go version go1.26.7 linux/amd64\\n'\n")
	if err := os.WriteFile(executable, executableBytes, 0o755); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(executableBytes)
	pin := managedworker.ToolchainPin{
		Name: "go",
		Executable: managedworker.ExecutablePin{
			Path:         executable,
			ResolvedPath: executable,
			SHA256:       hex.EncodeToString(digest[:]),
			VersionArgs:  []string{"version"},
			Version:      "go version go1.26.7 linux/amd64",
		},
	}

	if err := probeManagedWorkerToolchain(context.Background(), pin, map[string]string{"PATH": dir}); err != nil {
		t.Fatalf("probeManagedWorkerToolchain() error = %v", err)
	}
}

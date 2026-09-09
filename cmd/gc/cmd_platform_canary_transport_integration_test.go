//go:build integration

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/managedworker"
)

// This boundary owns real argv/environment/CWD transport and response decoding,
// not profile selection or provider permission acceptance. The runner is one
// shell using builtins only; it never invokes gc, a provider, a signer or a
// service. All captured bytes belong to this test's temporary directory.
func TestPlatformCanaryRunnerRoundTripsSelectedProfile(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source with spaces")
	scratch := filepath.Join(root, "capture with spaces")
	for _, path := range []string{source, scratch} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	const runner = `#!/bin/sh
set -eu
printf '%s\000' "$@" > "${12}/argv"
printf '%s' "$PWD" > "${12}/cwd"
printf '%s' "$GCT_CANARY_WORKER_PROFILE_JSON" > "${12}/profile.json"
printf '%s\n' '{"name":"candidate-launcher","resolution":"completed"}'
`
	runnerPath := filepath.Join(root, "capture-runner")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o700); err != nil {
		t.Fatal(err)
	}
	receipt, _, _ := dispatchGateFixture(t)
	profile := receipt.Profiles[0]
	profile.Name = "assembly/custom.builder"
	profile.ProfileKind = managedworker.ProfileKindCandidate
	profile.SignerIdentity = managedworker.NoSignerIdentity
	profile.ControlPolicy = &managedworker.FilePin{
		Path: "/policies/candidate.json", SHA256: digestBytes([]byte("synthetic policy")),
	}
	profile.Argv = []string{profile.Provider.Path, "--settings", profile.ControlPolicy.Path}
	profile.Environment["SYNTHETIC_LITERAL"] = "LÄS & ELF; $(not-a-command)"
	profile.WorkerProfileSHA256 = ""
	digest, err := managedworker.WorkerProfileDigest(profile)
	if err != nil {
		t.Fatal(err)
	}
	profile.WorkerProfileSHA256 = digest
	call := platformCanaryScenarioCall{
		BaseCommit: strings.Repeat("a", 40), CityPath: filepath.Join(root, "unused-city"),
		LauncherSource: source, RunID: "transport-only", RunnerPath: runnerPath,
		RunnerSHA256: digestBytes([]byte(runner)), Scenario: managedworker.CanaryScenarioCandidateLauncher,
		ScratchRoot: scratch, WorkerProfile: profile,
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	got, err := runPlatformCanaryScenarioCommand(ctx, call)
	if err != nil {
		t.Fatal(err)
	}
	want := managedworker.CanaryScenarioEvidence{
		Name: call.Scenario, Resolution: managedworker.CanaryResolutionCompleted,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runner response changed: got=%+v want=%+v", got, want)
	}
	read := func(name string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(scratch, name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	wantArgs := []string{
		"--scenario", call.Scenario, "--run-id", call.RunID,
		"--target-city", call.CityPath, "--launcher-source", source,
		"--base-commit", call.BaseCommit, "--scratch-root", scratch,
	}
	if got := string(read("argv")); got != strings.Join(wantArgs, "\x00")+"\x00" {
		t.Fatalf("argv boundaries changed: got=%q want=%q", got, wantArgs)
	}
	if got := string(read("cwd")); got != source {
		t.Fatalf("runner CWD=%q want=%q", got, source)
	}
	var observed managedworker.WorkerProfile
	wire := read("profile.json")
	if err := json.Unmarshal(wire, &observed); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(observed, profile) {
		t.Fatalf("selected profile changed at exec boundary: got=%+v want=%+v", observed, profile)
	}
	if got, err := managedworker.WorkerProfileDigest(observed); err != nil || got != digest {
		t.Fatalf("transport changed digest domain: got=%q want=%q err=%v", got, digest, err)
	}
	// Synthetic wire output also supports separately recorded consumer diagnosis;
	// it contains no real project data, credentials, or live receipt authority.
	t.Logf("captured_profile=%s", wire)
}

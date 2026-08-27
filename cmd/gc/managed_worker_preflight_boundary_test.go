package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/clock"
	"github.com/gastownhall/gascity/internal/events"
	"github.com/gastownhall/gascity/internal/managedworker"
	"github.com/gastownhall/gascity/internal/platforminstall"
	"github.com/gastownhall/gascity/internal/runtime"
	sessionpkg "github.com/gastownhall/gascity/internal/session"
	"github.com/gastownhall/gascity/internal/shellquote"
)

func TestStartPreparedStartCandidateRefusesProviderStartWhenManagedPreflightFails(t *testing.T) {
	sp := runtime.NewFake()
	item := preparedStart{
		candidate: startCandidate{
			info: sessionpkg.Info{
				SessionName:         "managed-worker",
				SessionNameMetadata: "managed-worker",
			},
			tp: TemplateParams{TemplateName: "worker"},
		},
		cfg: runtime.Config{
			Command: "codex --ask-for-approval never --sandbox workspace-write",
			WorkDir: t.TempDir(),
		},
		preflight: func(context.Context) error {
			return errors.New("rules sha256 mismatch")
		},
	}

	started, err := startPreparedStartCandidate(context.Background(), item, t.TempDir(), nil, sp, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "managed-worker preflight") || !strings.Contains(err.Error(), "rules sha256 mismatch") {
		t.Fatalf("start error = %v", err)
	}
	if started {
		t.Fatal("started = true, want false when preflight rejects")
	}
	for _, call := range sp.Calls {
		if call.Method == "Start" {
			t.Fatalf("runtime calls = %#v, provider Start must not run after failed preflight", sp.Calls)
		}
	}
}

func TestConfigureManagedWorkerPreflightUsesProductionReceiptAndRoutedWork(t *testing.T) {
	cityPath := t.TempDir()
	rulesPath := filepath.Join(cityPath, ".gc", "platform", "rules")
	checkPath := filepath.Join(cityPath, ".gc", "platform", "check")
	for path, data := range map[string][]byte{rulesPath: []byte("rules"), checkPath: []byte("check")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o555); err != nil {
			t.Fatal(err)
		}
	}
	profileName := "sample/custom.builder"
	argv := []string{"/managed/codex", "--ask-for-approval", "never", "--sandbox", "workspace-write"}
	profile := managedworker.WorkerProfile{
		Name: profileName,
		Provider: platforminstall.ProviderPin{
			Name: "custom-provider", Path: "/managed/provider", ResolvedPath: "/managed/releases/provider",
			SHA256: preflightDigest("provider"), VersionArgs: []string{"--version"}, Version: "codex-cli 1",
		},
		CheckPath:      managedworker.FilePin{Path: checkPath, SHA256: preflightDigest("check")},
		SignerIdentity: "SIGNER",
		Argv:           argv,
		WritableRoots:  []string{"/managed/worktrees"},
		ApprovalPolicy: "never",
		SandboxMode:    "workspace-write",
		NetworkPolicy:  "restricted",
	}
	receipt, encoded, err := managedworker.FinalizeProvisioningReceipt(managedworker.ProvisioningReceipt{
		Schema:             managedworker.ProvisioningReceiptSchemaV1,
		CanaryRunner:       managedworker.FilePin{Path: "/city/bin/managed-worker-canary", SHA256: preflightDigest("canary-runner")},
		MemberHeads:        []managedworker.MemberHead{{Name: "gct-xnf", Commit: strings.Repeat("a", 40)}},
		TemplateCommit:     strings.Repeat("b", 40),
		Pack:               managedworker.PackPin{Source: "pack", Commit: strings.Repeat("c", 40), SHA256: preflightDigest("pack")},
		Rules:              managedworker.FilePin{Path: rulesPath, SHA256: preflightDigest("rules")},
		PermissionRevision: preflightDigest("permission"),
		Profiles:           []managedworker.WorkerProfile{profile},
	})
	if err != nil {
		t.Fatalf("FinalizeProvisioningReceipt: %v", err)
	}
	receiptPath := managedworker.ProvisioningReceiptPath(cityPath)
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(receiptPath, encoded, 0o444); err != nil {
		t.Fatal(err)
	}

	item := preparedStart{
		candidate:         startCandidate{tp: TemplateParams{TemplateName: "custom.builder", RigName: "sample"}},
		cfg:               runtime.Config{Command: shellquote.Join(append(append([]string(nil), argv...), "--session-id", "dynamic-id")), WorkDir: t.TempDir()},
		managedWorkerArgv: append([]string(nil), argv...),
	}
	work := []beads.Bead{{
		ID: "hpf-work", Status: "open",
		Metadata: map[string]string{beadmeta.RoutedToMetadataKey: profileName, beadmeta.CheckPathMetadataKey: checkPath},
	}}
	probes := managedworker.Probes{
		ReadFile:        os.ReadFile,
		InspectProvider: func(context.Context, platforminstall.ProviderPin) error { return nil },
		ProbeReadiness:  func(context.Context, string) error { return nil },
		ProbeSigner:     func(context.Context, string) error { return nil },
	}
	configureManagedWorkerPreflight(&item, cityPath, nil, receipt.PermissionRevision, newTaskCheckPathResolver(work), &probes)
	if item.preflight == nil {
		t.Fatal("production preflight was not attached")
	}
	if err := item.preflight(context.Background()); err != nil {
		t.Fatalf("production preflight: %v", err)
	}
}

func TestTaskCheckPathResolverFailsClosedOnConflictingRoutedWork(t *testing.T) {
	profile := "hpfetcher/gc.implementation-worker"
	resolver := newTaskCheckPathResolver([]beads.Bead{
		{ID: "a", Status: "open", Metadata: map[string]string{beadmeta.RoutedToMetadataKey: profile, beadmeta.CheckPathMetadataKey: "/check/a"}},
		{ID: "b", Status: "open", Metadata: map[string]string{beadmeta.RoutedToMetadataKey: profile, beadmeta.CheckPathMetadataKey: "/check/b"}},
	})
	_, err := resolver(startCandidate{tp: TemplateParams{TemplateName: profile}}, nil)
	if err == nil || !strings.Contains(err.Error(), "conflicting gc.check_path") {
		t.Fatalf("resolver error = %v", err)
	}
}

func TestCommitStartFailureEmitsManagedWorkerAttentionInSameCycle(t *testing.T) {
	recorder := events.NewFake()
	failure := &managedworker.Failure{Profile: "hpfetcher/gc.implementation-worker", Err: errors.New("rules sha256 mismatch")}
	result := startResult{
		prepared: preparedStart{candidate: startCandidate{
			info: sessionpkg.Info{ID: "ci-managed", SessionName: "managed-worker", SessionNameMetadata: "managed-worker"},
			tp:   TemplateParams{TemplateName: "hpfetcher/gc.implementation-worker"},
		}},
		err:             failure,
		rollbackPending: true,
	}
	commitStartFailure(result, nil, &clock.Fake{Time: time.Unix(1, 0)}, recorder, 1, &bytes.Buffer{}, nil)
	if len(recorder.Events) != 1 {
		t.Fatalf("events = %#v, want one immediate attention event", recorder.Events)
	}
	got := recorder.Events[0]
	if got.Type != events.ManagedWorkerPreflightFailed || got.Subject != failure.Profile || got.SessionID != "ci-managed" {
		t.Fatalf("event = %#v", got)
	}
}

func TestProbeManagedWorkerSignerUsesNoSignManagedServiceReadiness(t *testing.T) {
	frontend := filepath.Join(t.TempDir(), "managed-git-commit")
	arguments := filepath.Join(t.TempDir(), "arguments")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + arguments + "\n"
	if err := os.WriteFile(frontend, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	previous := managedWorkerSignerFrontend
	managedWorkerSignerFrontend = frontend
	t.Cleanup(func() { managedWorkerSignerFrontend = previous })
	fingerprint := "ACCEBAF0C48FC8D43C527BE0C44EB18DC6A6E30F"

	if err := probeManagedWorkerSigner(context.Background(), fingerprint); err != nil {
		t.Fatalf("probeManagedWorkerSigner: %v", err)
	}
	data, err := os.ReadFile(arguments)
	if err != nil {
		t.Fatal(err)
	}
	want := "--probe\n--policy\ngascity-core\n--expect-fingerprint\n" + fingerprint + "\n"
	if string(data) != want {
		t.Fatalf("probe arguments = %q, want %q", data, want)
	}
	if err := probeManagedWorkerSigner(context.Background(), "SIGNER"); err == nil || !strings.Contains(err.Error(), "full 40-character fingerprint") {
		t.Fatalf("short identity error = %v", err)
	}
}

func preflightDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

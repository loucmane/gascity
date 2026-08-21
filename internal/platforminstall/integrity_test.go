package platforminstall

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestInspectIntegrityAcceptsExactFingerprint(t *testing.T) {
	dir := t.TempDir()
	manifest := integrityManifest(t, dir)
	if _, err := Install(manifest); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	report, err := InspectIntegrity(context.Background(), manifest)
	if err != nil {
		t.Fatalf("InspectIntegrity() error = %v", err)
	}
	if len(report.Drifts) != 0 {
		t.Fatalf("InspectIntegrity() drifts = %+v, want none", report.Drifts)
	}
}

func TestInspectIntegrityReportsAllDriftClasses(t *testing.T) {
	dir := t.TempDir()
	manifest := integrityManifest(t, dir)
	if _, err := Install(manifest); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if err := os.WriteFile(manifest.Integrity.Files[0].Path, []byte("rules drift"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(manifest.Integrity.Files[0].Path, 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, manifest.Integrity.Repositories[0].Path, "checkout", "--detach", "HEAD^")
	provider := manifest.Integrity.Providers[0]
	if err := os.Remove(provider.Path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "missing-provider"), provider.Path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest.Core.Destination, []byte("core drift"), 0o755); err != nil {
		t.Fatal(err)
	}

	report, err := InspectIntegrity(context.Background(), manifest)
	if err != nil {
		t.Fatalf("InspectIntegrity() error = %v", err)
	}
	fields := make([]string, 0, len(report.Drifts))
	for _, drift := range report.Drifts {
		fields = append(fields, drift.Field)
	}
	sort.Strings(fields)
	for _, want := range []string{
		"core.sha256",
		"files[control-rules].mode",
		"files[control-rules].sha256",
		"providers[codex].resolved_path",
		"repositories[template].commit",
	} {
		if !containsString(fields, want) {
			t.Errorf("drift fields = %v, want %q", fields, want)
		}
	}
}

func TestInspectIntegrityReportsProviderDigestAndVersionDrift(t *testing.T) {
	dir := t.TempDir()
	manifest := integrityManifest(t, dir)
	if _, err := Install(manifest); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	provider := manifest.Integrity.Providers[0]
	if err := os.WriteFile(provider.ResolvedPath, []byte("#!/bin/sh\nprintf 'codex-cli 0.148.0\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	report, err := InspectIntegrity(context.Background(), manifest)
	if err != nil {
		t.Fatalf("InspectIntegrity() error = %v", err)
	}
	fields := driftFields(report)
	for _, want := range []string{"providers[codex].sha256", "providers[codex].version"} {
		if !containsString(fields, want) {
			t.Errorf("drift fields = %v, want %q", fields, want)
		}
	}
}

func TestInspectIntegrityReportsReceiptSelfDigestDrift(t *testing.T) {
	dir := t.TempDir()
	manifest := integrityManifest(t, dir)
	if _, err := Install(manifest); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	receipt := mustReadFile(t, manifest.ReceiptPath)
	receipt = bytes.Replace(receipt, []byte(manifest.ReleaseID), []byte("different-release-id"), 1)
	if err := os.WriteFile(manifest.ReceiptPath, receipt, 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := InspectIntegrity(context.Background(), manifest)
	if err != nil {
		t.Fatalf("InspectIntegrity() error = %v", err)
	}
	if fields := driftFields(report); !containsString(fields, "receipt.self_digest") {
		t.Fatalf("drift fields = %v, want receipt.self_digest", fields)
	}
}

func TestInstallRejectsPinnedIntegrityDriftBeforeMutation(t *testing.T) {
	dir := t.TempDir()
	manifest := integrityManifest(t, dir)
	coreBefore := mustReadFile(t, manifest.Core.Destination)
	if err := os.WriteFile(manifest.Integrity.Files[0].Path, []byte("preflight drift"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Install(manifest)
	if err == nil || !strings.Contains(err.Error(), "preflight platform integrity drift") {
		t.Fatalf("Install() error = %v, want preflight integrity drift", err)
	}
	if got := mustReadFile(t, manifest.Core.Destination); !bytes.Equal(got, coreBefore) {
		t.Fatalf("core changed after pinned-integrity preflight failure: got %q want %q", got, coreBefore)
	}
	assertPathAbsent(t, manifest.BackupPath)
	assertPathAbsent(t, manifest.ReceiptPath)
}

func TestLoadManifestRejectsUnsafeIntegrityPins(t *testing.T) {
	dir := t.TempDir()
	manifest := integrityManifest(t, dir)
	manifest.Integrity.Files[0].Path = "relative/rules"
	manifest = finalizeManifest(t, manifest)

	_, err := LoadManifest(marshalManifest(t, manifest))
	if err == nil || !strings.Contains(err.Error(), "integrity.files[control-rules].path must be an absolute path") {
		t.Fatalf("LoadManifest() error = %v, want absolute integrity file path refusal", err)
	}
}

func integrityManifest(t *testing.T, dir string) Manifest {
	t.Helper()
	manifest := testManifest(t, dir, []byte("candidate"), []byte("installed"))
	rulesPath := filepath.Join(dir, "launcher", ".codex", "rules", "native.rules")
	if err := os.MkdirAll(filepath.Dir(rulesPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulesPath, []byte("allow gc hook"), 0o644); err != nil {
		t.Fatal(err)
	}

	repo := filepath.Join(dir, "template")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-q")
	git(t, repo, "config", "user.name", "fixture")
	git(t, repo, "config", "user.email", "fixture@example.invalid")
	git(t, repo, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(repo, "template.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "template.txt")
	git(t, repo, "commit", "-q", "-m", "one")
	firstCommit := gitOutput(t, repo, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(repo, "template.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "template.txt")
	git(t, repo, "commit", "-q", "-m", "two")
	secondCommit := gitOutput(t, repo, "rev-parse", "HEAD")
	if firstCommit == secondCommit {
		t.Fatal("fixture commits unexpectedly equal")
	}

	providerTarget := filepath.Join(dir, "providers", "releases", "codex")
	providerPath := filepath.Join(dir, "providers", "current", "codex")
	if err := os.MkdirAll(filepath.Dir(providerTarget), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(providerPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(providerTarget, []byte("#!/bin/sh\nprintf 'codex-cli 0.147.0\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(providerTarget, providerPath); err != nil {
		t.Fatal(err)
	}

	manifest.Integrity = &IntegritySpec{
		Files:        []FilePin{{Name: "control-rules", Path: rulesPath, SHA256: testSHA256([]byte("allow gc hook")), Mode: 0o644}},
		Repositories: []GitPin{{Name: "template", Path: repo, Commit: secondCommit}},
		Providers: []ProviderPin{{
			Name:         "codex",
			Path:         providerPath,
			ResolvedPath: providerTarget,
			SHA256:       testSHA256([]byte("#!/bin/sh\nprintf 'codex-cli 0.147.0\\n'\n")),
			VersionArgs:  []string{"--version"},
			Version:      "codex-cli 0.147.0",
		}},
	}
	return finalizeManifest(t, manifest)
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func driftFields(report IntegrityReport) []string {
	fields := make([]string, 0, len(report.Drifts))
	for _, drift := range report.Drifts {
		fields = append(fields, drift.Field)
	}
	sort.Strings(fields)
	return fields
}

package managedworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/platforminstall"
)

func TestLoadProvisioningReceiptIsStrictAndSelfDigested(t *testing.T) {
	receipt, encoded := finalizedReceipt(t)
	if !strings.Contains(string(encoded), `"canary_runner":`) {
		t.Fatalf("provisioning receipt does not bind the reviewed canary runner: %s", encoded)
	}

	loaded, err := LoadProvisioningReceipt(encoded)
	if err != nil {
		t.Fatalf("LoadProvisioningReceipt: %v", err)
	}
	if loaded.ReceiptSHA256 != receipt.ReceiptSHA256 {
		t.Fatalf("receipt digest = %q, want %q", loaded.ReceiptSHA256, receipt.ReceiptSHA256)
	}
	if strings.Contains(string(encoded), "\n") || strings.Contains(string(encoded), ": ") {
		t.Fatalf("receipt is not compact canonical JSON: %s", encoded)
	}
	if strings.Index(string(encoded), `"member_heads"`) > strings.Index(string(encoded), `"schema"`) {
		t.Fatalf("receipt keys are not sorted: %s", encoded)
	}

	unknown := append([]byte(nil), encoded[:len(encoded)-1]...)
	unknown = append(unknown, []byte(`,"unknown":true}`)...)
	if _, err := LoadProvisioningReceipt(unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown-field error = %v", err)
	}

	tampered := strings.Replace(string(encoded), `"permission_revision":"`+receipt.PermissionRevision, `"permission_revision":"`+digest("other"), 1)
	if _, err := LoadProvisioningReceipt([]byte(tampered)); err == nil || !strings.Contains(err.Error(), "receipt_sha256 mismatch") {
		t.Fatalf("tampered receipt error = %v", err)
	}
}

func TestWorkerProfileDigestCoversEveryLaunchControl(t *testing.T) {
	base := testProfile()
	want, err := WorkerProfileDigest(base)
	if err != nil {
		t.Fatalf("WorkerProfileDigest: %v", err)
	}

	mutations := map[string]func(*WorkerProfile){
		"argv":              func(p *WorkerProfile) { p.Argv[0] = "other" },
		"writable roots":    func(p *WorkerProfile) { p.WritableRoots[len(p.WritableRoots)-1] = "/zz-other" },
		"approval policy":   func(p *WorkerProfile) { p.ApprovalPolicy = "attended" },
		"sandbox mode":      func(p *WorkerProfile) { p.SandboxMode = "read-only" },
		"network policy":    func(p *WorkerProfile) { p.NetworkPolicy = "enabled" },
		"provider identity": func(p *WorkerProfile) { p.Provider.SHA256 = digest("different-provider") },
		"check path":        func(p *WorkerProfile) { p.CheckPath.SHA256 = digest("different-check") },
		"signer identity":   func(p *WorkerProfile) { p.SignerIdentity = "OTHER" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			got := cloneProfile(base)
			mutate(&got)
			digest, err := WorkerProfileDigest(got)
			if err != nil {
				t.Fatalf("WorkerProfileDigest: %v", err)
			}
			if digest == want {
				t.Fatalf("mutation did not change profile digest %s", want)
			}
		})
	}
}

func TestPreflightVerifiesEveryManagedWorkerBoundary(t *testing.T) {
	receipt, encoded := finalizedReceipt(t)
	profile := receipt.Profiles[0]
	var calls []string
	probes := Probes{
		ReadFile: func(path string) ([]byte, error) {
			calls = append(calls, "file:"+path)
			switch path {
			case receipt.Rules.Path:
				return []byte("rules"), nil
			case profile.CheckPath.Path:
				return []byte("check"), nil
			default:
				return nil, errors.New("unexpected path")
			}
		},
		InspectProvider: func(_ context.Context, got platforminstall.ProviderPin) error {
			calls = append(calls, "provider:"+got.Name)
			return nil
		},
		ProbeReadiness: func(_ context.Context, name string) error {
			calls = append(calls, "readiness:"+name)
			return nil
		},
		ProbeSigner: func(_ context.Context, identity string) error {
			calls = append(calls, "signer:"+identity)
			return nil
		},
	}

	report, err := Preflight(context.Background(), PreflightRequest{
		Receipt:            encoded,
		ProfileName:        profile.Name,
		ObservedProfile:    profile,
		PermissionRevision: receipt.PermissionRevision,
		CheckPath:          profile.CheckPath.Path,
	}, probes)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if !report.OK {
		t.Fatalf("report = %+v, want OK", report)
	}
	for _, want := range []string{
		"file:/city/.gc/platform/assets/gas-city-native-control.rules",
		"file:/cache/pack/assets/scripts/checks/build-artifact-valid.sh",
		"provider:codex", "readiness:codex", "signer:2ECF4432C7E7982D",
	} {
		if !contains(calls, want) {
			t.Fatalf("calls = %v, missing %q", calls, want)
		}
	}
}

func TestPreflightFailsClosedAtEachBoundary(t *testing.T) {
	receipt, encoded := finalizedReceipt(t)
	profile := receipt.Profiles[0]
	baseRequest := PreflightRequest{
		Receipt:            encoded,
		ProfileName:        profile.Name,
		ObservedProfile:    profile,
		PermissionRevision: receipt.PermissionRevision,
		CheckPath:          profile.CheckPath.Path,
	}
	baseProbes := func() Probes {
		return Probes{
			ReadFile: func(path string) ([]byte, error) {
				if path == receipt.Rules.Path {
					return []byte("rules"), nil
				}
				return []byte("check"), nil
			},
			InspectProvider: func(context.Context, platforminstall.ProviderPin) error { return nil },
			ProbeReadiness:  func(context.Context, string) error { return nil },
			ProbeSigner:     func(context.Context, string) error { return nil },
		}
	}

	tests := map[string]struct {
		mutateRequest func(*PreflightRequest)
		mutateProbes  func(*Probes)
		want          string
	}{
		"permission revision": {
			mutateRequest: func(r *PreflightRequest) { r.PermissionRevision = digest("stale-config") },
			want:          "permission_revision mismatch",
		},
		"profile digest": {
			mutateRequest: func(r *PreflightRequest) { r.ObservedProfile.NetworkPolicy = "enabled" },
			want:          "worker_profile_sha256 mismatch",
		},
		"check path stamp": {
			mutateRequest: func(r *PreflightRequest) { r.CheckPath = "/different/check" },
			want:          "gc.check_path mismatch",
		},
		"rules digest": {
			mutateProbes: func(p *Probes) {
				p.ReadFile = func(path string) ([]byte, error) {
					if path == receipt.Rules.Path {
						return []byte("tampered"), nil
					}
					return []byte("check"), nil
				}
			},
			want: "rules sha256 mismatch",
		},
		"provider identity": {
			mutateProbes: func(p *Probes) {
				p.InspectProvider = func(context.Context, platforminstall.ProviderPin) error { return errors.New("sha drift") }
			},
			want: "provider identity",
		},
		"provider readiness": {
			mutateProbes: func(p *Probes) {
				p.ProbeReadiness = func(context.Context, string) error { return errors.New("not logged in") }
			},
			want: "provider readiness",
		},
		"signer": {
			mutateProbes: func(p *Probes) {
				p.ProbeSigner = func(context.Context, string) error { return errors.New("key unavailable") }
			},
			want: "signer readiness",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			req := baseRequest
			req.ObservedProfile = cloneProfile(baseRequest.ObservedProfile)
			probes := baseProbes()
			if test.mutateRequest != nil {
				test.mutateRequest(&req)
			}
			if test.mutateProbes != nil {
				test.mutateProbes(&probes)
			}
			if _, err := Preflight(context.Background(), req, probes); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Preflight error = %v, want %q", err, test.want)
			}
		})
	}
}

func finalizedReceipt(t *testing.T) (ProvisioningReceipt, []byte) {
	t.Helper()
	receipt := ProvisioningReceipt{
		Schema: ProvisioningReceiptSchemaV1,
		MemberHeads: []MemberHead{
			{Name: "gct-xnf", Commit: strings.Repeat("a", 40)},
			{Name: "gct-xndt", Commit: strings.Repeat("b", 40)},
		},
		TemplateCommit: strings.Repeat("c", 40),
		Pack: PackPin{
			Source: "https://github.com/loucmane/gascity-packs/tree/main/gascity",
			Commit: strings.Repeat("d", 40),
			SHA256: digest("pack"),
		},
		Rules: FilePin{
			Path:   "/city/.gc/platform/assets/gas-city-native-control.rules",
			SHA256: digest("rules"),
		},
		PermissionRevision: digest("permission-config"),
		Profiles:           []WorkerProfile{testProfile()},
	}
	finalized, encoded, err := FinalizeProvisioningReceipt(receipt)
	if err != nil {
		t.Fatalf("FinalizeProvisioningReceipt: %v", err)
	}
	return finalized, encoded
}

func testProfile() WorkerProfile {
	return WorkerProfile{
		Name: "hpfetcher/gc.implementation-worker",
		Provider: platforminstall.ProviderPin{
			Name:         "codex",
			Path:         "/home/loucmane/gascity/bin/codex",
			ResolvedPath: "/home/loucmane/.codex/packages/standalone/releases/0.149.0/bin/codex",
			SHA256:       digest("provider"),
			VersionArgs:  []string{"--version"},
			Version:      "codex-cli 0.149.0",
		},
		CheckPath: FilePin{
			Path:   "/cache/pack/assets/scripts/checks/build-artifact-valid.sh",
			SHA256: digest("check"),
		},
		SignerIdentity: "2ECF4432C7E7982D",
		Argv: []string{
			"codex", "--ask-for-approval", "never", "--sandbox", "workspace-write",
		},
		WritableRoots:  []string{"/home/loucmane/dev/hpfetcher-worktrees", "/home/loucmane/dev/hpfetcher/.git", "/home/loucmane/vaults/main"},
		ApprovalPolicy: "never",
		SandboxMode:    "workspace-write",
		NetworkPolicy:  "restricted",
	}
}

func cloneProfile(profile WorkerProfile) WorkerProfile {
	profile.Argv = append([]string(nil), profile.Argv...)
	profile.WritableRoots = append([]string(nil), profile.WritableRoots...)
	profile.Provider.VersionArgs = append([]string(nil), profile.Provider.VersionArgs...)
	return profile
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

package managedworker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/platforminstall"
)

const (
	candidatePolicyBytes = `{"_gc":{"schema":"gc.candidate-control-policy.v1","profile_kind":"candidate"}}`
	candidatePolicyPath  = "/city/.gc/runtime/provisioning/policies/candidate.json"
)

// Wire fixtures exercise the real strict decoder, rather than constructing new
// Go fields that could accidentally omit the producer's declarations.
func typedReceiptWire(t *testing.T, kind string, mutate func(map[string]any)) []byte {
	t.Helper()
	_, encoded := finalizedReceipt(t)
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	profile := document["profiles"].([]any)[0].(map[string]any)
	profile["profile_kind"] = kind
	if kind == "candidate" {
		profile["signer_identity"] = "none"
		profile["name"] = "build/ops.candidate"
		profile["provider"] = map[string]any{
			"name": "claude", "path": "/fixture/bin/claude", "resolved_path": "/fixture/releases/claude",
			"sha256": digest("synthetic-claude"), "version_args": []string{"--version"}, "version": "fixture-claude 1",
		}
		profile["argv"] = []string{"claude", "--settings", candidatePolicyPath}
		profile["control_policy"] = map[string]any{"path": candidatePolicyPath, "sha256": digest(candidatePolicyBytes)}
	}
	if mutate != nil {
		mutate(profile)
	}
	delete(profile, "worker_profile_sha256")
	profileBytes, err := canonicalJSON(profile)
	if err != nil {
		t.Fatal(err)
	}
	profile["worker_profile_sha256"] = sha256Hex(profileBytes)
	delete(document, "receipt_sha256")
	receiptBytes, err := canonicalJSON(document)
	if err != nil {
		t.Fatal(err)
	}
	document["receipt_sha256"] = sha256Hex(receiptBytes)
	result, err := canonicalJSON(document)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestTypedProfileRoundTrip(t *testing.T) {
	for _, kind := range []string{"candidate", "signing"} {
		t.Run(kind, func(t *testing.T) {
			wire := typedReceiptWire(t, kind, nil)
			receipt, err := LoadProvisioningReceipt(wire)
			if err != nil {
				t.Fatalf("typed %s receipt refused: %v", kind, err)
			}
			roundTrip, err := canonicalJSON(receipt)
			if err != nil || string(roundTrip) != string(wire) {
				t.Fatalf("typed fields lost or normalized: error=%v\ngot=%s\nwant=%s", err, roundTrip, wire)
			}
		})
	}
}

func TestTypedProfileRejectsAmbiguousPolicyWire(t *testing.T) {
	for _, document := range []string{
		`{"control_policy":{"path":"/policy","path":"/other","sha256":"digest"}}`,
		`{"control_policy":{"path":"/policy","sha256":"one","sha256":"two"}}`,
		`{"control_policy":{"Path":"/policy","sha256":"digest"}}`,
		`{"CONTROL_POLICY":null}`,
		`{"PROFILE_KIND":"candidate"}`,
		`{"profile_kind":"signing","Profile_Kind":"candidate"}`,
	} {
		var profile WorkerProfile
		if err := json.Unmarshal([]byte(document), &profile); err == nil {
			t.Errorf("ambiguous policy authority decoded: %s", document)
		}
	}
}

func TestTypedProfileRejectsMalformedAndCrossSelectedControls(t *testing.T) {
	cases := map[string]func(map[string]any){
		"null kind":                func(p map[string]any) { p["profile_kind"] = nil },
		"empty kind":               func(p map[string]any) { p["profile_kind"] = "" },
		"unknown kind":             func(p map[string]any) { p["profile_kind"] = "privileged" },
		"boolean kind":             func(p map[string]any) { p["profile_kind"] = false },
		"candidate with signer":    func(p map[string]any) { p["signer_identity"] = testProfile().SignerIdentity },
		"candidate missing policy": func(p map[string]any) { delete(p, "control_policy") },
		"null policy":              func(p map[string]any) { p["control_policy"] = nil },
		"empty policy":             func(p map[string]any) { p["control_policy"] = map[string]any{} },
		"unknown policy field":     func(p map[string]any) { p["control_policy"].(map[string]any)["allow_all"] = true },
		"policy not in argv":       func(p map[string]any) { p["argv"] = []string{"claude"} },
		"policy without kind":      func(p map[string]any) { delete(p, "profile_kind"); p["signer_identity"] = testProfile().SignerIdentity },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadProvisioningReceipt(typedReceiptWire(t, "candidate", mutate)); err == nil {
				t.Fatal("malformed or contradictory controls accepted")
			}
		})
	}
	for _, explicit := range []bool{false, true} {
		wire := typedReceiptWire(t, "signing", func(p map[string]any) {
			p["signer_identity"] = "none"
			if !explicit {
				delete(p, "profile_kind")
			}
		})
		if _, err := LoadProvisioningReceipt(wire); err == nil {
			t.Fatalf("signing profile borrowed candidate no-signer sentinel, explicit=%v", explicit)
		}
	}
}

func TestTypedProfileLegacyDigestBaseline(t *testing.T) {
	receipt, encoded := finalizedReceipt(t)
	if receipt.ReceiptSHA256 != "3acb929dd8b836b306b46bb0c1c009182afebf0151fa30d541b4b85a061944bd" ||
		receipt.Profiles[0].WorkerProfileSHA256 != "da4bc80c903af42ed04954e3db8c4cbe8cfc5da992823519968b48c4447faf78" {
		t.Fatal("legacy v2 digest changed from the observed pre-repair baseline")
	}
	t.Logf("v2 receipt=%s profile=%s", receipt.ReceiptSHA256, receipt.Profiles[0].WorkerProfileSHA256)
	if strings.Contains(string(encoded), "profile_kind") || strings.Contains(string(encoded), "control_policy") {
		t.Fatal("legacy digest domain contains new declarations")
	}
	receipt.ReceiptSHA256 = ""
	receipt.Schema = ProvisioningReceiptSchemaV1
	receipt.Profiles[0].WorkerProfileSHA256 = ""
	receipt.Profiles[0].Environment = nil
	receipt.Profiles[0].Toolchains = nil
	v1, _, err := FinalizeProvisioningReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("v1 receipt=%s profile=%s", v1.ReceiptSHA256, v1.Profiles[0].WorkerProfileSHA256)
	if v1.ReceiptSHA256 != "e60839aa923583f813b50d15ce348396fc0a4843f432f00538f636493374776a" ||
		v1.Profiles[0].WorkerProfileSHA256 != "e8a5481cc4d200eb5b635734fa6a2eb2532584a2b63ae5f1f407b25542fda08e" {
		t.Fatal("legacy v1 digest changed from the observed pre-repair baseline")
	}
}

func TestTypedCandidatePreflightHasNoSignerAndPinsPolicy(t *testing.T) {
	wire := typedReceiptWire(t, "candidate", nil)
	receipt, err := LoadProvisioningReceipt(wire)
	if err != nil {
		t.Fatal(err)
	}
	profile := receipt.Profiles[0]
	request := PreflightRequest{
		Receipt: wire, ProfileName: profile.Name, ObservedProfile: profile,
		PermissionRevision: receipt.PermissionRevision, CheckPath: profile.CheckPath.Path,
	}
	providerCalls := 0
	probes := Probes{
		ReadFile: func(path string) ([]byte, error) {
			switch path {
			case receipt.Rules.Path:
				return []byte("rules"), nil
			case profile.CheckPath.Path:
				return []byte("check"), nil
			case candidatePolicyPath:
				return []byte(candidatePolicyBytes), nil
			default:
				return nil, errors.New("unexpected file")
			}
		},
		InspectProvider:  func(context.Context, platforminstall.ProviderPin) error { providerCalls++; return nil },
		InspectToolchain: func(context.Context, ToolchainPin, map[string]string) error { return nil },
		ProbeReadiness:   func(context.Context, string) error { return nil },
		// No signer probe exists in a candidate-only execution context.
	}
	probes.ReadControlPolicy = probes.ReadFile
	report, err := Preflight(context.Background(), request, probes)
	if err != nil || !report.OK || !contains(report.Checks, "control_policy") || contains(report.Checks, "signer") {
		t.Fatalf("candidate preflight: %+v, error=%v", report, err)
	}
	if providerCalls != 1 {
		t.Fatalf("provider calls=%d", providerCalls)
	}
	providerCalls = 0
	read := probes.ReadFile
	probes.ReadFile = func(path string) ([]byte, error) {
		if path == candidatePolicyPath {
			return []byte("tampered"), nil
		}
		return read(path)
	}
	probes.ReadControlPolicy = probes.ReadFile
	if _, err := Preflight(context.Background(), request, probes); err == nil || !strings.Contains(err.Error(), "control_policy sha256 mismatch") {
		t.Fatalf("policy drift not refused: %v", err)
	}
	if providerCalls != 0 {
		t.Fatal("provider reached before policy integrity check")
	}
	// A typed signing context still requires its signer probe.
	signingWire := typedReceiptWire(t, "signing", nil)
	signing, err := LoadProvisioningReceipt(signingWire)
	if err != nil {
		t.Fatal(err)
	}
	request.Receipt, request.ProfileName, request.ObservedProfile = signingWire, signing.Profiles[0].Name, signing.Profiles[0]
	if _, err := Preflight(context.Background(), request, probes); err == nil || !strings.Contains(err.Error(), "signer readiness") {
		t.Fatalf("signing probe requirement lost: %v", err)
	}
}

func TestTypedPolicyMarkerMustMatchReceipt(t *testing.T) {
	for _, document := range []string{
		`{"_gc":{"schema":"gc.candidate-control-policy.v1","profile_kind":"signing"}}`,
		`{"_gc":{"schema":"unknown","profile_kind":"candidate"}}`,
		`{}`, `null`, `not-json`,
	} {
		wire := typedReceiptWire(t, "candidate", func(p map[string]any) {
			p["control_policy"].(map[string]any)["sha256"] = digest(document)
		})
		receipt, err := LoadProvisioningReceipt(wire)
		if err != nil {
			t.Fatal(err)
		}
		profile := receipt.Profiles[0]
		probes := Probes{
			ReadFile: func(path string) ([]byte, error) {
				if path == receipt.Rules.Path {
					return []byte("rules"), nil
				}
				if path == profile.CheckPath.Path {
					return []byte("check"), nil
				}
				return []byte(document), nil
			},
			InspectProvider: func(context.Context, platforminstall.ProviderPin) error {
				t.Fatal("provider reached on invalid policy")
				return nil
			},
			InspectToolchain: func(context.Context, ToolchainPin, map[string]string) error { return nil },
			ProbeReadiness:   func(context.Context, string) error { return nil },
		}
		probes.ReadControlPolicy = probes.ReadFile
		_, err = Preflight(context.Background(), PreflightRequest{
			Receipt: wire, ProfileName: profile.Name,
			ObservedProfile: profile, PermissionRevision: receipt.PermissionRevision, CheckPath: profile.CheckPath.Path,
		}, probes)
		if err == nil || !strings.Contains(err.Error(), "control_policy") {
			t.Fatalf("invalid policy accepted: %s, error=%v", document, err)
		}
	}
}

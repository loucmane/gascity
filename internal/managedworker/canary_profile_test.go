package managedworker

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/fsys"
)

// These are storage-protocol fixtures, not a live provider or canary proof.
func typedCanaryWire(t *testing.T, kind string, mutate func(map[string]any)) []byte {
	t.Helper()
	provisioning, err := LoadProvisioningReceipt(typedReceiptWire(t, kind, nil))
	if err != nil {
		t.Fatal(err)
	}
	profile := provisioning.Profiles[0]
	names := RequiredCanaryScenarios()
	if kind == "candidate" {
		names = []string{"candidate-launcher"}
	}
	scenarios := make([]CanaryScenario, 0, len(names))
	for _, name := range names {
		scenarios = append(scenarios, CanaryScenario{Name: name, Outcome: CanaryResultPass})
	}
	base, err := canonicalJSON(canonicalizeCanaryReceipt(CanaryReceipt{
		Schema: "gc.canary-receipt.v2", CanaryRunID: "typed-storage-fixture", IssuedAt: "2026-09-08T12:00:00Z",
		Result: CanaryResultPass, Environment: testCanaryEnvironment(provisioning),
		ProvisioningReceipt: provisioning, Runner: provisioning.CanaryRunner, Scenarios: scenarios,
	}))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(base, &document); err != nil {
		t.Fatal(err)
	}
	document["profile"] = map[string]any{"name": profile.Name, "profile_kind": kind, "sha256": profile.WorkerProfileSHA256}
	if mutate != nil {
		mutate(document)
	}
	delete(document, "receipt_sha256")
	unsigned, err := canonicalJSON(document)
	if err != nil {
		t.Fatal(err)
	}
	document["receipt_sha256"] = sha256Hex(unsigned)
	encoded, err := canonicalJSON(document)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestTypedCanaryStorageRoundTrip(t *testing.T) {
	for _, kind := range []string{"candidate", "signing"} {
		t.Run(kind, func(t *testing.T) {
			wire := typedCanaryWire(t, kind, nil)
			receipt, err := LoadCanaryReceipt(wire)
			if err != nil {
				t.Fatalf("typed canary refused: %v", err)
			}
			encoded, err := canonicalJSON(receipt)
			if err != nil || string(encoded) != string(wire) {
				t.Fatalf("typed binding changed: %v", err)
			}
		})
	}
}

func TestTypedCanaryRejectsCrossSelectionAndIncompleteScenarios(t *testing.T) {
	for _, kind := range []string{"candidate", "signing"} {
		for name, mutate := range map[string]func(map[string]any){
			"opposite kind": func(d map[string]any) {
				other := "candidate"
				if kind == other {
					other = "signing"
				}
				d["profile"].(map[string]any)["profile_kind"] = other
			},
			"different exact identity": func(d map[string]any) { d["profile"].(map[string]any)["name"] = "unrelated/worker" },
			"different digest":         func(d map[string]any) { d["profile"].(map[string]any)["sha256"] = digest("other-profile") },
			"missing profile":          func(d map[string]any) { delete(d, "profile") },
			"null profile":             func(d map[string]any) { d["profile"] = nil },
			"missing scenario":         func(d map[string]any) { d["scenarios"] = []any{} },
			"extra scenario": func(d map[string]any) {
				d["scenarios"] = append(d["scenarios"].([]any), map[string]any{"name": "zzz-unreviewed", "outcome": "pass", "attention_latency_cycles": 0})
			},
			"environment disagrees": func(d map[string]any) {
				d["environment"].(map[string]any)["profiles"].([]any)[0].(map[string]any)["sha256"] = digest("not-observed")
			},
		} {
			t.Run(kind+"/"+name, func(t *testing.T) {
				if _, err := LoadCanaryReceipt(typedCanaryWire(t, kind, mutate)); err == nil {
					t.Fatal("invalid canary accepted")
				}
			})
		}
	}
}

func TestLegacyCanaryCannotCarryTypedProfileAuthority(t *testing.T) {
	for _, kind := range []string{"candidate", "signing"} {
		wire := typedCanaryWire(t, kind, func(d map[string]any) { d["schema"] = CanaryReceiptSchemaV1; delete(d, "profile") })
		if _, err := LoadCanaryReceipt(wire); err == nil {
			t.Fatalf("legacy unscoped receipt accepted typed %s authority", kind)
		}
	}
}

func TestTypedCanaryCannotUseUnscopedDispatchVerifier(t *testing.T) {
	for _, kind := range []string{"candidate", "signing"} {
		provisioning, err := LoadProvisioningReceipt(typedReceiptWire(t, kind, nil))
		if err != nil {
			t.Fatal(err)
		}
		_, err = VerifyCanaryReceipt(typedCanaryWire(t, kind, nil), testCanaryEnvironment(provisioning))
		if err == nil || !strings.Contains(err.Error(), "profile-scoped") {
			t.Fatalf("missing exact-profile refusal: %v", err)
		}
	}
}

func TestTypedCanaryRunPublishesOnlyItsExactProfile(t *testing.T) {
	for _, kind := range []ProfileKind{ProfileKindCandidate, ProfileKindSigning} {
		t.Run(string(kind), func(t *testing.T) {
			provisioning, err := LoadProvisioningReceipt(typedReceiptWire(t, string(kind), nil))
			if err != nil {
				t.Fatal(err)
			}
			profile := provisioning.Profiles[0]
			environment := testCanaryEnvironment(provisioning)
			filesystem := fsys.NewFake()
			filesystem.Files[CanaryReceiptPath("/city")] = []byte("preserved legacy receipt")
			var calls []string
			receipt, err := RunGoldenPathCanary(context.Background(), CanaryRunRequest{
				CityPath: "/city", RunID: "typed-run-fixture", MaxWallTime: time.Minute,
				Environment: environment, ProvisioningReceipt: provisioning, Runner: provisioning.CanaryRunner,
				Profile: &CanaryProfile{Name: profile.Name, Kind: kind, SHA256: profile.WorkerProfileSHA256},
			}, CanaryRunDeps{
				FS: filesystem, Now: func() time.Time { return time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC) },
				RunScenario: func(_ context.Context, name string) (CanaryScenarioEvidence, error) {
					calls = append(calls, name)
					return passingTypedCanaryEvidence(name), nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			want, _ := RequiredCanaryScenariosFor(kind)
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("calls=%v want=%v", calls, want)
			}
			if got := string(filesystem.Files[CanaryReceiptPath("/city")]); got != "preserved legacy receipt" {
				t.Fatal("legacy receipt overwritten")
			}
			if CurrentCanaryReceiptPath("/city", receipt) != ProfileCanaryReceiptPath("/city", profile.Name) {
				t.Fatal("wrong profile storage path")
			}
			wire := filesystem.Files[ProfileCanaryReceiptPath("/city", profile.Name)]
			if _, err := VerifyProfileCanaryReceipt(wire, environment, profile.Name, kind); err != nil {
				t.Fatal(err)
			}
			other := ProfileKindCandidate
			if kind == other {
				other = ProfileKindSigning
			}
			if _, err := VerifyProfileCanaryReceipt(wire, environment, profile.Name, other); err == nil {
				t.Fatal("cross-kind proof accepted")
			}
			if _, err := VerifyProfileCanaryReceipt(wire, environment, "other/agent", kind); err == nil {
				t.Fatal("cross-profile proof accepted")
			}
			if _, err := VerifyCanaryReceipt(wire, environment); err == nil {
				t.Fatal("scoped proof accepted by unscoped dispatch")
			}
		})
	}
}

func TestTypedCanaryRunRefusesBeforeScenarioOnBindingDrift(t *testing.T) {
	for name, mutate := range map[string]func(*CanaryRunRequest){
		"opposite kind":             func(r *CanaryRunRequest) { r.Profile.Kind = ProfileKindSigning },
		"different identity":        func(r *CanaryRunRequest) { r.Profile.Name = "other/agent" },
		"different digest":          func(r *CanaryRunRequest) { r.Profile.SHA256 = digest("other") },
		"missing explicit selector": func(r *CanaryRunRequest) { r.Profile = nil },
		"invalid provisioning":      func(r *CanaryRunRequest) { r.ProvisioningReceipt.ReceiptSHA256 = digest("wrong") },
		"invalid environment":       func(r *CanaryRunRequest) { r.Environment.PermissionRevision = digest("wrong") },
	} {
		t.Run(name, func(t *testing.T) {
			provisioning, err := LoadProvisioningReceipt(typedReceiptWire(t, "candidate", nil))
			if err != nil {
				t.Fatal(err)
			}
			profile := provisioning.Profiles[0]
			request := CanaryRunRequest{
				CityPath: "/city", RunID: "refusal-fixture", MaxWallTime: time.Minute,
				ProvisioningReceipt: provisioning, Environment: testCanaryEnvironment(provisioning), Runner: provisioning.CanaryRunner,
				Profile: &CanaryProfile{Name: profile.Name, Kind: ProfileKindCandidate, SHA256: profile.WorkerProfileSHA256},
			}
			mutate(&request)
			filesystem := fsys.NewFake()
			_, err = RunGoldenPathCanary(context.Background(), request, CanaryRunDeps{
				FS: filesystem, Now: time.Now,
				RunScenario: func(context.Context, string) (CanaryScenarioEvidence, error) {
					t.Fatal("scenario ran after invalid selection")
					return CanaryScenarioEvidence{}, nil
				},
			})
			if err == nil || len(filesystem.Calls) != 0 {
				t.Fatalf("error=%v filesystem calls=%v", err, filesystem.Calls)
			}
		})
	}
}

func TestCandidateCanaryRefusesSigningAndIncompleteSteps(t *testing.T) {
	for name, mutate := range map[string]func(*CanaryScenarioEvidence){
		"signing":       func(e *CanaryScenarioEvidence) { e.SignedCandidate = true },
		"signing steps": func(e *CanaryScenarioEvidence) { e.CompletedSteps = RequiredCleanLauncherSteps() },
		"missing step":  func(e *CanaryScenarioEvidence) { e.CompletedSteps = e.CompletedSteps[:len(e.CompletedSteps)-1] },
	} {
		t.Run(name, func(t *testing.T) {
			evidence := passingTypedCanaryEvidence(CanaryScenarioCandidateLauncher)
			mutate(&evidence)
			if err := validateCanaryScenarioEvidence(CanaryScenarioCandidateLauncher, evidence); err == nil {
				t.Fatal("candidate violation accepted")
			}
		})
	}
}

func passingTypedCanaryEvidence(name string) CanaryScenarioEvidence {
	if name == CanaryScenarioCandidateLauncher {
		return CanaryScenarioEvidence{Name: name, Resolution: CanaryResolutionCompleted, CompletedSteps: RequiredCandidateLauncherSteps()}
	}
	return passingCanaryScenarioEvidence(name)
}

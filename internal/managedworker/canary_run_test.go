package managedworker

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/fsys"
)

func TestRunGoldenPathCanaryPublishesOnlyAfterEveryRequiredScenarioPasses(t *testing.T) {
	provisioning, _ := finalizedReceipt(t)
	cityPath := "/city"
	fakeFS := fsys.NewFake()
	fakeFS.Dirs[cityPath] = true

	var observed []string
	receipt, err := RunGoldenPathCanary(context.Background(), CanaryRunRequest{
		CityPath:            cityPath,
		Environment:         testCanaryEnvironment(provisioning),
		MaxWallTime:         time.Minute,
		ProvisioningReceipt: provisioning,
		Runner:              testCanaryRunnerPin(),
		RunID:               "canary-20260822-green",
	}, CanaryRunDeps{
		FS:  fakeFS,
		Now: func() time.Time { return time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC) },
		RunScenario: func(_ context.Context, name string) (CanaryScenarioEvidence, error) {
			observed = append(observed, name)
			return passingCanaryScenarioEvidence(name), nil
		},
	})
	if err != nil {
		t.Fatalf("RunGoldenPathCanary: %v", err)
	}
	if !reflect.DeepEqual(observed, RequiredCanaryScenarios()) {
		t.Fatalf("scenario order = %v, want %v", observed, RequiredCanaryScenarios())
	}
	if receipt.Result != CanaryResultPass || receipt.ReceiptSHA256 == "" {
		t.Fatalf("receipt = %+v, want finalized pass", receipt)
	}
	if receipt.Runner != testCanaryRunnerPin() {
		t.Fatalf("receipt runner = %+v, want %+v", receipt.Runner, testCanaryRunnerPin())
	}

	receiptPath := CanaryReceiptPath(cityPath)
	encoded, ok := fakeFS.Files[receiptPath]
	if !ok {
		t.Fatalf("receipt was not published at %s", receiptPath)
	}
	loaded, err := LoadCanaryReceipt(encoded)
	if err != nil {
		t.Fatalf("LoadCanaryReceipt(published): %v", err)
	}
	if loaded.ReceiptSHA256 != receipt.ReceiptSHA256 {
		t.Fatalf("published digest = %q, want %q", loaded.ReceiptSHA256, receipt.ReceiptSHA256)
	}
	if len(loaded.Scenarios) != len(RequiredCanaryScenarios()) {
		t.Fatalf("published scenarios = %d, want %d", len(loaded.Scenarios), len(RequiredCanaryScenarios()))
	}
	if !hasFSMethod(fakeFS.Calls, "Rename") {
		t.Fatalf("filesystem calls = %+v, want atomic rename to %s", fakeFS.Calls, receiptPath)
	}
	historyPath := CanaryHistoryReceiptPath(cityPath, receipt.ReceiptSHA256)
	history, ok := fakeFS.Files[historyPath]
	if !ok {
		t.Fatalf("append-only history receipt was not published at %s", historyPath)
	}
	if !bytes.Equal(history, encoded) {
		t.Fatal("history receipt differs from verified current receipt")
	}
	for path := range fakeFS.Files {
		if strings.Contains(filepath.Base(path), ".tmp.") {
			t.Fatalf("temporary receipt survived successful publication: %s", path)
		}
	}
}

func TestRunGoldenPathCanaryFailureNeverPublishesOrReplacesReceipt(t *testing.T) {
	provisioning, _ := finalizedReceipt(t)
	cityPath := "/city"
	receiptPath := CanaryReceiptPath(cityPath)
	previous := []byte("previous-reviewed-receipt")
	fakeFS := fsys.NewFake()
	fakeFS.Dirs[filepath.Dir(receiptPath)] = true
	fakeFS.Files[receiptPath] = append([]byte(nil), previous...)

	_, err := RunGoldenPathCanary(context.Background(), CanaryRunRequest{
		CityPath:            cityPath,
		Environment:         testCanaryEnvironment(provisioning),
		MaxWallTime:         time.Minute,
		ProvisioningReceipt: provisioning,
		Runner:              testCanaryRunnerPin(),
		RunID:               "canary-20260822-red",
	}, CanaryRunDeps{
		FS:  fakeFS,
		Now: func() time.Time { return time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC) },
		RunScenario: func(_ context.Context, name string) (CanaryScenarioEvidence, error) {
			if name == CanaryScenarioDeniedSubprocessSocket {
				return CanaryScenarioEvidence{}, errors.New("injected transport fault was silent")
			}
			return passingCanaryScenarioEvidence(name), nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), CanaryScenarioDeniedSubprocessSocket) {
		t.Fatalf("RunGoldenPathCanary error = %v, want named scenario failure", err)
	}
	if got := fakeFS.Files[receiptPath]; !reflect.DeepEqual(got, previous) {
		t.Fatalf("receipt changed on failed run: got %q want %q", got, previous)
	}
	if hasFSMethod(fakeFS.Calls, "Rename") {
		t.Fatalf("failed run attempted receipt publication: %+v", fakeFS.Calls)
	}
}

func TestRunGoldenPathCanaryRejectsVacuousOrUnsafeEvidence(t *testing.T) {
	tests := map[string]struct {
		scenario string
		mutate   func(*CanaryScenarioEvidence)
		want     string
	}{
		"manual intervention": {
			scenario: CanaryScenarioCleanLauncher,
			mutate:   func(e *CanaryScenarioEvidence) { e.ManualInterventions = 1 },
			want:     "manual interventions",
		},
		"coordinator watcher": {
			scenario: CanaryScenarioCleanLauncher,
			mutate:   func(e *CanaryScenarioEvidence) { e.CoordinatorWatchers = 1 },
			want:     "coordinator watchers",
		},
		"unsigned candidate": {
			scenario: CanaryScenarioCleanLauncher,
			mutate:   func(e *CanaryScenarioEvidence) { e.SignedCandidate = false },
			want:     "signed candidate",
		},
		"incomplete clean journey": {
			scenario: CanaryScenarioCleanLauncher,
			mutate:   func(e *CanaryScenarioEvidence) { e.CompletedSteps = e.CompletedSteps[:len(e.CompletedSteps)-1] },
			want:     "eight golden-path steps",
		},
		"silent no work": {
			scenario: CanaryScenarioUnreadableMail,
			mutate:   func(e *CanaryScenarioEvidence) { e.SilentNoWork = true },
			want:     "silent no_work",
		},
		"late attention": {
			scenario: CanaryScenarioMissingProvider,
			mutate: func(e *CanaryScenarioEvidence) {
				e.Resolution = CanaryResolutionAttention
				e.AttentionLatencyCycles = 2
			},
			want: "within one reconciliation cycle",
		},
		"publisher after failed finalize": {
			scenario: CanaryScenarioFailedFinalize,
			mutate:   func(e *CanaryScenarioEvidence) { e.PublisherOutcome = CanaryPublisherPublished },
			want:     "publisher blocked",
		},
		"publisher not noop": {
			scenario: CanaryScenarioSuccessfulFinalize,
			mutate:   func(e *CanaryScenarioEvidence) { e.PublisherOutcome = CanaryPublisherPublished },
			want:     "noop publisher",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			provisioning, _ := finalizedReceipt(t)
			fakeFS := fsys.NewFake()
			fakeFS.Dirs["/city"] = true
			_, err := RunGoldenPathCanary(context.Background(), CanaryRunRequest{
				CityPath:            "/city",
				Environment:         testCanaryEnvironment(provisioning),
				MaxWallTime:         time.Minute,
				ProvisioningReceipt: provisioning,
				Runner:              testCanaryRunnerPin(),
				RunID:               "canary-20260822-invalid",
			}, CanaryRunDeps{
				FS:  fakeFS,
				Now: func() time.Time { return time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC) },
				RunScenario: func(_ context.Context, scenario string) (CanaryScenarioEvidence, error) {
					evidence := passingCanaryScenarioEvidence(scenario)
					if scenario == test.scenario {
						test.mutate(&evidence)
					}
					return evidence, nil
				},
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("RunGoldenPathCanary error = %v, want %q", err, test.want)
			}
			if _, exists := fakeFS.Files[CanaryReceiptPath("/city")]; exists {
				t.Fatal("invalid evidence published a receipt")
			}
		})
	}
}

func passingCanaryScenarioEvidence(name string) CanaryScenarioEvidence {
	evidence := CanaryScenarioEvidence{
		Name:                   name,
		Resolution:             CanaryResolutionRecovered,
		AttentionLatencyCycles: 0,
	}
	switch name {
	case CanaryScenarioCleanLauncher:
		evidence.Resolution = CanaryResolutionCompleted
		evidence.CompletedSteps = RequiredCleanLauncherSteps()
		evidence.SignedCandidate = true
	case CanaryScenarioFailedFinalize:
		evidence.FinalizeOutcome = CanaryFinalizeFailed
		evidence.PublisherOutcome = CanaryPublisherBlocked
	case CanaryScenarioSuccessfulFinalize:
		evidence.FinalizeOutcome = CanaryFinalizePassed
		evidence.PublisherOutcome = CanaryPublisherNoop
	}
	return evidence
}

func hasFSMethod(calls []fsys.Call, method string) bool {
	for _, call := range calls {
		if call.Method == method {
			return true
		}
	}
	return false
}

func testCanaryRunnerPin() FilePin {
	return FilePin{Path: "/city/bin/managed-worker-canary", SHA256: digest("canary-runner")}
}

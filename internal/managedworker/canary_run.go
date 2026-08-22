package managedworker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/fsys"
)

const (
	// CanaryScenarioCleanLauncher proves an unattended eight-step signed run.
	CanaryScenarioCleanLauncher = "clean-launcher"
	// CanaryScenarioValidatorAbsent proves a missing validator is not silent.
	CanaryScenarioValidatorAbsent = "validator-absent"
	// CanaryScenarioDetachedHead proves worktree preparation creates a branch.
	CanaryScenarioDetachedHead = "detached-head"
	// CanaryScenarioUnreadableMail proves replies remain deliverable in-band.
	CanaryScenarioUnreadableMail = "unreadable-mail"
	// CanaryScenarioDeniedSubprocessSocket proves transport failures are surfaced.
	CanaryScenarioDeniedSubprocessSocket = "denied-subprocess-socket"
	// CanaryScenarioStaleSession proves killed runtimes cannot remain active.
	CanaryScenarioStaleSession = "stale-session-killed-tmux"
	// CanaryScenarioMissingProvider proves missing provider binaries raise attention.
	CanaryScenarioMissingProvider = "missing-provider"
	// CanaryScenarioFailedFinalize proves a failed workflow cannot publish.
	CanaryScenarioFailedFinalize = "publisher-failed-finalize"
	// CanaryScenarioSuccessfulFinalize proves push=false publishing is a no-op.
	CanaryScenarioSuccessfulFinalize = "publisher-success-noop"

	// CanaryResolutionCompleted records normal end-to-end completion.
	CanaryResolutionCompleted = "completed"
	// CanaryResolutionRecovered records automatic recovery from an injected fault.
	CanaryResolutionRecovered = "recovered"
	// CanaryResolutionAttention records a bounded operator-attention event.
	CanaryResolutionAttention = "attention_event"

	// CanaryFinalizeFailed records a deliberately failed workflow finalizer.
	CanaryFinalizeFailed = "failed"
	// CanaryFinalizePassed records a successful workflow finalizer.
	CanaryFinalizePassed = "passed"

	// CanaryPublisherBlocked records that publishing was correctly withheld.
	CanaryPublisherBlocked = "blocked"
	// CanaryPublisherNoop records the push=false no-op publication path.
	CanaryPublisherNoop = "noop"
	// CanaryPublisherPublished is used by tests to model a forbidden publish.
	CanaryPublisherPublished = "published"
)

var requiredCanaryScenarios = []string{
	CanaryScenarioCleanLauncher,
	CanaryScenarioValidatorAbsent,
	CanaryScenarioDetachedHead,
	CanaryScenarioUnreadableMail,
	CanaryScenarioDeniedSubprocessSocket,
	CanaryScenarioStaleSession,
	CanaryScenarioMissingProvider,
	CanaryScenarioFailedFinalize,
	CanaryScenarioSuccessfulFinalize,
}

var requiredCleanLauncherSteps = []string{
	"clone-launcher",
	"init-scratch-city",
	"register-test-rig",
	"dispatch-work",
	"prepare-worktree",
	"controller-validate-artifact",
	"sign-candidate",
	"finalize-workflow",
}

// CanaryScenarioEvidence is the controller-observed result of one canary
// scenario. It intentionally records the failure modes that were previously
// mistaken for progress: coordinator help, silent no_work, and late attention.
type CanaryScenarioEvidence struct {
	AttentionLatencyCycles int      `json:"attention_latency_cycles"`
	CompletedSteps         []string `json:"completed_steps,omitempty"`
	CoordinatorWatchers    int      `json:"coordinator_watchers"`
	FinalizeOutcome        string   `json:"finalize_outcome,omitempty"`
	ManualInterventions    int      `json:"manual_interventions"`
	Name                   string   `json:"name"`
	PublisherOutcome       string   `json:"publisher_outcome,omitempty"`
	Resolution             string   `json:"resolution"`
	SignedCandidate        bool     `json:"signed_candidate"`
	SilentNoWork           bool     `json:"silent_no_work"`
}

// CanaryRunRequest binds one clean/fault canary run to the exact environment
// that will be authorized for managed-product dispatch.
type CanaryRunRequest struct {
	CityPath            string
	Environment         CanaryEnvironment
	MaxWallTime         time.Duration
	ProvisioningReceipt ProvisioningReceipt
	RunID               string
}

// CanaryRunDeps contains the controller-owned side-effect boundaries. The
// scenario runner is supplied by the composition root so the policy remains
// independently testable while the production runner exercises real cities.
type CanaryRunDeps struct {
	FS          fsys.FS
	Now         func() time.Time
	RunScenario func(context.Context, string) (CanaryScenarioEvidence, error)
}

// RequiredCanaryScenarios returns the stable scenario order. A copy is
// returned so callers cannot weaken a later run by mutating package state.
func RequiredCanaryScenarios() []string {
	return append([]string(nil), requiredCanaryScenarios...)
}

// RequiredCleanLauncherSteps returns the eight unattended golden-path steps.
func RequiredCleanLauncherSteps() []string {
	return append([]string(nil), requiredCleanLauncherSteps...)
}

// RunGoldenPathCanary executes every required scenario under one wall-clock
// bound and publishes a receipt only after all observations pass. The receipt
// write is the final operation; failed or partial runs leave any prior receipt
// byte-for-byte untouched.
func RunGoldenPathCanary(ctx context.Context, request CanaryRunRequest, deps CanaryRunDeps) (CanaryReceipt, error) {
	if err := validateCanaryRunRequest(request, deps); err != nil {
		return CanaryReceipt{}, err
	}
	runContext, cancel := context.WithTimeout(ctx, request.MaxWallTime)
	defer cancel()

	scenarios := make([]CanaryScenario, 0, len(requiredCanaryScenarios))
	for _, name := range requiredCanaryScenarios {
		evidence, err := deps.RunScenario(runContext, name)
		if err != nil {
			return CanaryReceipt{}, fmt.Errorf("canary scenario %q: %w", name, err)
		}
		if err := validateCanaryScenarioEvidence(name, evidence); err != nil {
			return CanaryReceipt{}, fmt.Errorf("canary scenario %q: %w", name, err)
		}
		scenarios = append(scenarios, CanaryScenario{
			Name:                   name,
			Outcome:                CanaryResultPass,
			AttentionLatencyCycles: evidence.AttentionLatencyCycles,
		})
	}
	if err := runContext.Err(); err != nil {
		return CanaryReceipt{}, fmt.Errorf("canary wall-clock bound: %w", err)
	}

	receipt := CanaryReceipt{
		Schema:              CanaryReceiptSchemaV1,
		CanaryRunID:         request.RunID,
		IssuedAt:            deps.Now().UTC().Format(time.RFC3339),
		Result:              CanaryResultPass,
		Environment:         request.Environment,
		ProvisioningReceipt: request.ProvisioningReceipt,
		Scenarios:           scenarios,
	}
	return PublishCanaryReceipt(deps.FS, request.CityPath, receipt)
}

// PublishCanaryReceipt atomically installs and then independently reloads the
// canonical pass receipt. A caller cannot claim canary success until this
// verification returns the same self-digest that was finalized in memory.
func PublishCanaryReceipt(filesystem fsys.FS, cityPath string, receipt CanaryReceipt) (CanaryReceipt, error) {
	if filesystem == nil {
		return CanaryReceipt{}, errors.New("canary receipt filesystem is required")
	}
	cityPath = filepath.Clean(strings.TrimSpace(cityPath))
	if !filepath.IsAbs(cityPath) || cityPath == string(filepath.Separator) {
		return CanaryReceipt{}, fmt.Errorf("city path must be a clean absolute non-root path: %q", cityPath)
	}
	finalized, encoded, err := FinalizeCanaryReceipt(receipt)
	if err != nil {
		return CanaryReceipt{}, fmt.Errorf("finalize canary receipt: %w", err)
	}
	path := CanaryReceiptPath(cityPath)
	if err := filesystem.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return CanaryReceipt{}, fmt.Errorf("create canary receipt directory: %w", err)
	}
	if err := fsys.WriteFileAtomic(filesystem, path, encoded, 0o600); err != nil {
		return CanaryReceipt{}, fmt.Errorf("publish canary receipt: %w", err)
	}
	observed, err := filesystem.ReadFile(path)
	if err != nil {
		return CanaryReceipt{}, fmt.Errorf("verify published canary receipt: %w", err)
	}
	if !bytes.Equal(observed, encoded) {
		return CanaryReceipt{}, errors.New("verify published canary receipt: bytes changed after atomic rename")
	}
	loaded, err := LoadCanaryReceipt(observed)
	if err != nil {
		return CanaryReceipt{}, fmt.Errorf("verify published canary receipt: %w", err)
	}
	if !equalDigest(loaded.ReceiptSHA256, finalized.ReceiptSHA256) {
		return CanaryReceipt{}, fmt.Errorf("verify published canary receipt: digest got %q want %q", loaded.ReceiptSHA256, finalized.ReceiptSHA256)
	}
	return loaded, nil
}

func validateCanaryRunRequest(request CanaryRunRequest, deps CanaryRunDeps) error {
	if strings.TrimSpace(request.RunID) == "" {
		return errors.New("canary run id is required")
	}
	if request.MaxWallTime <= 0 {
		return errors.New("canary max wall time must be positive")
	}
	if strings.TrimSpace(request.CityPath) == "" {
		return errors.New("canary city path is required")
	}
	if deps.FS == nil {
		return errors.New("canary filesystem is required")
	}
	if deps.Now == nil {
		return errors.New("canary clock is required")
	}
	if deps.RunScenario == nil {
		return errors.New("canary scenario runner is required")
	}
	return nil
}

func validateCanaryScenarioEvidence(wantName string, evidence CanaryScenarioEvidence) error {
	if evidence.Name != wantName {
		return fmt.Errorf("evidence name = %q, want %q", evidence.Name, wantName)
	}
	if evidence.ManualInterventions != 0 {
		return fmt.Errorf("manual interventions = %d, want zero", evidence.ManualInterventions)
	}
	if evidence.CoordinatorWatchers != 0 {
		return fmt.Errorf("coordinator watchers = %d, want zero", evidence.CoordinatorWatchers)
	}
	if evidence.SilentNoWork {
		return errors.New("silent no_work is forbidden")
	}
	if evidence.AttentionLatencyCycles < 0 {
		return errors.New("attention latency cycles must not be negative")
	}

	switch wantName {
	case CanaryScenarioCleanLauncher:
		if evidence.Resolution != CanaryResolutionCompleted {
			return fmt.Errorf("resolution = %q, want %q", evidence.Resolution, CanaryResolutionCompleted)
		}
		if !reflect.DeepEqual(evidence.CompletedSteps, requiredCleanLauncherSteps) {
			return fmt.Errorf("completed steps do not equal the eight golden-path steps: got %v want %v", evidence.CompletedSteps, requiredCleanLauncherSteps)
		}
		if !evidence.SignedCandidate {
			return errors.New("clean launcher did not produce a signed candidate")
		}
	case CanaryScenarioFailedFinalize:
		if evidence.FinalizeOutcome != CanaryFinalizeFailed || evidence.PublisherOutcome != CanaryPublisherBlocked {
			return fmt.Errorf("failed finalize must leave publisher blocked: finalize=%q publisher=%q", evidence.FinalizeOutcome, evidence.PublisherOutcome)
		}
	case CanaryScenarioSuccessfulFinalize:
		if evidence.FinalizeOutcome != CanaryFinalizePassed || evidence.PublisherOutcome != CanaryPublisherNoop {
			return fmt.Errorf("successful finalize with push=false must produce a noop publisher: finalize=%q publisher=%q", evidence.FinalizeOutcome, evidence.PublisherOutcome)
		}
	default:
		if evidence.Resolution != CanaryResolutionRecovered && evidence.Resolution != CanaryResolutionAttention {
			return fmt.Errorf("fault resolution = %q, want recovered or attention_event", evidence.Resolution)
		}
		if evidence.Resolution == CanaryResolutionAttention && evidence.AttentionLatencyCycles > 1 {
			return fmt.Errorf("attention event arrived after %d cycles, want within one reconciliation cycle", evidence.AttentionLatencyCycles)
		}
	}
	return nil
}

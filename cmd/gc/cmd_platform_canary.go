package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/gastownhall/gascity/internal/managedworker"
	"github.com/spf13/cobra"
)

type platformCanaryOptions struct {
	baseCommit     string
	launcherSource string
	maxWallTime    time.Duration
	runID          string
	runnerPath     string
	runnerSHA256   string
	scratchRoot    string
}

type platformCanaryScenarioCall struct {
	BaseCommit     string
	CityPath       string
	LauncherSource string
	RunID          string
	RunnerPath     string
	RunnerSHA256   string
	Scenario       string
	ScratchRoot    string
	WorkerProfile  managedworker.WorkerProfile
}

type platformCanaryRuntime struct {
	FS          fsys.FS
	LoadInputs  func(context.Context, string) (managedworker.ProvisioningReceipt, managedworker.CanaryEnvironment, error)
	Now         func() time.Time
	ResolveCity func() (string, error)
	RunScenario func(context.Context, platformCanaryScenarioCall) (managedworker.CanaryScenarioEvidence, error)
}

var platformCanaryRuntimeFactory = defaultPlatformCanaryRuntime

func newPlatformCanaryCmd(stdout, _ io.Writer) *cobra.Command {
	options := platformCanaryOptions{}
	cmd := &cobra.Command{
		Use:   "canary",
		Short: "Run the bounded managed-worker golden-path and fault canary",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			receipt, path, err := runPlatformCanary(cmd.Context(), options, platformCanaryRuntimeFactory())
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "platform canary result=%s run_id=%q receipt_sha256=%s receipt=%s\n", receipt.Result, receipt.CanaryRunID, receipt.ReceiptSHA256, path) //nolint:errcheck // best-effort stdout
			return nil
		},
	}
	cmd.Flags().StringVar(&options.runID, "run-id", "", "unique canary run identifier")
	cmd.Flags().StringVar(&options.runnerPath, "runner", "", "absolute path to the reviewed canary scenario runner")
	cmd.Flags().StringVar(&options.runnerSHA256, "runner-sha256", "", "lowercase SHA-256 of the reviewed runner")
	cmd.Flags().StringVar(&options.launcherSource, "launcher-source", "", "absolute clean launcher source cloned by the runner")
	cmd.Flags().StringVar(&options.baseCommit, "base-commit", "", "full lowercase Git commit for the fresh launcher clone")
	cmd.Flags().StringVar(&options.scratchRoot, "scratch-root", "", "absolute empty-parent root for canary artifacts")
	cmd.Flags().DurationVar(&options.maxWallTime, "max-wall-time", 30*time.Minute, "hard wall-clock bound for the complete canary")
	for _, name := range []string{"run-id", "runner", "runner-sha256", "launcher-source", "base-commit", "scratch-root"} {
		_ = cmd.MarkFlagRequired(name)
	}
	return cmd
}

func runPlatformCanary(ctx context.Context, options platformCanaryOptions, runtime platformCanaryRuntime) (managedworker.CanaryReceipt, string, error) {
	if err := validatePlatformCanaryRuntime(runtime); err != nil {
		return managedworker.CanaryReceipt{}, "", err
	}
	if err := validatePlatformCanaryOptions(options); err != nil {
		return managedworker.CanaryReceipt{}, "", err
	}
	cityPath, err := runtime.ResolveCity()
	if err != nil {
		return managedworker.CanaryReceipt{}, "", fmt.Errorf("resolve canary city: %w", err)
	}
	cityPath = filepath.Clean(cityPath)
	provisioning, environment, err := runtime.LoadInputs(ctx, cityPath)
	if err != nil {
		return managedworker.CanaryReceipt{}, "", fmt.Errorf("load canary inputs: %w", err)
	}
	runner := managedworker.FilePin{Path: options.runnerPath, SHA256: options.runnerSHA256}
	if runner != provisioning.CanaryRunner {
		return managedworker.CanaryReceipt{}, "", errors.New("runner is not bound to provisioning receipt")
	}
	if err := verifyPlatformCanaryRunner(runtime.FS, runner.Path, runner.SHA256); err != nil {
		return managedworker.CanaryReceipt{}, "", err
	}
	workerProfile, err := selectCanaryWorkerProfile(provisioning)
	if err != nil {
		return managedworker.CanaryReceipt{}, "", err
	}
	receipt, err := managedworker.RunGoldenPathCanary(ctx, managedworker.CanaryRunRequest{
		CityPath:            cityPath,
		Environment:         environment,
		MaxWallTime:         options.maxWallTime,
		ProvisioningReceipt: provisioning,
		Runner:              runner,
		RunID:               options.runID,
	}, managedworker.CanaryRunDeps{
		FS:  runtime.FS,
		Now: runtime.Now,
		RunScenario: func(scenarioContext context.Context, scenario string) (managedworker.CanaryScenarioEvidence, error) {
			return runtime.RunScenario(scenarioContext, platformCanaryScenarioCall{
				BaseCommit:     options.baseCommit,
				CityPath:       cityPath,
				LauncherSource: options.launcherSource,
				RunID:          options.runID,
				RunnerPath:     options.runnerPath,
				RunnerSHA256:   options.runnerSHA256,
				Scenario:       scenario,
				ScratchRoot:    options.scratchRoot,
				WorkerProfile:  workerProfile,
			})
		},
	})
	if err != nil {
		return managedworker.CanaryReceipt{}, "", err
	}
	return receipt, managedworker.CanaryReceiptPath(cityPath), nil
}

func defaultPlatformCanaryRuntime() platformCanaryRuntime {
	return platformCanaryRuntime{
		FS:          fsys.OSFS{},
		LoadInputs:  loadPlatformCanaryInputs,
		Now:         time.Now,
		ResolveCity: resolveCity,
		RunScenario: runPlatformCanaryScenarioCommand,
	}
}

func loadPlatformCanaryInputs(ctx context.Context, cityPath string) (managedworker.ProvisioningReceipt, managedworker.CanaryEnvironment, error) {
	cfg, provenance, err := loadCityConfigWithProvenance(cityPath, io.Discard)
	if err != nil {
		return managedworker.ProvisioningReceipt{}, managedworker.CanaryEnvironment{}, err
	}
	permissionRevision := configRevisionForLoadedCity(cityPath, cfg, provenance)
	gate := newManagedProductDispatchGate(cityPath, cfg, permissionRevision, nil)
	environment, err := gate.observeLiveEnvironment(ctx, cityPath, permissionRevision)
	if err != nil {
		return managedworker.ProvisioningReceipt{}, managedworker.CanaryEnvironment{}, err
	}
	data, err := os.ReadFile(managedworker.ProvisioningReceiptPath(cityPath))
	if err != nil {
		return managedworker.ProvisioningReceipt{}, managedworker.CanaryEnvironment{}, fmt.Errorf("read provisioning receipt: %w", err)
	}
	provisioning, err := managedworker.LoadProvisioningReceipt(data)
	if err != nil {
		return managedworker.ProvisioningReceipt{}, managedworker.CanaryEnvironment{}, err
	}
	return provisioning, environment, nil
}

func runPlatformCanaryScenarioCommand(ctx context.Context, call platformCanaryScenarioCall) (managedworker.CanaryScenarioEvidence, error) {
	if err := verifyPlatformCanaryRunner(fsys.OSFS{}, call.RunnerPath, call.RunnerSHA256); err != nil {
		return managedworker.CanaryScenarioEvidence{}, err
	}
	command := exec.CommandContext(ctx, call.RunnerPath,
		"--scenario", call.Scenario,
		"--run-id", call.RunID,
		"--target-city", call.CityPath,
		"--launcher-source", call.LauncherSource,
		"--base-commit", call.BaseCommit,
		"--scratch-root", call.ScratchRoot,
	)
	command.Dir = call.LauncherSource
	profileJSON, err := json.Marshal(call.WorkerProfile)
	if err != nil {
		return managedworker.CanaryScenarioEvidence{}, fmt.Errorf("encode canary worker profile: %w", err)
	}
	command.Env = overlayEnvironment(os.Environ(), map[string]string{
		"GCT_CANARY_WORKER_PROFILE_JSON": string(profileJSON),
	})
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return managedworker.CanaryScenarioEvidence{}, fmt.Errorf("runner scenario %q: %w: %s", call.Scenario, err, boundedCanaryDiagnostic(stderr.String()))
	}
	evidence, err := decodePlatformCanaryScenarioEvidence(stdout.Bytes())
	if err != nil {
		return managedworker.CanaryScenarioEvidence{}, fmt.Errorf("runner scenario %q output: %w", call.Scenario, err)
	}
	return evidence, nil
}

func selectCanaryWorkerProfile(receipt managedworker.ProvisioningReceipt) (managedworker.WorkerProfile, error) {
	var matches []managedworker.WorkerProfile
	for _, profile := range receipt.Profiles {
		if strings.HasSuffix(profile.Name, "/gc.implementation-worker") {
			matches = append(matches, profile)
		}
	}
	if len(matches) != 1 {
		return managedworker.WorkerProfile{}, fmt.Errorf("canary requires exactly one gc.implementation-worker profile, got %d", len(matches))
	}
	if len(matches[0].Environment) == 0 || len(matches[0].Toolchains) == 0 {
		return managedworker.WorkerProfile{}, errors.New("canary requires a v2 implementation-worker profile with environment and toolchains")
	}
	return matches[0], nil
}

func validatePlatformCanaryRuntime(runtime platformCanaryRuntime) error {
	for name, present := range map[string]bool{
		"filesystem":      runtime.FS != nil,
		"input loader":    runtime.LoadInputs != nil,
		"clock":           runtime.Now != nil,
		"city resolver":   runtime.ResolveCity != nil,
		"scenario runner": runtime.RunScenario != nil,
	} {
		if !present {
			return fmt.Errorf("platform canary %s is required", name)
		}
	}
	return nil
}

func validatePlatformCanaryOptions(options platformCanaryOptions) error {
	for name, value := range map[string]string{
		"run-id":          options.runID,
		"runner":          options.runnerPath,
		"runner-sha256":   options.runnerSHA256,
		"launcher-source": options.launcherSource,
		"base-commit":     options.baseCommit,
		"scratch-root":    options.scratchRoot,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("platform canary %s is required", name)
		}
	}
	for name, path := range map[string]string{
		"runner":          options.runnerPath,
		"launcher-source": options.launcherSource,
		"scratch-root":    options.scratchRoot,
	} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fmt.Errorf("platform canary %s must be a clean absolute path: %q", name, path)
		}
	}
	commit, err := hex.DecodeString(options.baseCommit)
	if err != nil || len(commit) != 20 || options.baseCommit != strings.ToLower(options.baseCommit) {
		return errors.New("platform canary base-commit must be a full lowercase Git commit")
	}
	if _, err := hex.DecodeString(options.runnerSHA256); err != nil || len(options.runnerSHA256) != sha256.Size*2 || options.runnerSHA256 != strings.ToLower(options.runnerSHA256) {
		return errors.New("platform canary runner-sha256 must be a lowercase SHA-256")
	}
	if options.maxWallTime <= 0 {
		return errors.New("platform canary max-wall-time must be positive")
	}
	return nil
}

func verifyPlatformCanaryRunner(filesystem fsys.FS, path, expectedSHA256 string) error {
	info, err := filesystem.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect canary runner: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("canary runner must be an executable regular file: %s", path)
	}
	data, err := filesystem.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read canary runner: %w", err)
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	if actual != expectedSHA256 {
		return fmt.Errorf("runner sha256 mismatch: got %s want %s", actual, expectedSHA256)
	}
	return nil
}

func encodePlatformCanaryScenarioEvidence(evidence managedworker.CanaryScenarioEvidence) ([]byte, error) {
	return json.Marshal(evidence)
}

func decodePlatformCanaryScenarioEvidence(data []byte) (managedworker.CanaryScenarioEvidence, error) {
	var evidence managedworker.CanaryScenarioEvidence
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evidence); err != nil {
		return managedworker.CanaryScenarioEvidence{}, fmt.Errorf("decode scenario evidence: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return managedworker.CanaryScenarioEvidence{}, errors.New("decode scenario evidence: trailing JSON value")
		}
		return managedworker.CanaryScenarioEvidence{}, fmt.Errorf("decode scenario evidence trailer: %w", err)
	}
	return evidence, nil
}

func boundedCanaryDiagnostic(value string) string {
	const limit = 4096
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/agentutil"
	"github.com/gastownhall/gascity/internal/api"
	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/managedworker"
	"github.com/gastownhall/gascity/internal/platforminstall"
	"github.com/gastownhall/gascity/internal/shellquote"
)

type taskCheckPathResolver func(startCandidate, *config.City) (string, error)

// configureManagedWorkerPreflight attaches the production gate to every start.
// Receipt membership is the only managed/unmanaged classifier: no provider,
// role, rig, or template name is hard-coded here.
func configureManagedWorkerPreflight(
	item *preparedStart,
	cityPath string,
	cfg *config.City,
	permissionRevision string,
	checkPathResolver taskCheckPathResolver,
	probeOverride *managedworker.Probes,
) {
	if item == nil || strings.TrimSpace(cityPath) == "" {
		return
	}
	// The resolved template identity retains import bindings (for example
	// hpfetcher/gc.implementation-worker), unlike provider names.
	profileName := managedWorkerProfileName(item.candidate, cfg)
	commandArgv := append([]string(nil), item.managedWorkerArgv...)
	if len(commandArgv) == 0 {
		// Compatibility for direct preparedStart fixtures. Production
		// buildPreparedStart always captures the stable pre-resume argv.
		commandArgv = shellquote.Split(strings.TrimSpace(item.cfg.Command))
	}
	workDir := item.cfg.WorkDir
	receiptPath := managedworker.ProvisioningReceiptPath(cityPath)
	item.preflight = func(ctx context.Context) error {
		receiptBytes, err := os.ReadFile(receiptPath)
		if errors.Is(err, os.ErrNotExist) {
			// Unprovisioned control-plane and legacy sessions remain outside this
			// gate. The receipt-checked product dispatch gate makes absence a
			// routing refusal before managed-product sessions reach this point.
			return nil
		}
		if err != nil {
			return &managedworker.Failure{Profile: profileName, Err: fmt.Errorf("read provisioning receipt %q: %w", receiptPath, err)}
		}
		receipt, err := managedworker.LoadProvisioningReceipt(receiptBytes)
		if err != nil {
			return &managedworker.Failure{Profile: profileName, Err: err}
		}
		expected, managed := receipt.Profile(profileName)
		if !managed {
			return nil
		}
		if len(commandArgv) == 0 {
			return &managedworker.Failure{Profile: profileName, Err: errors.New("resolved provider argv is empty")}
		}
		if checkPathResolver == nil {
			return &managedworker.Failure{Profile: profileName, Err: fmt.Errorf("%s is unavailable from the controller work snapshot", beadmeta.CheckPathMetadataKey)}
		}
		checkPath, err := checkPathResolver(item.candidate, cfg)
		if err != nil {
			return &managedworker.Failure{Profile: profileName, Err: err}
		}
		if strings.TrimSpace(checkPath) == "" {
			return &managedworker.Failure{Profile: profileName, Err: fmt.Errorf("%s is not stamped on the driving work bead", beadmeta.CheckPathMetadataKey)}
		}

		observed := expected
		observed.Argv = append([]string(nil), commandArgv...)
		observed.CheckPath.Path = filepath.Clean(checkPath)
		probes := defaultManagedWorkerPreflightProbes(workDir)
		if probeOverride != nil {
			probes = *probeOverride
		}
		_, err = managedworker.Preflight(ctx, managedworker.PreflightRequest{
			Receipt:            receiptBytes,
			ProfileName:        profileName,
			ObservedProfile:    observed,
			PermissionRevision: permissionRevision,
			CheckPath:          checkPath,
		}, probes)
		if err != nil {
			return &managedworker.Failure{Profile: profileName, Err: err}
		}
		return nil
	}
}

func managedWorkerProfileName(candidate startCandidate, cfg *config.City) string {
	if agent := findAgentByTemplate(cfg, candidate.logicalTemplate(cfg)); agent != nil {
		return agent.QualifiedName()
	}
	template := strings.TrimSpace(candidate.tp.TemplateName)
	rig := strings.TrimSpace(candidate.tp.RigName)
	if rig != "" && template != "" && !strings.Contains(template, "/") {
		return rig + "/" + template
	}
	return template
}

func defaultManagedWorkerPreflightProbes(workDir string) managedworker.Probes {
	return managedworker.Probes{
		ReadFile:        os.ReadFile,
		InspectProvider: platforminstall.VerifyProviderPin,
		ProbeReadiness: func(ctx context.Context, provider string) error {
			if !api.SupportsProviderReadiness(provider) {
				return fmt.Errorf("provider %q has no independent readiness probe", provider)
			}
			results, err := api.ProbeProviders(ctx, []string{provider}, true)
			if err != nil {
				return err
			}
			result, ok := results[provider]
			if !ok {
				return fmt.Errorf("provider %q returned no readiness result", provider)
			}
			if result.Status != api.ProbeStatusConfigured {
				return fmt.Errorf("status=%s detail=%s", result.Status, strings.TrimSpace(result.Detail))
			}
			return nil
		},
		ProbeSigner: func(ctx context.Context, identity string) error {
			return probeManagedWorkerSigner(ctx, workDir, identity)
		},
	}
}

func probeManagedWorkerSigner(ctx context.Context, workDir, identity string) error {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return errors.New("signer identity is empty")
	}
	workDir = strings.TrimSpace(workDir)
	if !filepath.IsAbs(workDir) || filepath.Clean(workDir) != workDir {
		return fmt.Errorf("signer work directory must be a clean absolute path: %q", workDir)
	}
	if info, err := os.Stat(workDir); err != nil || !info.IsDir() {
		if err != nil {
			return fmt.Errorf("signer work directory %q: %w", workDir, err)
		}
		return fmt.Errorf("signer work directory %q is not a directory", workDir)
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	configured, err := exec.CommandContext(probeCtx, "git", "-C", workDir, "config", "--get", "user.signingkey").CombinedOutput()
	if err != nil {
		return fmt.Errorf("git signing key: %w", err)
	}
	if got := strings.TrimSpace(string(configured)); !strings.EqualFold(got, identity) {
		return fmt.Errorf("git signing key mismatch: got %q want %q", got, identity)
	}
	secret, err := exec.CommandContext(probeCtx, "gpg", "--batch", "--with-colons", "--list-secret-keys", identity).CombinedOutput()
	if err != nil {
		return fmt.Errorf("secret signing key unavailable: %w", err)
	}
	for _, line := range strings.Split(string(secret), "\n") {
		if strings.HasPrefix(line, "sec:") {
			return nil
		}
	}
	return errors.New("secret signing key probe returned no secret key")
}

// newTaskCheckPathResolver binds a candidate to the controller's single tick
// snapshot. It supports both already-assigned work and routed-unassigned pool
// demand, and fails closed if matching work disagrees on its stamped path.
func newTaskCheckPathResolver(work []beads.Bead) taskCheckPathResolver {
	snapshot := append([]beads.Bead(nil), work...)
	return func(candidate startCandidate, cfg *config.City) (string, error) {
		identities := make(map[string]struct{})
		for _, identity := range taskWorkDirAssignees(candidate, cfg) {
			if identity = strings.TrimSpace(identity); identity != "" {
				identities[identity] = struct{}{}
			}
		}
		profile := normalizeAgentTemplateIdentity(cfg, managedWorkerProfileName(candidate, cfg))
		var selected string
		for _, bead := range snapshot {
			if bead.Status != "open" && bead.Status != "in_progress" {
				continue
			}
			matched := false
			if assignee := strings.TrimSpace(bead.Assignee); assignee != "" {
				_, matched = identities[assignee]
			}
			if !matched {
				route := normalizeAgentTemplateIdentity(cfg, agentutil.NormalizePoolRouteTarget(cfg, routedToOrLegacyWorkflowTarget(bead)))
				matched = route != "" && agentTemplateIdentitiesEquivalent(cfg, route, profile)
			}
			if !matched {
				continue
			}
			path := strings.TrimSpace(bead.Metadata[beadmeta.CheckPathMetadataKey])
			if path == "" {
				return "", fmt.Errorf("matching work bead %q has no %s", bead.ID, beadmeta.CheckPathMetadataKey)
			}
			if selected != "" && selected != path {
				return "", fmt.Errorf("conflicting %s values for %q: %q and %q", beadmeta.CheckPathMetadataKey, profile, selected, path)
			}
			selected = path
		}
		return selected, nil
	}
}

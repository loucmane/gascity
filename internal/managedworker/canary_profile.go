package managedworker

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
)

const (
	// CanaryReceiptSchemaV2 certifies exactly one explicit profile and kind.
	CanaryReceiptSchemaV2 = "gc.canary-receipt.v2"
	// CanaryScenarioCandidateLauncher proves the unsigned candidate-only path.
	CanaryScenarioCandidateLauncher = "candidate-launcher"
)

// CanaryProfile binds the selected resolved identity, contract kind and digest.
// It is separate from the full environment inventory, which is not a claim that
// every inventoried profile executed this canary.
type CanaryProfile struct {
	Name   string      `json:"name"`
	Kind   ProfileKind `json:"profile_kind"`
	SHA256 string      `json:"sha256"`
}

// SelectCanaryProfile has no role-name or provider-name heuristics. Callers must
// select the actual resolved profile and explicitly state the expected kind.
func SelectCanaryProfile(receipt ProvisioningReceipt, name string, kind ProfileKind) (WorkerProfile, error) {
	if name == "" || (kind != ProfileKindCandidate && kind != ProfileKindSigning) {
		return WorkerProfile{}, errors.New("canary requires an exact profile name and candidate or signing kind")
	}
	profile, ok := receipt.Profile(name)
	if !ok {
		return WorkerProfile{}, fmt.Errorf("canary profile %q is not declared", name)
	}
	if err := validateWorkerProfile(profile, true, true); err != nil {
		return WorkerProfile{}, err
	}
	if profile.EffectiveProfileKind() != kind {
		return WorkerProfile{}, errors.New("canary profile kind does not match selected profile")
	}
	return profile, nil
}

// RequiredCanaryScenariosFor returns a copy of the appropriate typed contract.
func RequiredCanaryScenariosFor(kind ProfileKind) ([]string, error) {
	switch kind {
	case ProfileKindSigning:
		return RequiredCanaryScenarios(), nil
	case ProfileKindCandidate:
		return []string{CanaryScenarioCandidateLauncher}, nil
	default:
		return nil, fmt.Errorf("unsupported canary profile kind %q", kind)
	}
}

// RequiredCandidateLauncherSteps excludes signing without weakening the signed
// nine-step contract. This is candidate evidence, never signed-delivery proof.
func RequiredCandidateLauncherSteps() []string {
	steps := make([]string, 0, len(requiredCleanLauncherSteps)-1)
	for _, step := range requiredCleanLauncherSteps {
		if step != "sign-candidate" {
			steps = append(steps, step)
		}
	}
	return steps
}

// ProfileCanaryReceiptPath keeps typed proofs out of the legacy city-wide slot.
// Hashing the full identity prevents path traversal and alias collisions.
func ProfileCanaryReceiptPath(cityPath, profileName string) string {
	return filepath.Join(cityPath, ".gc/runtime/canary/profiles", sha256Hex([]byte(profileName))+".json")
}

// CurrentCanaryReceiptPath selects storage only; publication validates first.
func CurrentCanaryReceiptPath(cityPath string, receipt CanaryReceipt) string {
	if receipt.Schema == CanaryReceiptSchemaV2 && receipt.Profile != nil {
		return ProfileCanaryReceiptPath(cityPath, receipt.Profile.Name)
	}
	return CanaryReceiptPath(cityPath)
}

func validateLegacyCanaryProfiles(receipt ProvisioningReceipt) error {
	for _, profile := range receipt.Profiles {
		if profile.ProfileKind != "" || profile.ControlPolicy != nil {
			return errors.New("typed profiles require a profile-scoped v2 canary")
		}
	}
	return nil
}

func validateCanaryProfile(provisioning ProvisioningReceipt, environment CanaryEnvironment, target *CanaryProfile) error {
	if target == nil {
		return errors.New("v2 canary profile is required")
	}
	profile, err := SelectCanaryProfile(provisioning, target.Name, target.Kind)
	if err != nil {
		return err
	}
	if target.SHA256 != profile.WorkerProfileSHA256 {
		return errors.New("canary profile digest differs from provisioning receipt")
	}
	if err := validateSHA256("profile.sha256", target.SHA256); err != nil {
		return err
	}
	// Inventory is exact, but only target is certified. Do not let a selected
	// profile's proof silently certify another inventoried profile.
	if len(environment.Profiles) != len(provisioning.Profiles) {
		return errors.New("canary environment profile inventory differs from provisioning receipt")
	}
	providers := make(map[string]bool)
	for _, declared := range provisioning.Profiles {
		found := false
		for _, pin := range environment.Profiles {
			if pin.Name == declared.Name && pin.SHA256 == declared.WorkerProfileSHA256 {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("canary environment profile %q differs from provisioning receipt", declared.Name)
		}
		found = false
		for _, pin := range environment.Providers {
			if reflect.DeepEqual(pin, declared.Provider) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("canary environment provider %q differs from provisioning receipt", declared.Provider.Name)
		}
		providers[declared.Provider.Name] = true
	}
	if len(providers) != len(environment.Providers) {
		return errors.New("canary environment provider inventory differs from provisioning receipt")
	}
	if environment.Pack != provisioning.Pack || environment.Rules != provisioning.Rules ||
		environment.TemplateCommit != provisioning.TemplateCommit || environment.PermissionRevision != provisioning.PermissionRevision ||
		environment.ProvisioningReceiptSHA256 != provisioning.ReceiptSHA256 {
		return errors.New("canary environment differs from provisioning receipt")
	}
	return nil
}

func validateTypedCanaryScenarios(receipt CanaryReceipt) error {
	want, err := RequiredCanaryScenariosFor(receipt.Profile.Kind)
	if err != nil {
		return err
	}
	sort.Strings(want)
	if len(receipt.Scenarios) != len(want) {
		return errors.New("canary scenarios do not match selected profile kind")
	}
	for i, name := range want {
		if receipt.Scenarios[i].Name != name {
			return errors.New("canary scenarios do not match selected profile kind")
		}
		if receipt.Scenarios[i].AttentionLatencyCycles > 1 {
			return errors.New("canary attention exceeds one reconciliation cycle")
		}
	}
	return nil
}

// VerifyProfileCanaryReceipt is the explicit, exact-profile consumer. Passing a
// candidate receipt to a signing caller (or conversely) always refuses. The old
// unscoped dispatch verifier intentionally accepts neither form of v2 receipt.
func VerifyProfileCanaryReceipt(data []byte, observed CanaryEnvironment, name string, kind ProfileKind) (CanaryReceipt, error) {
	receipt, err := verifyCanaryFingerprint(data, observed)
	if err != nil {
		return CanaryReceipt{}, err
	}
	if receipt.Schema != CanaryReceiptSchemaV2 || receipt.Profile == nil {
		return CanaryReceipt{}, newDispatchRefusal("profile", "profile-scoped v2 receipt", "legacy unscoped receipt")
	}
	if receipt.Profile.Name != name {
		return CanaryReceipt{}, newDispatchRefusal("profile.name", name, receipt.Profile.Name)
	}
	if receipt.Profile.Kind != kind {
		return CanaryReceipt{}, newDispatchRefusal("profile.profile_kind", string(kind), string(receipt.Profile.Kind))
	}
	return receipt, nil
}

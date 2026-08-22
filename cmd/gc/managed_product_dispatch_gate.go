package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/events"
	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/gastownhall/gascity/internal/managedworker"
	"github.com/gastownhall/gascity/internal/platforminstall"
)

type managedProductDispatchGate struct {
	cityPath           string
	cfg                *config.City
	permissionRevision string
	recorder           events.Recorder
	readFile           func(string) ([]byte, error)
	observe            func(context.Context, string, string) (managedworker.CanaryEnvironment, error)
}

func newManagedProductDispatchGate(cityPath string, cfg *config.City, permissionRevision string, recorder events.Recorder) *managedProductDispatchGate {
	gate := &managedProductDispatchGate{
		cityPath:           cityPath,
		cfg:                cfg,
		permissionRevision: permissionRevision,
		recorder:           recorder,
		readFile:           os.ReadFile,
	}
	gate.observe = gate.observeLiveEnvironment
	return gate
}

func (gate *managedProductDispatchGate) Verify(rigName string) error {
	if gate == nil || !managedProductRig(gate.cfg, rigName) {
		return nil
	}
	receiptPath := managedworker.CanaryReceiptPath(gate.cityPath)
	receiptData, err := gate.readFile(receiptPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = managedworker.RefuseDispatch("receipt", "present", "missing")
		} else {
			err = managedworker.RefuseDispatch("receipt", "readable", err.Error())
		}
		gate.recordRefusal(rigName, err)
		return err
	}
	observed, err := gate.observe(context.Background(), gate.cityPath, gate.permissionRevision)
	if err != nil {
		var refusal *managedworker.DispatchRefusal
		if !errors.As(err, &refusal) {
			err = managedworker.RefuseDispatch("live_fingerprint", "observable", err.Error())
		}
		gate.recordRefusal(rigName, err)
		return err
	}
	_, err = managedworker.VerifyCanaryReceipt(receiptData, observed)
	if err != nil {
		gate.recordRefusal(rigName, err)
	}
	return err
}

func managedProductRig(cfg *config.City, rigName string) bool {
	if cfg == nil {
		return false
	}
	for _, rig := range cfg.Rigs {
		if rig.Name == rigName {
			return rig.ManagedProduct
		}
	}
	return false
}

func (gate *managedProductDispatchGate) recordRefusal(rigName string, err error) {
	if gate.recorder == nil {
		return
	}
	payload, _ := json.Marshal(err)
	gate.recorder.Record(events.Event{
		Type:    events.ManagedProductDispatchRefused,
		Actor:   eventActor(),
		Subject: rigName,
		Message: err.Error(),
		Payload: payload,
	})
}

func (gate *managedProductDispatchGate) observeLiveEnvironment(ctx context.Context, cityPath, permissionRevision string) (managedworker.CanaryEnvironment, error) {
	manifestData, err := gate.readFile(platforminstall.DefaultManifestPath(cityPath))
	if err != nil {
		return managedworker.CanaryEnvironment{}, managedworker.RefuseDispatch("platform_manifest", "present and readable", err.Error())
	}
	manifest, err := platforminstall.LoadManifest(manifestData)
	if err != nil {
		return managedworker.CanaryEnvironment{}, managedworker.RefuseDispatch("platform_manifest", "valid", err.Error())
	}
	report, err := platforminstall.InspectIntegrity(ctx, manifest)
	if err != nil {
		return managedworker.CanaryEnvironment{}, managedworker.RefuseDispatch("platform_integrity", "observable", err.Error())
	}
	if len(report.Drifts) > 0 {
		drift := report.Drifts[0]
		return managedworker.CanaryEnvironment{}, managedworker.RefuseDispatch("platform."+drift.Field, drift.Expected, drift.Actual)
	}
	if manifest.Activation == nil {
		return managedworker.CanaryEnvironment{}, managedworker.RefuseDispatch("gc_binary.commit", "activation-pinned", "missing")
	}

	provisioningData, err := gate.readFile(managedworker.ProvisioningReceiptPath(cityPath))
	if err != nil {
		return managedworker.CanaryEnvironment{}, managedworker.RefuseDispatch("provisioning_receipt", "present and readable", err.Error())
	}
	provisioning, err := managedworker.LoadProvisioningReceipt(provisioningData)
	if err != nil {
		return managedworker.CanaryEnvironment{}, managedworker.RefuseDispatch("provisioning_receipt", "valid", err.Error())
	}
	if permissionRevision != provisioning.PermissionRevision {
		return managedworker.CanaryEnvironment{}, managedworker.RefuseDispatch("permission_revision", provisioning.PermissionRevision, permissionRevision)
	}
	if err := verifyDispatchFilePin(gate.readFile, "rules", provisioning.Rules); err != nil {
		return managedworker.CanaryEnvironment{}, err
	}

	providerPins := make(map[string]platforminstall.ProviderPin)
	if manifest.Integrity != nil {
		for _, pin := range manifest.Integrity.Providers {
			providerPins[pin.Name] = pin
		}
	}
	profiles := make([]managedworker.ProfilePin, 0, len(provisioning.Profiles))
	providers := make(map[string]platforminstall.ProviderPin)
	for _, profile := range provisioning.Profiles {
		if err := verifyDispatchFilePin(gate.readFile, "profiles["+profile.Name+"].check_path", profile.CheckPath); err != nil {
			return managedworker.CanaryEnvironment{}, err
		}
		pin, ok := providerPins[profile.Provider.Name]
		if !ok {
			return managedworker.CanaryEnvironment{}, managedworker.RefuseDispatch("providers["+profile.Provider.Name+"]", "platform-pinned", "missing")
		}
		if !providerPinsEqual(profile.Provider, pin) {
			return managedworker.CanaryEnvironment{}, managedworker.RefuseDispatch("providers["+profile.Provider.Name+"]", fmt.Sprintf("%+v", profile.Provider), fmt.Sprintf("%+v", pin))
		}
		digest, digestErr := managedworker.WorkerProfileDigest(profile)
		if digestErr != nil {
			return managedworker.CanaryEnvironment{}, managedworker.RefuseDispatch("profiles["+profile.Name+"].sha256", "valid", digestErr.Error())
		}
		profiles = append(profiles, managedworker.ProfilePin{Name: profile.Name, SHA256: digest})
		providers[pin.Name] = pin
	}

	if manifest.Integrity == nil || !containsRepositoryCommit(manifest.Integrity.Repositories, provisioning.TemplateCommit) {
		return managedworker.CanaryEnvironment{}, managedworker.RefuseDispatch("template_commit", "platform-pinned", provisioning.TemplateCommit)
	}
	if !containsRepositoryCommit(manifest.Integrity.Repositories, provisioning.Pack.Commit) {
		return managedworker.CanaryEnvironment{}, managedworker.RefuseDispatch("pack.commit", "platform-pinned", provisioning.Pack.Commit)
	}
	providerList := make([]platforminstall.ProviderPin, 0, len(providers))
	for _, pin := range providers {
		providerList = append(providerList, pin)
	}
	sort.Slice(providerList, func(i, j int) bool { return providerList[i].Name < providerList[j].Name })
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	return managedworker.CanaryEnvironment{
		GCBinary:                  managedworker.BinaryPin{Commit: manifest.Activation.ExpectedCommit, SHA256: manifest.Core.SHA256},
		Pack:                      provisioning.Pack,
		PermissionRevision:        permissionRevision,
		Profiles:                  profiles,
		Providers:                 providerList,
		ProvisioningReceiptSHA256: provisioning.ReceiptSHA256,
		Rules:                     provisioning.Rules,
		TemplateCommit:            provisioning.TemplateCommit,
	}, nil
}

func verifyDispatchFilePin(readFile func(string) ([]byte, error), field string, pin managedworker.FilePin) error {
	data, err := readFile(pin.Path)
	if err != nil {
		return managedworker.RefuseDispatch(field+".sha256", pin.SHA256, err.Error())
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	if !strings.EqualFold(actual, pin.SHA256) {
		return managedworker.RefuseDispatch(field+".sha256", pin.SHA256, actual)
	}
	return nil
}

func providerPinsEqual(left, right platforminstall.ProviderPin) bool {
	return left.Name == right.Name && left.Path == right.Path && left.ResolvedPath == right.ResolvedPath &&
		left.SHA256 == right.SHA256 && left.Version == right.Version && strings.Join(left.VersionArgs, "\x00") == strings.Join(right.VersionArgs, "\x00")
}

func containsRepositoryCommit(repositories []platforminstall.GitPin, commit string) bool {
	for _, repository := range repositories {
		if repository.Commit == commit {
			return true
		}
	}
	return false
}

func configRevisionForLoadedCity(cityPath string, cfg *config.City, provenance *config.Provenance) string {
	return config.Revision(fsys.OSFS{}, provenance, cfg, filepath.Clean(cityPath))
}

package platforminstall

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/gastownhall/gascity/internal/providerallowlist"
)

// VerifyCityProviderAllowlist enforces an installed city's signed provider
// delegation before a worker start. Cities without a manifest or delegation
// retain their existing behavior. A required approval never falls back to
// legacy behavior when the list or receipt disappears (ga-r9lv).
func VerifyCityProviderAllowlist(city, permissionRevision string) error {
	if city == "" {
		return nil
	}
	data, err := os.ReadFile(DefaultManifestPath(city))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("provider allowlist manifest: %w", err)
	}
	manifest, err := LoadManifest(data)
	if err != nil {
		return fmt.Errorf("provider allowlist manifest: %w", err)
	}
	if manifest.CityPath != city {
		return fmt.Errorf("provider allowlist manifest city mismatch")
	}
	return verifyManifestProviderAllowlist(manifest, permissionRevision)
}

func approvedProviders(manifest Manifest, revision string) (providerallowlist.List, error) {
	ref := manifest.Integrity.ProviderAllowlist
	if err := ref.Validate(manifest.CityPath); err != nil {
		return providerallowlist.List{}, err
	}
	receipt, err := os.ReadFile(providerallowlist.ReceiptPath(manifest.CityPath))
	if err != nil {
		return providerallowlist.List{}, fmt.Errorf("provider allowlist approval receipt: %w", err)
	}
	list, err := providerallowlist.ReadApproved(ref.Path, receipt, revision)
	if err != nil {
		return providerallowlist.List{}, err
	}
	if err := list.Verify(ref.Bindings); err != nil {
		return providerallowlist.List{}, err
	}
	return list, nil
}

func verifyManifestProviderAllowlist(manifest Manifest, revision string) error {
	if manifest.Integrity == nil || manifest.Integrity.ProviderAllowlist == nil {
		return nil
	}
	_, err := approvedProviders(manifest, revision)
	return err
}

// VerifyAllowlistedProvider checks a receipt's direct CLI provider pin against
// the delegated active entry, or a closed wrapper against its signed source
// identity with the approved CLI component. VerifyProviderPin still compares
// the complete reported version exactly against the derived receipt pin.
func VerifyAllowlistedProvider(ctx context.Context, manifest Manifest, pin ProviderPin) error {
	if manifest.Integrity == nil || manifest.Integrity.ProviderAllowlist == nil {
		return fmt.Errorf("provider %q is not allowlisted", pin.Path)
	}
	list, err := approvedProviders(manifest, "")
	if err != nil {
		return err
	}
	if closedWorkerVersion.MatchString(pin.Version) {
		for _, signed := range manifest.Integrity.Providers {
			if signed.Key() != pin.Key() {
				continue
			}
			effective, err := effectiveProviderPin(manifest, signed)
			if err != nil {
				return err
			}
			if !closedWorkerVersion.MatchString(signed.Version) || !sameProviderPin(pin, effective) {
				return fmt.Errorf("closed worker source/policy or CLI pin drift for %q", pin.Path)
			}
			return VerifyProviderPin(ctx, pin)
		}
		return fmt.Errorf("closed worker %q is not platform-pinned", pin.Path)
	}
	for _, binding := range manifest.Integrity.ProviderAllowlist.Bindings {
		if binding.Path != pin.Path {
			continue
		}
		entry, ok := list.Active(binding.Provider)
		if !ok || entry.Path != pin.ResolvedPath || entry.SHA256 != pin.SHA256 {
			return fmt.Errorf("provider allowlist pin drift for %q", pin.Path)
		}
		return VerifyProviderPin(ctx, pin)
	}
	return fmt.Errorf("provider %q has no allowlist binding", pin.Path)
}

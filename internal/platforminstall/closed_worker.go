package platforminstall

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
)

var closedWorkerVersion = regexp.MustCompile(`^(gct-claude-(signing|candidate|template-candidate|project-candidate)-worker) 2 sources_sha256=([0-9a-f]{64}) cli_sha256=([0-9a-f]{64})$`)

// effectiveProviderPin derives only the CLI component of a signed closed-worker
// identity. Entrypoint bytes, paths, arguments and every source/policy pin remain
// signed authority; the active CLI digest comes from the approved city receipt.
func effectiveProviderPin(manifest Manifest, pin ProviderPin) (ProviderPin, error) {
	parts := closedWorkerVersion.FindStringSubmatch(pin.Version)
	if parts == nil || manifest.Integrity == nil || manifest.Integrity.ProviderAllowlist == nil {
		return pin, nil
	}
	if filepath.Base(pin.Path) != parts[1] || !slices.Equal(pin.VersionArgs, []string{"--version"}) {
		return ProviderPin{}, fmt.Errorf("closed worker entrypoint/version arguments mismatch")
	}
	bound := false
	for _, binding := range manifest.Integrity.ProviderAllowlist.Bindings {
		if binding.Provider == "claude" && binding.Path == filepath.Join(filepath.Dir(manifest.CityPath), "bin", "claude") {
			bound = true
		}
	}
	if !bound {
		return ProviderPin{}, fmt.Errorf("closed worker requires canonical Claude allowlist binding")
	}
	list, err := approvedProviders(manifest, "")
	if err != nil {
		return ProviderPin{}, err
	}
	entry, ok := list.Active("claude")
	if !ok {
		return ProviderPin{}, fmt.Errorf("closed worker requires active Claude")
	}
	pin.Version = parts[1] + " 2 sources_sha256=" + parts[3] + " cli_sha256=" + entry.SHA256
	return pin, nil
}

func sameProviderPin(a, b ProviderPin) bool {
	return a.Name == b.Name && a.Path == b.Path && a.ResolvedPath == b.ResolvedPath &&
		a.SHA256 == b.SHA256 && a.Version == b.Version && slices.Equal(a.VersionArgs, b.VersionArgs)
}

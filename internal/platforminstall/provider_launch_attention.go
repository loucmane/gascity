package platforminstall

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Attention entrypoints use the same approved-native launch contract as closed
// workers, with the complete local source chain signed as ordinary file pins.
// Names select a reviewed adapter contract; they never grant authority by
// themselves. Every selected file must match the installed manifest.
func verifyAttentionLaunchChain(manifest Manifest, executable string) (bool, error) {
	name := filepath.Base(executable)
	if name != "claude-attention" && name != "codex-attention" {
		return false, nil
	}
	provider := strings.TrimSuffix(name, "-attention")
	canonical := filepath.Join(filepath.Dir(manifest.CityPath), "bin", provider)
	bound := false
	for _, binding := range manifest.Integrity.ProviderAllowlist.Bindings {
		if binding.Provider == provider && binding.Path == canonical {
			bound = true
		}
	}
	if !bound || filepath.Base(filepath.Dir(executable)) != "bin" {
		return true, fmt.Errorf("provider allowlist attention chain requires canonical provider binding and bin entrypoint")
	}
	root := filepath.Dir(filepath.Dir(executable))
	for _, path := range []string{executable, filepath.Join(root, "lib", "gct_attention_launch.py"), filepath.Join(root, "lib", "gct_provider_allowlist.py")} {
		var pin *FilePin
		for i := range manifest.Integrity.Files {
			if manifest.Integrity.Files[i].Path == path {
				pin = &manifest.Integrity.Files[i]
				break
			}
		}
		if pin == nil {
			return true, fmt.Errorf("provider allowlist attention chain file is unsigned: %q", path)
		}
		info, err := os.Lstat(path)
		if err != nil {
			return true, fmt.Errorf("provider allowlist attention chain: %w", err)
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || resolved != path || !info.Mode().IsRegular() {
			return true, fmt.Errorf("provider allowlist attention chain requires direct regular files: %q", path)
		}
		if info.Mode().Perm() != os.FileMode(pin.Mode) || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || (path == executable && info.Mode().Perm()&0o111 == 0) {
			return true, fmt.Errorf("provider allowlist attention chain mode drift: %q", path)
		}
		digest, err := digestRegularFile(path)
		if err != nil || digest != pin.SHA256 {
			return true, fmt.Errorf("provider allowlist attention chain bytes drift: %q", path)
		}
	}
	return true, nil
}

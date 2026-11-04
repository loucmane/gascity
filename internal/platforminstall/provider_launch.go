package platforminstall

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gastownhall/gascity/internal/shellquote"
	"github.com/google/shlex"
)

// ResolveCityProviderLaunch binds the actual fresh/resume command to the city's
// approved executable or signed closed-worker chain. Legacy commands are returned
// byte-for-byte. Direct native commands enter a signed launcher that verifies
// and executes one retained descriptor at the runtime boundary (ga-r9lv).
func ResolveCityProviderLaunch(ctx context.Context, city, revision, command string) (string, error) {
	manifest, delegated, err := cityProviderManifest(city)
	if err != nil || !delegated {
		return command, err
	}
	list, err := approvedProviders(manifest, revision)
	if err != nil {
		return "", err
	}
	argv, err := shlex.Split(command)
	if err != nil || len(argv) == 0 || !filepath.IsAbs(argv[0]) || filepath.Clean(argv[0]) != argv[0] {
		return "", fmt.Errorf("provider allowlist launch requires an absolute approved executable")
	}
	// Canonical commands may be gated again by the session boundary or stored
	// as resume commands. Reconstruct the same signed chain; never grant general
	// permission to launch a Python interpreter or a caller-selected city.
	if len(argv) >= 5 && argv[0] == "/usr/bin/python3" && argv[1] == "-I" && argv[4] == manifest.CityPath &&
		filepath.IsAbs(argv[2]) && filepath.Clean(argv[2]) == argv[2] && filepath.Base(argv[2]) == "gct_attention_launch.py" && filepath.Base(filepath.Dir(argv[2])) == "lib" &&
		(argv[3] == "claude" || argv[3] == "codex" || argv[3] == "native:claude" || argv[3] == "native:codex") {
		root := filepath.Dir(filepath.Dir(argv[2]))
		wrapper := filepath.Join(root, "bin", strings.TrimPrefix(argv[3], "native:")+"-attention")
		if _, err := verifyAttentionLaunchChain(manifest, wrapper); err != nil {
			return "", err
		}
		return shellquote.Join(argv), nil
	}
	for _, binding := range manifest.Integrity.ProviderAllowlist.Bindings {
		entry, ok := list.Active(binding.Provider)
		if ok && (argv[0] == binding.Path || argv[0] == entry.Path) {
			// Returning entry.Path would reopen it in the runtime after this
			// check. Enter the pinned helper instead; it opens, verifies and
			// executes the same inode even if the pathname is then replaced.
			return nativeProviderLaunch(manifest, binding.Provider, argv[1:])
		}
	}
	if supported, err := verifyAttentionLaunchChain(manifest, argv[0]); supported {
		if err != nil {
			return "", err
		}
		// Enter the signed delegated branch with the fixed interpreter and city.
		// The shell entrypoint's env-based shebang must not resolve through the
		// session's PATH, and legacy caller city overrides cannot redirect it.
		root := filepath.Dir(filepath.Dir(argv[0]))
		provider := strings.TrimSuffix(filepath.Base(argv[0]), "-attention")
		argv = append([]string{"/usr/bin/python3", "-I", filepath.Join(root, "lib", "gct_attention_launch.py"), provider, manifest.CityPath}, argv[1:]...)
		return shellquote.Join(argv), nil
	}
	for _, pin := range manifest.Integrity.Providers {
		if argv[0] != pin.Path || !closedWorkerVersion.MatchString(pin.Version) {
			continue
		}
		// Refuse drift before running even a version probe. A signed chain is
		// trusted only at its exact pinned path and entrypoint bytes.
		resolved, err := filepath.EvalSymlinks(pin.Path)
		if err != nil || resolved != pin.ResolvedPath || resolved != pin.Path {
			return "", fmt.Errorf("provider allowlist wrapper requires its exact physical source path: %q", pin.Path)
		}
		digest, err := digestRegularFile(resolved)
		if err != nil || digest != pin.SHA256 {
			return "", fmt.Errorf("provider allowlist wrapper bytes drift: %q", pin.Path)
		}
		effective, err := effectiveProviderPin(manifest, pin)
		if err != nil {
			return "", err
		}
		if err := VerifyAllowlistedProvider(ctx, manifest, effective); err != nil {
			return "", err
		}
		argv[0] = resolved
		return shellquote.Join(argv), nil
	}
	return "", fmt.Errorf("provider allowlist launch executable %q is not approved", argv[0])
}

func nativeProviderLaunch(manifest Manifest, provider string, arguments []string) (string, error) {
	for _, pin := range manifest.Integrity.Files {
		if filepath.Base(pin.Path) != provider+"-attention" || filepath.Base(filepath.Dir(pin.Path)) != "bin" {
			continue
		}
		supported, err := verifyAttentionLaunchChain(manifest, pin.Path)
		if err != nil {
			return "", err
		}
		if !supported {
			return "", fmt.Errorf("provider allowlist native launcher does not support %q", provider)
		}
		helper := filepath.Join(filepath.Dir(filepath.Dir(pin.Path)), "lib", "gct_attention_launch.py")
		return shellquote.Join(append([]string{"/usr/bin/python3", "-I", helper, "native:" + provider, manifest.CityPath}, arguments...)), nil
	}
	return "", fmt.Errorf("provider allowlist native launcher for %q requires a signed attention source chain", provider)
}

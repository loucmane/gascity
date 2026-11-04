package platforminstall

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Closed helpers import sibling source before reporting a version. Authenticate
// that entire existing source domain without executing any of it first.
func verifyClosedSource(city string, pin ProviderPin) error {
	resolved, err := filepath.EvalSymlinks(pin.Path)
	if err != nil || resolved != pin.Path || resolved != pin.ResolvedPath {
		return fmt.Errorf("closed worker requires its exact physical source path: %q", pin.Path)
	}
	parts := closedWorkerVersion.FindStringSubmatch(pin.Version)
	if parts == nil || filepath.Base(filepath.Dir(pin.Path)) != "bin" {
		return fmt.Errorf("closed worker source requires a bin entrypoint and version identity")
	}
	root := filepath.Dir(filepath.Dir(pin.Path))
	family := parts[2]
	label := family
	if family == "signing" {
		label = "core-signing"
	}
	paths := []string{
		filepath.Join(root, "templates/claude", label+"-control-policy.json"), pin.Path,
		filepath.Join(root, "lib", "gct_claude_"+strings.ReplaceAll(family, "-", "_")+"_worker.py"),
	}
	if family != "signing" {
		paths = append(paths, filepath.Join(root, "lib/gct_claude_signing_worker.py"))
	}
	paths = append(paths, filepath.Join(root, "lib/gct_claude_subscription.py"))
	if family == "project-candidate" {
		paths = append(paths, filepath.Join(root, "lib/gct_claude_workspace_trust.py"))
	}
	paths = append(paths, filepath.Join(root, "templates/claude", family+"-provider.toml"),
		filepath.Join(root, "lib/gct_provider_allowlist.py"))
	records := make([]map[string]string, 0, len(paths))
	for _, path := range paths {
		physical, err := filepath.EvalSymlinks(path)
		if err != nil || physical != path {
			return fmt.Errorf("closed worker source requires physical files: %q", path)
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("closed worker source requires regular files: %q", path)
		}
		digest, err := digestRegularFile(path)
		if err != nil {
			return err
		}
		if path == pin.Path && (digest != pin.SHA256 || info.Mode().Perm()&0o111 == 0) {
			return fmt.Errorf("closed worker entrypoint sha256 or mode drift")
		}
		records = append(records, map[string]string{"path": path, "sha256": digest})
	}
	domain := map[string]any{
		"dependencies": records, "provider": "claude",
		"entrypoint": filepath.Join(filepath.Dir(city), "bin/claude"),
		"allowlist":  filepath.Join(city, "managed/providers.json"),
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(domain); err != nil {
		return err
	}
	encoded := strings.TrimSuffix(buffer.String(), "\n")
	var canonical strings.Builder
	for i := 0; i < len(encoded); i++ {
		if encoded[i] == '\\' && i+1 < len(encoded) {
			if strings.HasPrefix(encoded[i:], `\u2028`) || strings.HasPrefix(encoded[i:], `\u2029`) {
				if encoded[i+5] == '8' {
					canonical.WriteRune('\u2028')
				} else {
					canonical.WriteRune('\u2029')
				}
				i += 5
				continue
			}
			// Consume other escapes together, preserving literal \\u2028 paths.
			canonical.WriteByte(encoded[i])
			i++
		}
		canonical.WriteByte(encoded[i])
	}
	digest := sha256.Sum256([]byte(canonical.String()))
	if hex.EncodeToString(digest[:]) != parts[3] {
		return fmt.Errorf("closed worker source version drift: %q", pin.Path)
	}
	return nil
}

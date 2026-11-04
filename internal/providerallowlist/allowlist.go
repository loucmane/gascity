// Package providerallowlist verifies city-owned, versioned provider executables.
package providerallowlist

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Schema identifies the city provider allowlist format.
const Schema = "gc.provider-allowlist.v1"

var component = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// Entry selects immutable bytes for one provider version.
type Entry struct {
	Provider string `json:"provider"`
	Version  string `json:"version"`
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	Active   bool   `json:"active"`
}

// List retains installed versions and the previous selection for rollback.
type List struct {
	Schema   string            `json:"schema"`
	Entries  []Entry           `json:"entries"`
	Previous map[string]string `json:"previous,omitempty"`
}

// Binding requires a stable entrypoint to resolve to a provider's active entry.
type Binding struct {
	Provider string `json:"provider"`
	Path     string `json:"path"`
}

// Reference delegates CLI selection to approved city configuration.
type Reference struct {
	Path     string    `json:"path"`
	Bindings []Binding `json:"bindings"`
}

// Path returns the canonical city config input location.
func Path(city string) string { return filepath.Join(city, "managed", "providers.json") }

// ReceiptPath returns the protected approval receipt location for a city.
func ReceiptPath(city string) string {
	return filepath.Join(city, ".gc", "runtime", "provisioning", "receipt.json")
}

// Validate checks the signed reference without consulting mutable files.
func (ref Reference) Validate(city string) error {
	if !absolute(ref.Path) || ref.Path != Path(city) {
		return fmt.Errorf("provider allowlist path must be %q", Path(city))
	}
	if len(ref.Bindings) == 0 {
		return fmt.Errorf("provider allowlist bindings must not be empty")
	}
	providers, paths := map[string]bool{}, map[string]bool{}
	for _, binding := range ref.Bindings {
		if !component.MatchString(binding.Provider) || !absolute(binding.Path) || providers[binding.Provider] || paths[binding.Path] {
			return fmt.Errorf("provider allowlist has invalid or duplicate binding %q at %q", binding.Provider, binding.Path)
		}
		providers[binding.Provider], paths[binding.Path] = true, true
	}
	return nil
}

// Parse strictly decodes a list and rejects ambiguous selections.
func Parse(data []byte) (List, error) {
	if len(data) > 1<<20 {
		return List{}, fmt.Errorf("provider allowlist exceeds 1 MiB")
	}
	value, err := strictJSON(data)
	if err != nil {
		return List{}, fmt.Errorf("provider allowlist: %w", err)
	}
	object, ok := value.(map[string]any)
	if !ok || object["entries"] == nil {
		return List{}, fmt.Errorf("provider allowlist entries are required")
	}
	for key := range object {
		if key != "schema" && key != "entries" && key != "previous" {
			return List{}, fmt.Errorf("provider allowlist unknown field %q", key)
		}
	}
	if object["schema"] != Schema {
		return List{}, fmt.Errorf("provider allowlist schema is unsupported")
	}
	if previous, exists := object["previous"]; exists {
		if _, ok := previous.(map[string]any); !ok {
			return List{}, fmt.Errorf("provider allowlist previous must be an object")
		}
	}
	var list List
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&list); err != nil {
		return List{}, fmt.Errorf("provider allowlist: %w", err)
	}
	if list.Schema != Schema {
		return List{}, fmt.Errorf("provider allowlist schema %q is unsupported", list.Schema)
	}
	seen, active, represented := map[string]bool{}, map[string]int{}, map[string]bool{}
	for _, entry := range list.Entries {
		key := entry.Provider + "/" + entry.Version
		if !component.MatchString(entry.Provider) || !component.MatchString(entry.Version) || !absolute(entry.Path) || !validSHA(entry.SHA256) || seen[key] {
			return List{}, fmt.Errorf("provider allowlist invalid or duplicate entry %q", key)
		}
		seen[key], represented[entry.Provider] = true, true
		if entry.Active {
			active[entry.Provider]++
		}
	}
	// Require explicit active booleans; a missing field must not silently mean false.
	entries, ok := object["entries"].([]any)
	if !ok {
		return List{}, fmt.Errorf("provider allowlist entries must be an array")
	}
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			return List{}, fmt.Errorf("provider allowlist entry must be an object")
		}
		if len(entry) != 5 {
			return List{}, fmt.Errorf("provider allowlist entry has unknown or missing fields")
		}
		for _, key := range []string{"provider", "version", "path", "sha256"} {
			if _, ok := entry[key].(string); !ok {
				return List{}, fmt.Errorf("provider allowlist %s must be a string", key)
			}
		}
		if _, ok := entry["active"].(bool); !ok {
			return List{}, fmt.Errorf("provider allowlist active must be a boolean")
		}
	}
	for provider := range represented {
		if active[provider] != 1 {
			return List{}, fmt.Errorf("provider allowlist %q needs exactly one active entry", provider)
		}
	}
	for provider, version := range list.Previous {
		if !seen[provider+"/"+version] {
			return List{}, fmt.Errorf("provider allowlist previous %q/%q is missing", provider, version)
		}
		if entry, ok := list.Active(provider); ok && entry.Version == version {
			return List{}, fmt.Errorf("provider allowlist previous %q is active", provider)
		}
	}
	return list, nil
}

// Active returns the selected version of a provider from a validated list.
func (list List) Active(provider string) (Entry, bool) {
	for _, entry := range list.Entries {
		if entry.Provider == provider && entry.Active {
			return entry, true
		}
	}
	return Entry{}, false
}

// Verify checks every active binary and every required stable entrypoint.
// It never executes bytes being inspected, including untrusted changed bytes.
func (list List) Verify(bindings []Binding) error {
	for _, entry := range list.Entries {
		if !entry.Active {
			continue
		}
		if err := entry.verify(); err != nil {
			return fmt.Errorf("provider allowlist %s/%s: %w", entry.Provider, entry.Version, err)
		}
	}
	for _, binding := range bindings {
		entry, ok := list.Active(binding.Provider)
		if !ok {
			return fmt.Errorf("provider allowlist %q has no active entry", binding.Provider)
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(binding.Path))
		if err != nil || parent != filepath.Dir(binding.Path) {
			return fmt.Errorf("provider allowlist %q entrypoint parent must be physical", binding.Provider)
		}
		if binding.Path != entry.Path {
			target, err := os.Readlink(binding.Path)
			if err != nil || target != entry.Path {
				return fmt.Errorf("provider allowlist %q entrypoint must link directly to active binary", binding.Provider)
			}
		}
		resolved, err := filepath.EvalSymlinks(binding.Path)
		if err != nil {
			return fmt.Errorf("provider allowlist %q entrypoint: %w", binding.Provider, err)
		}
		if resolved != entry.Path {
			return fmt.Errorf("provider allowlist %q entrypoint resolves to %q, want %q", binding.Provider, resolved, entry.Path)
		}
	}
	return nil
}

func (entry Entry) verify() error {
	resolved, err := filepath.EvalSymlinks(entry.Path)
	if err != nil {
		return fmt.Errorf("active binary missing or unreadable: %w", err)
	}
	if resolved != entry.Path {
		return fmt.Errorf("active path must be immutable, not a symlink: %q", entry.Path)
	}
	info, err := os.Lstat(entry.Path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o555 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return fmt.Errorf("active binary must be a regular file with mode 0555: %q", entry.Path)
	}
	file, err := os.Open(entry.Path)
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck // read-only descriptor
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != entry.SHA256 {
		return fmt.Errorf("active binary sha256 mismatch: got %s want %s", got, entry.SHA256)
	}
	return nil
}

func absolute(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsRune(path, 0)
}

func validSHA(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == value
}

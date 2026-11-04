package providerallowlist

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ValidateOptionalJSON enforces exact keys/types on the new optional pin or
// reference before encoding/json can accept case aliases or null pointers.
// Legacy documents without this extension keep their existing decoder path.
func ValidateOptionalJSON(data []byte, manifest bool) error {
	// ga-r9lv: legacy documents keep encoding/json's original byte and duplicate
	// handling. Inspect every member before choosing that branch, so a duplicate
	// outer integrity object cannot hide an earlier provider authority declaration.
	found, err := HasProviderExtension(data, manifest)
	if err != nil || !found {
		return err
	}
	value, err := strictJSON(data)
	if err != nil {
		return err
	}
	if _, ok := value.(map[string]any); !ok {
		return fmt.Errorf("document must be an object")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	if manifest {
		for key := range object {
			if strings.EqualFold(key, "integrity") && key != "integrity" {
				return fmt.Errorf("integrity key must be exact")
			}
		}
		raw, ok := object["integrity"]
		if !ok {
			return nil
		}
		if err := json.Unmarshal(raw, &object); err != nil {
			return err
		}
	}
	found = false
	for key := range object {
		if strings.EqualFold(key, "provider_allowlist") {
			if key != "provider_allowlist" {
				return fmt.Errorf("provider_allowlist key must be exact")
			}
			found = true
		}
	}
	if !found {
		return nil
	}
	raw, err := strictJSON(object["provider_allowlist"])
	if err != nil {
		return err
	}
	fields, ok := raw.(map[string]any)
	if !ok || len(fields) != 2 {
		return fmt.Errorf("provider_allowlist must be an exact object")
	}
	path, ok := fields["path"].(string)
	if !ok || !absolute(path) {
		return fmt.Errorf("provider_allowlist path invalid")
	}
	if !manifest {
		digest, ok := fields["sha256"].(string)
		if !ok || !validSHA(digest) {
			return fmt.Errorf("provider_allowlist sha256 invalid")
		}
		return nil
	}
	bindings, ok := fields["bindings"].([]any)
	if !ok || len(bindings) == 0 {
		return fmt.Errorf("provider_allowlist bindings invalid")
	}
	for _, raw := range bindings {
		binding, ok := raw.(map[string]any)
		if !ok || len(binding) != 2 {
			return fmt.Errorf("provider_allowlist binding fields invalid")
		}
		provider, ok := binding["provider"].(string)
		if !ok || !component.MatchString(provider) {
			return fmt.Errorf("provider_allowlist binding provider invalid")
		}
		path, ok := binding["path"].(string)
		if !ok || !absolute(path) {
			return fmt.Errorf("provider_allowlist binding path invalid")
		}
	}
	return nil
}

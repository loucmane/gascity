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
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	if manifest {
		raw, ok := object["integrity"]
		if !ok {
			for key, value := range object {
				if strings.EqualFold(key, "integrity") {
					var fields map[string]json.RawMessage
					if err := json.Unmarshal(value, &fields); err != nil {
						return err
					}
					for field := range fields {
						if strings.EqualFold(field, "provider_allowlist") {
							return fmt.Errorf("integrity key must be exact for provider_allowlist")
						}
					}
				}
			}
			return nil
		}
		if err := json.Unmarshal(raw, &object); err != nil {
			return err
		}
	}
	found := false
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
	// Detect duplicates in the original bytes, including within the extension.
	if _, err := strictJSON(data); err != nil {
		return err
	}
	raw, err := strictJSON(object["provider_allowlist"])
	if err != nil {
		return err
	}
	value, ok := raw.(map[string]any)
	if !ok || len(value) != 2 {
		return fmt.Errorf("provider_allowlist must be an exact object")
	}
	path, ok := value["path"].(string)
	if !ok || !absolute(path) {
		return fmt.Errorf("provider_allowlist path invalid")
	}
	if !manifest {
		digest, ok := value["sha256"].(string)
		if !ok || !validSHA(digest) {
			return fmt.Errorf("provider_allowlist sha256 invalid")
		}
		return nil
	}
	bindings, ok := value["bindings"].([]any)
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

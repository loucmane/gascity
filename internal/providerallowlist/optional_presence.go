package providerallowlist

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// hasProviderExtension uses the legacy decoder only to select the contract.
// Decode each value as raw JSON, retaining every duplicate object member. The
// strict decoder validates the entire image whenever any member opts in.
func hasProviderExtension(data []byte, manifest bool) (bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return false, err
	}
	if token != json.Delim('{') {
		return false, fmt.Errorf("document must be an object")
	}
	found := false
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return false, err
		}
		key, ok := token.(string)
		if !ok {
			return false, fmt.Errorf("document key must be a string")
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return false, err
		}
		if !manifest {
			found = found || strings.EqualFold(key, "provider_allowlist")
		} else if strings.EqualFold(key, "integrity") && !bytes.Equal(raw, []byte("null")) {
			present, err := hasProviderExtension(raw, false)
			if err != nil {
				return false, err
			}
			found = found || present
		}
	}
	if _, err := decoder.Token(); err != nil {
		return false, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return false, fmt.Errorf("trailing JSON data")
	}
	return found, nil
}

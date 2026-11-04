package providerallowlist

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// HasProviderExtension detects presence without broadening accepted encodings.
// Retain every duplicate member and any recognized opt-in even when later
// parsing fails. The strict decoder validates the entire opted-in image.
func HasProviderExtension(data []byte, manifest bool) (bool, error) {
	found, err := scanProviderExtension(data, manifest)
	if found || err == nil {
		return found, err
	}
	found = scopedProviderDeclaration(data, manifest)
	if recovered, ok := recoverPresenceEncoding(data); !found && ok {
		found = scopedProviderDeclaration(recovered, manifest)
	}
	// Keep the original error even without an extension: legacy JSON loaders
	// must not start accepting BOM or wide-encoded documents through this gate.
	return found, err
}

func scanProviderExtension(data []byte, manifest bool) (bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return false, err
	}
	if token != json.Delim('{') {
		return false, fmt.Errorf("document must be an object")
	}
	found, err := providerExtensionFields(decoder, manifest)
	if err != nil {
		return found, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return found, fmt.Errorf("trailing JSON data")
	}
	return found, nil
}

// The opening brace has already been consumed. Walk integrity directly so a
// truncated extension value cannot hide its already recognized member name.
func providerExtensionFields(decoder *json.Decoder, manifest bool) (bool, error) {
	found := false
	var shapeErr error
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return found, err
		}
		key, ok := token.(string)
		if !ok {
			return found, fmt.Errorf("document key must be a string")
		}
		if !manifest {
			found = found || strings.EqualFold(key, "provider_allowlist")
		}
		if manifest && strings.EqualFold(key, "integrity") {
			token, err := decoder.Token()
			if err != nil {
				return found, err
			}
			if token == nil {
				continue
			}
			if token != json.Delim('{') {
				shapeErr = fmt.Errorf("integrity must be an object")
				// Finish this value and keep scanning duplicate integrity members:
				// an invalid earlier shape must not hide a later opt-in either.
				if token == json.Delim('[') {
					for decoder.More() {
						var raw json.RawMessage
						if err := decoder.Decode(&raw); err != nil {
							return found, err
						}
					}
					if _, err := decoder.Token(); err != nil {
						return found, err
					}
				}
				continue
			}
			present, err := providerExtensionFields(decoder, false)
			found = found || present
			if err != nil {
				return found, err
			}
		} else {
			var raw json.RawMessage
			if err := decoder.Decode(&raw); err != nil {
				return found, err
			}
		}
	}
	if _, err := decoder.Token(); err != nil {
		return found, err
	}
	return found, shapeErr
}

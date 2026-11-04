package providerallowlist

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// ReadApproved checks a protected provisioning receipt's self-digest and its
// approval of the exact allowlist bytes before decoding the list. The full
// worker receipt contract remains owned by managedworker; this narrow storage
// projection avoids importing that higher-level package into platforminstall.
// An empty revision is used by platform inspection, which has no loaded config.
func ReadApproved(path string, receipt []byte, revision string) (List, error) {
	value, err := strictJSON(receipt)
	if err != nil {
		return List{}, fmt.Errorf("provider allowlist approval: %w", err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return List{}, fmt.Errorf("provider allowlist approval must be an object")
	}
	if object["schema"] != "gc.provisioning-receipt.v1" && object["schema"] != "gc.provisioning-receipt.v2" {
		return List{}, fmt.Errorf("provider allowlist approval receipt schema is unsupported")
	}
	want, ok := object["receipt_sha256"].(string)
	if !ok || !validSHA(want) {
		return List{}, fmt.Errorf("provider allowlist approval receipt_sha256 is invalid")
	}
	delete(object, "receipt_sha256")
	canonical, err := json.Marshal(object)
	if err != nil {
		return List{}, err
	}
	sum := sha256.Sum256(canonical)
	if hex.EncodeToString(sum[:]) != want {
		return List{}, fmt.Errorf("provider allowlist approval receipt_sha256 mismatch")
	}
	approvedRevision, ok := object["permission_revision"].(string)
	if !ok || !validSHA(approvedRevision) || (revision != "" && approvedRevision != revision) {
		return List{}, fmt.Errorf("provider allowlist approval permission_revision mismatch")
	}
	pin, ok := object["provider_allowlist"].(map[string]any)
	if !ok || len(pin) != 2 || pin["path"] != path {
		return List{}, fmt.Errorf("provider allowlist approval missing or path mismatch")
	}
	want, ok = pin["sha256"].(string)
	if !ok || !validSHA(want) {
		return List{}, fmt.Errorf("provider allowlist approval sha256 invalid")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return List{}, fmt.Errorf("provider allowlist: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > 1024*1024 {
		return List{}, fmt.Errorf("provider allowlist must be a regular file at most 1 MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return List{}, fmt.Errorf("provider allowlist: %w", err)
	}
	sum = sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != want {
		return List{}, fmt.Errorf("provider allowlist approval sha256 mismatch; refresh provisioning receipt")
	}
	return Parse(data)
}

// strictJSON retains exact numeric spelling and rejects duplicate keys at
// every nesting level, unlike encoding/json's default last-value-wins decoder.
func strictJSON(data []byte) (any, error) {
	if len(data) > 16*1024*1024 {
		return nil, fmt.Errorf("JSON exceeds 16 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, err := jsonValue(decoder, 0)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing JSON data")
	}
	return value, nil
}

func jsonValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, fmt.Errorf("JSON nesting exceeds 64")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, nested := token.(json.Delim)
	if !nested {
		return token, nil
	}
	if delim == '{' {
		object := map[string]any{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("JSON object key must be a string")
			}
			if _, exists := object[name]; exists {
				return nil, fmt.Errorf("duplicate JSON field %q", name)
			}
			value, err := jsonValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			object[name] = value
		}
		_, err := decoder.Token()
		return object, err
	}
	if delim == '[' {
		array := []any{}
		for decoder.More() {
			value, err := jsonValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		_, err := decoder.Token()
		return array, err
	}
	return nil, fmt.Errorf("unexpected JSON delimiter %q", delim)
}

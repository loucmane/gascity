package providerallowlist

import (
	"encoding/json"
	"regexp"
	"strings"
)

var presenceTokens = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|[{}\[\],:]`)

// scopedProviderDeclaration recovers keys at their schema location after a
// malformed unrelated value. It never accepts JSON: the caller retains the
// original parse error. String values and nested application keys are ignored.
func scopedProviderDeclaration(data []byte, manifest bool) bool {
	var stack []byte
	previous, integrity := "", false
	for _, raw := range presenceTokens.FindAll(data, -1) {
		token := string(raw)
		switch token {
		case "{", "[":
			stack = append(stack, token[0])
		case "}", "]":
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		default:
			if token[0] == '"' && (previous == "{" || previous == ",") && len(stack) > 0 && stack[len(stack)-1] == '{' {
				if len(stack) == 1 {
					integrity = false
				}
				var key string
				if err := json.Unmarshal(raw, &key); err == nil {
					if len(stack) == 1 {
						integrity = strings.EqualFold(key, "integrity")
					}
					atReceipt := !manifest && string(stack) == "{"
					atManifest := manifest && string(stack) == "{{" && integrity
					if (atReceipt || atManifest) && strings.EqualFold(key, "provider_allowlist") {
						return true
					}
				}
			}
		}
		previous = token
	}
	return false
}

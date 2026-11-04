package providerallowlist

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOptionalJSONPreservesLegacyDecoder(t *testing.T) {
	for _, manifest := range []bool{false, true} {
		for name, input := range map[string][]byte{
			"ordinary":     []byte(`{"x":1}`),
			"duplicates":   []byte(`{"x":1,"x":2}`),
			"invalid UTF8": []byte("{\"x\":\"\xff\"}"),
			"deep":         []byte(`{"x":` + strings.Repeat("[", 70) + "0" + strings.Repeat("]", 70) + "}"),
			"BOM":          []byte("\xef\xbb\xbf{}"),
			"UTF16":        {0xff, 0xfe, '{', 0, '}', 0},
			"UTF32":        {0xff, 0xfe, 0, 0, '{', 0, 0, 0, '}', 0, 0, 0},
		} {
			t.Run(name+map[bool]string{false: "/receipt", true: "/manifest"}[manifest], func(t *testing.T) {
				var baseline map[string]json.RawMessage
				want := json.Unmarshal(input, &baseline)
				got := ValidateOptionalJSON(input, manifest)
				if (got == nil) != (want == nil) {
					t.Fatalf("baseline error=%v; optional extension gate error=%v", want, got)
				}
			})
		}
	}
}

func TestOptionalJSONCannotHideOptInBehindLegacyDuplicates(t *testing.T) {
	for _, input := range []string{
		`{"integrity":{"provider_allowlist":null},"integrity":{}}`,
		`{"Integrity":{"provider_allowlist":null},"integrity":{}}`,
		`{"integrity":{"Provider_Allowlist":null},"integrity":{}}`,
		`{"integrity":{"provider_allowlist":null},"integrity":null}`,
	} {
		if err := ValidateOptionalJSON([]byte(input), true); err == nil {
			t.Fatalf("hidden authority was accepted: %s", input)
		}
	}
}

func TestPresenceRecoveryIgnoresNestedEnvironment(t *testing.T) {
	for _, manifest := range []bool{false, true} {
		for _, key := range []string{"provider_allowlist", "PROVIDER_ALLOWLIST"} {
			input := `{"profiles":[{"environment":{"` + key + `":"application-setting"}}]}`
			if manifest {
				input = `{"integrity":` + input + `}`
			}
			for _, raw := range [][]byte{[]byte(input), []byte("\xef\xbb\xbf" + input), []byte(input + "\xff"), []byte(`{"unrelated":invalid,` + input[1:])} {
				if found, err := HasProviderExtension(raw, manifest); found {
					t.Fatalf("nested environment selected approval: %s, %v", raw, err)
				}
			}
		}
	}
}

func TestOptionalJSONStrictAfterOptIn(t *testing.T) {
	for _, manifest := range []bool{false, true} {
		pin := `"provider_allowlist":{"path":"/city/managed/providers.json","sha256":"` + strings.Repeat("a", 64) + `"}`
		if manifest {
			pin = `"integrity":{"provider_allowlist":{"path":"/city/managed/providers.json","bindings":[{"provider":"claude","path":"/bin/claude"}]}}`
		}
		for name, field := range map[string]string{
			"duplicates":   `"x":1,"x":2`,
			"invalid UTF8": "\"x\":\"\xff\"",
			"deep":         `"x":` + strings.Repeat("[", 70) + "0" + strings.Repeat("]", 70),
		} {
			t.Run(name+map[bool]string{false: "/receipt", true: "/manifest"}[manifest], func(t *testing.T) {
				if err := ValidateOptionalJSON([]byte("{"+field+","+pin+"}"), manifest); err == nil {
					t.Fatal("opted-in document escaped strict decoding")
				}
			})
		}
	}
}

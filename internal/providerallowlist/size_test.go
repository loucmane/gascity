package providerallowlist

import (
	"bytes"
	"testing"
)

func TestParseMatchesAuthorityByteLimit(t *testing.T) {
	data := []byte(`{"schema":"gc.provider-allowlist.v1","entries":[]}`)
	for _, size := range []int{1 << 20, (1 << 20) + 1} {
		padded := append(append([]byte(nil), data...), bytes.Repeat([]byte(" "), size-len(data))...)
		_, err := Parse(padded)
		if (err == nil) != (size == 1<<20) {
			t.Fatalf("Parse %d bytes: %v", size, err)
		}
	}
}

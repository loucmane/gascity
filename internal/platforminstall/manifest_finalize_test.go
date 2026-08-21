package platforminstall

import (
	"bytes"
	"strings"
	"testing"
)

func TestFinalizeManifestProducesCanonicalLoadableSelfDigest(t *testing.T) {
	manifest := activationManifest(t, t.TempDir())
	manifest.ManifestSHA256 = ""
	input := marshalManifest(t, manifest)

	finalized, data, err := FinalizeManifest(input)
	if err != nil {
		t.Fatalf("FinalizeManifest() error = %v", err)
	}
	if finalized.ManifestSHA256 == "" || finalized.ManifestSHA256 != mustManifestDigest(t, finalized) {
		t.Fatalf("finalized manifest digest = %q, want canonical self-digest", finalized.ManifestSHA256)
	}
	want := marshalManifest(t, finalized)
	if !bytes.Equal(data, want) {
		t.Fatalf("FinalizeManifest() data is not canonical:\ngot  %s\nwant %s", data, want)
	}
	loaded, err := LoadManifest(data)
	if err != nil {
		t.Fatalf("LoadManifest(finalized) error = %v", err)
	}
	if loaded.ManifestSHA256 != finalized.ManifestSHA256 {
		t.Fatalf("loaded digest = %q, want %q", loaded.ManifestSHA256, finalized.ManifestSHA256)
	}
}

func TestFinalizeManifestRejectsDigestUnknownFieldsAndTrailingData(t *testing.T) {
	manifest := activationManifest(t, t.TempDir())
	valid := marshalManifest(t, manifest)
	unsigned := manifest
	unsigned.ManifestSHA256 = ""
	unsignedData := marshalManifest(t, unsigned)
	tests := []struct {
		name  string
		input []byte
		want  string
	}{
		{name: "already finalized", input: valid, want: "manifest_sha256 must be empty"},
		{name: "unknown field", input: bytes.Replace(unsignedData, []byte(`"schema":`), []byte(`"unknown":true,"schema":`), 1), want: "unknown field"},
		{name: "trailing value", input: append(append([]byte(nil), unsignedData...), []byte(` {}`)...), want: "trailing JSON value"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := FinalizeManifest(test.input)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("FinalizeManifest() error = %v, want %q", err, test.want)
			}
		})
	}
}

func mustManifestDigest(t *testing.T, manifest Manifest) string {
	t.Helper()
	digest, err := ManifestDigest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

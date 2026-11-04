package platforminstall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
)

// legacyManifestDecode is the c31fb80bf loader's sequence before the provider
// extension gate. All manifests retain their separate duplicate-key guard.
func legacyManifestDecode(data []byte) (Manifest, error) {
	if err := rejectDuplicateMetadataFields(data); err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Manifest{}, err
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	want, err := ManifestDigest(manifest)
	if err != nil {
		return Manifest{}, err
	}
	if manifest.ManifestSHA256 != want {
		return Manifest{}, fmt.Errorf("manifest_sha256 mismatch")
	}
	return manifest, nil
}

func TestManifestLegacyDecoderBaseline(t *testing.T) {
	manifest := testManifest(t, t.TempDir(), []byte("candidate"), []byte("installed"))
	data := marshalManifest(t, manifest)
	manifest.ReleaseID = "legacy-\ufffd"
	manifest = finalizeManifest(t, manifest)
	unicode := marshalManifest(t, manifest)
	wide := func(width int) []byte {
		encoded := []byte{0xff, 0xfe}
		if width == 4 {
			encoded = append(encoded, 0, 0)
		}
		for _, b := range data {
			encoded = append(encoded, b)
			for i := 1; i < width; i++ {
				encoded = append(encoded, 0)
			}
		}
		return encoded
	}
	for name, raw := range map[string][]byte{
		"ordinary":        data,
		"duplicate field": append([]byte(`{"release_id":"ignored",`), data[1:]...),
		"invalid UTF8":    bytes.Replace(unicode, []byte("\ufffd"), []byte{0xff}, 1),
		"BOM":             append([]byte{0xef, 0xbb, 0xbf}, data...),
		"UTF16":           wide(2),
		"UTF32":           wide(4),
	} {
		t.Run(name, func(t *testing.T) {
			want, oldErr := legacyManifestDecode(raw)
			got, newErr := LoadManifest(raw)
			if (oldErr == nil) != (newErr == nil) || (oldErr == nil && !reflect.DeepEqual(want, got)) {
				t.Fatalf("legacy manifest changed: baseline error=%v; candidate error=%v", oldErr, newErr)
			}
			// The new city gate must preserve the complete command for every
			// legacy document the old manifest loader accepted.
			if oldErr == nil {
				path := DefaultManifestPath(manifest.CityPath)
				writeProviderFixture(t, path, raw)
				defer func() {
					if err := os.Remove(path); err != nil {
						t.Error(err)
					}
				}()
				const command = "legacy --resume 'a conversation'"
				actual, err := ResolveCityProviderLaunch(context.Background(), manifest.CityPath, "", command)
				if err != nil || actual != command {
					t.Fatalf("legacy city gate: %q, %v", actual, err)
				}
			}
		})
	}
}

// Package platforminstall validates and applies digest-pinned Gas City release manifests.
package platforminstall

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// ManifestSchemaV1 is the first supported platform-install manifest schema.
	ManifestSchemaV1 = "gc.platform-install-manifest.v1"

	// ResultInstalled records that the requested release was published.
	ResultInstalled = "installed"
	// ResultNoop records that the exact requested release was already installed.
	ResultNoop = "noop"
)

// ErrApplyDisabled is returned by the validation-only red baseline. It keeps
// release publication fail-closed until the transactional publisher lands.
var ErrApplyDisabled = errors.New("platform install apply is disabled")

// Artifact identifies one immutable release artifact and its destination.
type Artifact struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	SHA256      string `json:"sha256"`
	Mode        uint32 `json:"mode"`
}

// Manifest is the digest-pinned input to one platform installation.
type Manifest struct {
	Schema         string   `json:"schema"`
	ReleaseID      string   `json:"release_id"`
	Core           Artifact `json:"core"`
	BackupPath     string   `json:"backup_path"`
	ReceiptPath    string   `json:"receipt_path"`
	ManifestSHA256 string   `json:"manifest_sha256"`
}

// Receipt is the durable result of an installation attempt that reached a
// terminal installed or no-op state.
type Receipt struct {
	Schema         string `json:"schema"`
	ReleaseID      string `json:"release_id"`
	ManifestSHA256 string `json:"manifest_sha256"`
	ArtifactSHA256 string `json:"artifact_sha256"`
	PreviousSHA256 string `json:"previous_sha256,omitempty"`
	Result         string `json:"result"`
}

// MarshalManifest serializes a manifest without insignificant whitespace.
func MarshalManifest(manifest Manifest) ([]byte, error) {
	return json.Marshal(manifest)
}

// ManifestDigest returns the canonical SHA-256 over the compact JSON manifest
// with ManifestSHA256 excluded from its own digest domain.
func ManifestDigest(manifest Manifest) (string, error) {
	manifest.ManifestSHA256 = ""
	data, err := MarshalManifest(manifest)
	if err != nil {
		return "", fmt.Errorf("marshal manifest digest domain: %w", err)
	}
	return sha256Hex(data), nil
}

// LoadManifest decodes and validates a platform-install manifest.
func LoadManifest(data []byte) (Manifest, error) {
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode platform install manifest: %w", err)
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
		return Manifest{}, fmt.Errorf("manifest_sha256 mismatch: got %q want %q", manifest.ManifestSHA256, want)
	}
	return manifest, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode platform install manifest: trailing JSON value")
		}
		return fmt.Errorf("decode platform install manifest trailer: %w", err)
	}
	return nil
}

func validateManifest(manifest Manifest) error {
	if manifest.Schema != ManifestSchemaV1 {
		return fmt.Errorf("manifest schema %q is unsupported", manifest.Schema)
	}
	if strings.TrimSpace(manifest.ReleaseID) == "" {
		return errors.New("manifest release_id is required")
	}
	if strings.TrimSpace(manifest.Core.Name) == "" {
		return errors.New("manifest core name is required")
	}
	for name, path := range map[string]string{
		"core.source":      manifest.Core.Source,
		"core.destination": manifest.Core.Destination,
		"backup_path":      manifest.BackupPath,
		"receipt_path":     manifest.ReceiptPath,
	} {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("manifest %s must be an absolute path: %q", name, path)
		}
	}
	paths := []string{manifest.Core.Source, manifest.Core.Destination, manifest.BackupPath, manifest.ReceiptPath}
	for i := range paths {
		for j := i + 1; j < len(paths); j++ {
			if filepath.Clean(paths[i]) == filepath.Clean(paths[j]) {
				return fmt.Errorf("manifest paths must be distinct: %q", paths[i])
			}
		}
	}
	if err := validateSHA256("core.sha256", manifest.Core.SHA256); err != nil {
		return err
	}
	if err := validateSHA256("manifest_sha256", manifest.ManifestSHA256); err != nil {
		return err
	}
	if manifest.Core.Mode != 0o755 {
		return fmt.Errorf("manifest core.mode = %#o, want 0755", manifest.Core.Mode)
	}
	return nil
}

func validateSHA256(name, value string) error {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size || value != strings.ToLower(value) {
		return fmt.Errorf("manifest %s must be a lowercase SHA-256 digest", name)
	}
	return nil
}

type installer struct {
	rename       func(string, string) error
	writeReceipt func(string, Receipt) error
}

func newInstaller() *installer {
	return &installer{
		rename: os.Rename,
		writeReceipt: func(string, Receipt) error {
			return ErrApplyDisabled
		},
	}
}

// Install validates and preflights a manifest, then applies it through the
// transactional installer. The red baseline intentionally fails closed at
// the publication boundary.
func Install(manifest Manifest) (Receipt, error) {
	return newInstaller().install(manifest)
}

func (i *installer) install(manifest Manifest) (Receipt, error) {
	if err := validateManifest(manifest); err != nil {
		return Receipt{}, err
	}
	want, err := ManifestDigest(manifest)
	if err != nil {
		return Receipt{}, err
	}
	if manifest.ManifestSHA256 != want {
		return Receipt{}, fmt.Errorf("manifest_sha256 mismatch: got %q want %q", manifest.ManifestSHA256, want)
	}
	data, err := os.ReadFile(manifest.Core.Source)
	if err != nil {
		return Receipt{}, fmt.Errorf("read candidate artifact: %w", err)
	}
	if got := sha256Hex(data); got != manifest.Core.SHA256 {
		return Receipt{}, fmt.Errorf("candidate sha256 mismatch: got %s want %s", got, manifest.Core.SHA256)
	}
	return Receipt{}, ErrApplyDisabled
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

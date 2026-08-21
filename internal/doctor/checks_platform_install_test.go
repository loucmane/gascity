package doctor

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/platforminstall"
)

func TestPlatformInstallIntegrityCheckNotManaged(t *testing.T) {
	result := NewPlatformInstallIntegrityCheck().Run(&CheckContext{CityPath: t.TempDir()})
	if result.Status != StatusOK || !strings.Contains(result.Message, "not managed") {
		t.Fatalf("Run() = %+v, want OK not-managed result", result)
	}
}

func TestPlatformInstallIntegrityCheckClean(t *testing.T) {
	cityPath := t.TempDir()
	manifest := installDoctorFixture(t, cityPath)
	if _, err := platforminstall.Install(manifest); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	writeInstallManifest(t, cityPath, manifest)

	result := NewPlatformInstallIntegrityCheck().Run(&CheckContext{CityPath: cityPath})
	if result.Status != StatusOK {
		t.Fatalf("Run() = %+v, want OK", result)
	}
}

func TestPlatformInstallIntegrityCheckReportsFieldDrift(t *testing.T) {
	cityPath := t.TempDir()
	manifest := installDoctorFixture(t, cityPath)
	if _, err := platforminstall.Install(manifest); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	writeInstallManifest(t, cityPath, manifest)
	if err := os.WriteFile(manifest.Core.Destination, []byte("drift"), 0o755); err != nil {
		t.Fatal(err)
	}

	result := NewPlatformInstallIntegrityCheck().Run(&CheckContext{CityPath: cityPath})
	if result.Status != StatusError {
		t.Fatalf("Run() = %+v, want error", result)
	}
	if len(result.Details) == 0 || !strings.Contains(strings.Join(result.Details, "\n"), "core.sha256") {
		t.Fatalf("Run() details = %v, want core.sha256 drift", result.Details)
	}
	if !strings.Contains(result.FixHint, "reviewed manifest") {
		t.Fatalf("Run() FixHint = %q, want reviewed-manifest remediation", result.FixHint)
	}
}

func installDoctorFixture(t *testing.T, cityPath string) platforminstall.Manifest {
	t.Helper()
	candidate := []byte("candidate")
	previous := []byte("previous")
	candidatePath := filepath.Join(cityPath, "candidate", "gc")
	destinationPath := filepath.Join(cityPath, "bin", "gc")
	for _, parent := range []string{filepath.Dir(candidatePath), filepath.Dir(destinationPath)} {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(candidatePath, candidate, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destinationPath, previous, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := platforminstall.Manifest{
		Schema:         platforminstall.ManifestSchemaV1,
		ReleaseID:      "test-release",
		CityPath:       cityPath,
		Core:           platforminstall.Artifact{Name: "gc", Source: candidatePath, Destination: destinationPath, SHA256: doctorSHA256(candidate), Mode: 0o755},
		PreviousSHA256: doctorSHA256(previous),
		BackupPath:     filepath.Join(cityPath, ".gc", "platform", "gc.previous"),
		ReceiptPath:    filepath.Join(cityPath, ".gc", "platform", "install-receipt.json"),
	}
	digest, err := platforminstall.ManifestDigest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifest.ManifestSHA256 = digest
	return manifest
}

func writeInstallManifest(t *testing.T, cityPath string, manifest platforminstall.Manifest) {
	t.Helper()
	data, err := platforminstall.MarshalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := platforminstall.DefaultManifestPath(cityPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func doctorSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

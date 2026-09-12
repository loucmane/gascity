package platforminstall

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMetadataSetupFileNoAmbientAuthority(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "setup")
	data := []byte("fixture only, never executed")
	if err := os.WriteFile(path, data, 0o755); err != nil {
		t.Fatal(err)
	}
	pin := FilePin{Path: path, Mode: 0o755, SHA256: sha256Hex(data)}
	if err := metadataCheckSetupFile(pin); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	bad := pin
	bad.Path = alias
	if err := metadataCheckSetupFile(bad); err == nil {
		t.Fatal("setup alias accepted")
	}
	bad = pin
	bad.Mode = 0o777
	if err := metadataCheckSetupFile(bad); err == nil {
		t.Fatal("mutable mode accepted")
	}
	if err := os.WriteFile(path, []byte("different"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := metadataCheckSetupFile(pin); err == nil {
		t.Fatal("changed executable accepted")
	}
}

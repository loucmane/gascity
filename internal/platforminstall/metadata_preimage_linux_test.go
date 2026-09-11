package platforminstall

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestMetadataPreimageModeRefusedBeforeStaging(t *testing.T) {
	root := t.TempDir()
	target, stage := filepath.Join(root, "manifest"), filepath.Join(root, "stage")
	if err := os.WriteFile(target, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	publication, err := newInstaller().prepareMetadataPublication(target, stage, []byte("new"), sha256Hex([]byte("private")))
	if publication != nil {
		if err := publication.close(); err != nil {
			t.Fatal(err)
		}
	}
	if err == nil {
		t.Fatal("0600 preimage accepted despite 0644-only restoration")
	}
	if _, err := os.Lstat(stage); !os.IsNotExist(err) {
		t.Fatalf("staging occurred: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "private" {
		t.Fatal("preimage bytes changed", err)
	}
	stat, err := os.Stat(target)
	if err != nil || stat.Mode().Perm() != 0o600 {
		t.Fatal("preimage mode changed", err)
	}
}

func TestMetadataPreimageGroupAndSpecialModeRefusal(t *testing.T) {
	stat := unix.Stat_t{Mode: unix.S_IFREG | 0o644, Uid: 1000, Gid: 1000, Nlink: 1, Size: 3}
	if err := metadataRestorableImage(stat, "bound", 0, 1000, 1000); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"group", "private", "special"} {
		changed := stat
		switch kind {
		case "group":
			changed.Gid++
		case "private":
			changed.Mode = unix.S_IFREG | 0o600
		case "special":
			changed.Mode |= unix.S_ISGID
		}
		if err := metadataRestorableImage(changed, "bound", 0, 1000, 1000); err == nil {
			t.Fatalf("accepted %s", kind)
		}
	}
}

func TestMetadataPrecommitRestoreExactSupportedImage(t *testing.T) {
	manifest := metadataContractFixture(t)
	target := DefaultManifestPath(manifest.CityPath)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	previous := []byte("previous canonical metadata")
	if err := os.WriteFile(target, previous, 0o644); err != nil {
		t.Fatal(err)
	}
	var before unix.Stat_t
	if err := unix.Stat(target, &before); err != nil {
		t.Fatal(err)
	}
	installer := newInstaller()
	changed, err := metadataPublishFile(installer, manifest, target, []byte("new"), sha256Hex(previous))
	if err != nil || !changed {
		t.Fatal("precommit publication failed", err)
	}
	if err := metadataRestoreManifest(installer, manifest, previous, sha256Hex([]byte("new"))); err != nil {
		t.Fatal(err)
	}
	var after unix.Stat_t
	if err := unix.Stat(target, &after); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != string(previous) || after.Mode != before.Mode || after.Uid != before.Uid || after.Gid != before.Gid {
		t.Fatal("restoration changed preimage bytes/mode/owner/group", err)
	}
}

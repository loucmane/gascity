package platforminstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func metadataProtectedFixture(t *testing.T) Manifest {
	t.Helper()
	manifest := metadataContractFixture(t)
	manifest.Metadata.Evidence = t.TempDir()
	manifest.Metadata.Parents = nil
	parents := map[string]bool{manifest.Metadata.Evidence: true}
	for _, output := range metadataOutputs(manifest) {
		parents[filepath.Dir(output)] = true
	}
	for path := range parents {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		var info unix.Stat_t
		if err := unix.Lstat(path, &info); err != nil {
			t.Fatal(err)
		}
		manifest.Metadata.Parents = append(manifest.Metadata.Parents, MetadataParent{
			Path: path, Device: info.Dev, Inode: info.Ino, UID: info.Uid, GID: info.Gid, Mode: info.Mode & 0o7777,
		})
	}
	root := filepath.Join(filepath.Dir(DefaultManifestPath(manifest.CityPath)), "assets")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "preserved"), []byte("protected infrastructure"), 0o600); err != nil {
		t.Fatal(err)
	}
	var info unix.Stat_t
	if err := unix.Lstat(root, &info); err != nil {
		t.Fatal(err)
	}
	digest, err := metadataTreeDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Metadata.ProtectedTrees = []MetadataProtectedTree{{
		Pin:    FilePin{Path: root, SHA256: digest, Mode: info.Mode & 0o7777},
		Device: info.Dev, Inode: info.Ino, UID: info.Uid, GID: info.Gid,
	}}
	return manifest
}

// A real canonical-shaped output directory may contain only declared protected
// siblings, while ordinary undeclared directories remain a refusal.
func TestMetadataProtectedSiblingParentContract(t *testing.T) {
	for _, kind := range []string{"valid", "undeclared", "same-content-replacement", "content-drift", "mode-drift", "hardlink", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			manifest := metadataProtectedFixture(t)
			root := manifest.Metadata.ProtectedTrees[0].Pin.Path
			file := filepath.Join(root, "preserved")
			switch kind {
			case "undeclared":
				manifest.Metadata.ProtectedTrees = nil
			case "same-content-replacement":
				// Preserve the original inode as evidence; create byte-identical
				// replacement at the bound path, not merely a corrupted test pin.
				if err := os.Rename(root, filepath.Join(t.TempDir(), "original")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte("protected infrastructure"), 0o600); err != nil {
					t.Fatal(err)
				}
				digest, err := metadataTreeDigest(root)
				if err != nil || digest != manifest.Metadata.ProtectedTrees[0].Pin.SHA256 {
					t.Fatalf("replacement fixture changed content: %s %v", digest, err)
				}
			case "content-drift":
				if err := os.WriteFile(file, []byte("changed"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "mode-drift":
				if err := os.Chmod(root, 0o755); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(file, filepath.Join(t.TempDir(), "alias")); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(file, filepath.Join(root, "alias")); err != nil {
					t.Fatal(err)
				}
			}
			err := metadataCheckParents(manifest)
			if (err == nil) != (kind == "valid") {
				t.Fatalf("%s: %v", kind, err)
			}
		})
	}
}

func TestMetadataProtectedSiblingBindOrder(t *testing.T) {
	manifest := metadataProtectedFixture(t)
	args, err := metadataSandboxArgv(manifest)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, "\x00")
	root := manifest.Metadata.ProtectedTrees[0].Pin.Path
	parent := filepath.Dir(root)
	parentAt := strings.Index(joined, "--bind\x00"+parent+"\x00"+parent)
	childAt := strings.Index(joined, "--ro-bind\x00"+root+"\x00"+root)
	if parentAt < 0 || childAt <= parentAt {
		t.Fatal("protected child must be overmounted after writable parent")
	}
	if strings.Contains(joined[childAt:], "--bind\x00") {
		t.Fatal("later writable mount can shadow protected tree")
	}
}

func TestMetadataProtectedRefusesWritableDirectoryIdentityAlias(t *testing.T) {
	for _, nested := range []bool{false, true} {
		manifest := metadataProtectedFixture(t)
		tree := manifest.Metadata.ProtectedTrees[0]
		alias := tree.Pin.Path
		if nested {
			alias = filepath.Join(alias, "nested")
			if err := os.Mkdir(alias, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		var stat unix.Stat_t
		if err := unix.Lstat(alias, &stat); err != nil {
			t.Fatal(err)
		}
		// Identity-only policy fixture, not a claim of a real bind mount.
		parents := []MetadataParent{{Path: "/separate-writable-alias", Device: stat.Dev, Inode: stat.Ino}}
		if err := metadataProtectedAliases(tree.Pin.Path, parents); err == nil {
			t.Fatal("writable parent aliases protected directory identity")
		}
	}
}

package platforminstall

import (
	"os"
	"path/filepath"
	"testing"
)

// Real filesystem semantics own link text, traversal, and the cache-only boundary.
func TestMetadataCacheLeafLinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("frozen"), 0o600); err != nil {
		t.Fatal(err)
	}
	legacy, err := metadataTreeDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	if digest, err := metadataCacheTreeDigest(root); err != nil || digest != legacy {
		t.Fatal("link-free digest changed", err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink("target", link); err != nil {
		t.Fatal(err)
	}
	first, err := metadataCacheTreeDigest(root)
	if err != nil {
		t.Fatal("internal cache leaf refused", err)
	}
	if _, err := metadataTreeDigest(root); err == nil {
		t.Fatal("ordinary/protected tree accepted a link")
	}
	if repeat, err := metadataCacheTreeDigest(root); err != nil || repeat != first {
		t.Fatal("non-deterministic cache digest", err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("./target", link); err != nil {
		t.Fatal(err)
	}
	second, err := metadataCacheTreeDigest(root)
	if err != nil || second == first {
		t.Fatal("exact link text not bound", err)
	}
	if err := os.WriteFile(target, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if third, err := metadataCacheTreeDigest(root); err != nil || third == second {
		t.Fatal("target content not bound", err)
	}
}

func TestMetadataCacheLinkRefusals(t *testing.T) {
	for _, name := range []string{"dangling", "cycle", "directory", "external", "escape-reentry", "chain", "absolute", "file-parent", "trailing-slash"} {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "cache")
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "target"), []byte("frozen"), 0o600); err != nil {
				t.Fatal(err)
			}
			target := "missing"
			switch name {
			case "cycle":
				target = "link"
			case "directory":
				target = "."
			case "external":
				target = "../outside"
				if err := os.WriteFile(filepath.Join(parent, "outside"), []byte("outside"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "escape-reentry":
				target = "../cache/target"
			case "chain":
				target = "other"
				if err := os.Symlink("target", filepath.Join(root, "other")); err != nil {
					t.Fatal(err)
				}
			case "absolute":
				target = filepath.Join(root, "target")
			case "file-parent":
				target = "target/../target"
			case "trailing-slash":
				target = "target/"
			}
			if err := os.Symlink(target, filepath.Join(root, "link")); err != nil {
				t.Fatal(err)
			}
			if _, err := metadataCacheTreeDigest(root); err == nil {
				t.Fatal("unsafe or unsupported link accepted")
			}
		})
	}
}

func TestMetadataCachePackRelativeLink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "pack", "template-fragments", "gc-role-worker.template.md")
	link := filepath.Join(root, "pack", "agents", "reviewer", "template-fragments", "gc-role-worker.template.md")
	// Match the installed pack's three-level relative hop from its agent fragment.
	for _, dir := range []string{filepath.Dir(target), filepath.Dir(link)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(target, []byte("frozen"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../../template-fragments/gc-role-worker.template.md", link); err != nil {
		t.Fatal(err)
	}
	if _, err := metadataCacheTreeDigest(root); err != nil {
		t.Fatal("actual pack relative-link shape refused", err)
	}
}

func TestMetadataInputTreeCacheSelection(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "cache", "repos")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "target"), []byte("frozen"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	digest, err := metadataCacheTreeDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	pin := FilePin{Path: root, Mode: 0o700, SHA256: digest}
	if err := metadataCheckInputTree(pin, &MetadataLaunch{GCHome: home}); err != nil {
		t.Fatal("exact cache root refused", err)
	}
	if err := metadataCheckInputTree(pin, &MetadataLaunch{GCHome: filepath.Join(home, "other")}); err == nil {
		t.Fatal("non-cache tree accepted links")
	}
	pin.SHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := metadataCheckInputTree(pin, &MetadataLaunch{GCHome: home}); err == nil {
		t.Fatal("cache digest drift accepted")
	}
}

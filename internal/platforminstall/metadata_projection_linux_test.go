package platforminstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func metadataProjectionFixture(t *testing.T) (Manifest, string, string) {
	t.Helper()
	manifest := metadataContractFixture(t)
	root := t.TempDir()
	library := filepath.Join(root, "usr", "lib", "loader.so")
	if err := os.MkdirAll(filepath.Dir(library), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(library, []byte("pinned-library"), 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "lib")
	if err := os.Symlink("usr/lib", alias); err != nil {
		t.Fatal(err)
	}
	manifest.Metadata.Inputs = append(manifest.Metadata.Inputs, FilePin{Path: library, SHA256: sha256Hex([]byte("pinned-library")), Mode: 0o600})
	manifest.Metadata.Links = []MetadataLink{{Path: alias, Target: "usr/lib"}}
	return manifest, root, library
}

func TestMetadataSyntheticDirectoryProjection(t *testing.T) {
	for _, shape := range []string{"elf-loader", "provider-current"} {
		t.Run(shape, func(t *testing.T) {
			manifest, root, library := metadataProjectionFixture(t)
			entry := filepath.Join(root, "lib64", "loader.so")
			target := "../lib/loader.so"
			if shape == "provider-current" {
				entry = filepath.Join(root, "bin", "provider")
			}
			if err := os.MkdirAll(filepath.Dir(entry), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, entry); err != nil {
				t.Fatal(err)
			}
			manifest.Metadata.Links = append(manifest.Metadata.Links, MetadataLink{Path: entry, Target: target})
			if err := metadataValidateLinks(manifest); err != nil {
				t.Fatal("closed synthetic directory alias refused", err)
			}
			if metadataReadCovered(manifest.Metadata, filepath.Dir(library)) || metadataReadCovered(manifest.Metadata, filepath.Join(filepath.Dir(library), "unbound")) {
				t.Fatal("directory alias granted general input access")
			}
			input := metadataInputFS{launch: manifest.Metadata}
			if _, err := input.ReadDir(filepath.Join(root, "lib")); err == nil {
				t.Fatal("projected directory alias granted host directory reads")
			}
			if data, err := input.ReadFile(entry); err != nil || string(data) != "pinned-library" {
				t.Fatal("explicit pinned file link became unreadable", err)
			}
			args, err := metadataSandboxArgv(manifest)
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(args, "\x00")
			if strings.Contains(joined, "--ro-bind\x00"+filepath.Dir(library)+"\x00") || strings.Contains(joined, "--bind\x00"+filepath.Dir(library)+"\x00") {
				t.Fatal("synthetic directory became a host directory mount")
			}
		})
	}
}

func TestMetadataProjectionRefusals(t *testing.T) {
	for _, kind := range []string{"unbound-directory", "writable-alias", "writable-target", "protected", "readonly-shadow", "directory-over-tree", "undeclared-intermediate", "nested-links", "different-text", "duplicate", "forbidden"} {
		t.Run(kind, func(t *testing.T) {
			manifest, root, library := metadataProjectionFixture(t)
			alias := filepath.Join(root, "lib")
			switch kind {
			case "unbound-directory":
				manifest.Metadata.Inputs = manifest.Metadata.Inputs[:1]
			case "writable-alias":
				manifest.Metadata.Evidence = alias
			case "writable-target":
				manifest.Metadata.Evidence = filepath.Dir(library)
			case "protected":
				manifest.Metadata.ProtectedTrees = []MetadataProtectedTree{{Pin: FilePin{Path: filepath.Dir(library)}}}
			case "readonly-shadow":
				manifest.Metadata.Inputs = append(manifest.Metadata.Inputs, FilePin{Path: filepath.Join(alias, "loader.so")})
			case "directory-over-tree":
				manifest.Metadata.Trees = append(manifest.Metadata.Trees, FilePin{Path: filepath.Join(filepath.Dir(library), "subtree")})
			case "undeclared-intermediate":
				hidden := filepath.Join(root, "hidden")
				if err := os.Symlink("usr/lib", hidden); err != nil {
					t.Fatal(err)
				}
				entry := filepath.Join(root, "entry")
				if err := os.Symlink("hidden/loader.so", entry); err != nil {
					t.Fatal(err)
				}
				manifest.Metadata.Links = append(manifest.Metadata.Links, MetadataLink{Path: entry, Target: "hidden/loader.so"})
			case "nested-links":
				entry := filepath.Join(alias, "compat.so")
				if err := os.Symlink("loader.so", entry); err != nil {
					t.Fatal(err)
				}
				manifest.Metadata.Links = append(manifest.Metadata.Links, MetadataLink{Path: entry, Target: "loader.so"})
			case "different-text":
				manifest.Metadata.Links[0].Target = "usr/./lib"
			case "duplicate":
				manifest.Metadata.Links = append(manifest.Metadata.Links, manifest.Metadata.Links[0])
			case "forbidden":
				entry := filepath.Join(root, "proc-alias")
				if err := os.Symlink("/proc", entry); err != nil {
					t.Fatal(err)
				}
				manifest.Metadata.Links = append(manifest.Metadata.Links, MetadataLink{Path: entry, Target: "/proc"})
			}
			if err := metadataValidateLinks(manifest); err == nil {
				t.Fatal("invalid projection accepted")
			}
			if _, err := metadataSandboxArgv(manifest); err == nil {
				t.Fatal("invalid projection reached launch argv")
			}
		})
	}
}

func TestMetadataProjectedResolution(t *testing.T) {
	links := map[string]string{"/r/a": "/r/x/y", "/r/b": "a/../z"}
	if got, err := metadataProjectedPath("/r/b", links); err != nil || got != "/r/x/z" {
		t.Fatal("intermediate alias dotdot semantics", got, err)
	}
	for _, cyclic := range []map[string]string{{"/r/a": "a"}, {"/r/a": "b", "/r/b": "a"}} {
		if _, err := metadataProjectedPath("/r/a", cyclic); err == nil {
			t.Fatal("cyclic projection accepted")
		}
	}
	if _, err := metadataProjectedPath("/r/a", map[string]string{"/r/a": "../../escape"}); err == nil {
		t.Fatal("root escape accepted")
	}
}

func TestMetadataProjectionRetainsPinDigestAuthority(t *testing.T) {
	manifest, _, library := metadataProjectionFixture(t)
	pin := manifest.Metadata.Inputs[len(manifest.Metadata.Inputs)-1]
	if err := metadataValidateLinks(manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(library, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := metadataCheckPin(pin, false); err == nil {
		t.Fatal("directory alias replaced ordinary pin verification")
	}
}

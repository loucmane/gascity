package platforminstall

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestMetadataMountNamespaceRootLabels(t *testing.T) {
	base := "10 1 8:32 / / rw - ext4 /dev/test rw\n"
	for _, name := range []string{"net", "mnt", "uts", "ipc", "pid", "user", "cgroup", "time"} {
		t.Run(name, func(t *testing.T) {
			label := name + ":[4026532242]"
			record := "1028 10 0:4 " + label + " /run/docker/netns/fixture rw shared:441 - nsfs nsfs rw\n"
			mounts, err := metadataParseMounts(strings.NewReader(base + record))
			if err != nil {
				t.Fatal(err)
			}
			if len(mounts) != 2 || mounts[0].id != 10 || mounts[1].id != 1028 || mounts[1].parent != 10 || mounts[1].root != label || mounts[1].point != "/run/docker/netns/fixture" || !mounts[1].propagated || mounts[1].readonly {
				t.Fatalf("namespace record discarded or normalized: %+v", mounts)
			}
		})
	}
	// Ordinary canonical roots and closed mountinfo escapes retain their contract.
	for _, fs := range []string{"ext4", "nsfs"} {
		mounts, err := metadataParseMounts(strings.NewReader("11 10 0:4 /root\\040space /point\\040space ro - " + fs + " source ro\n"))
		if err != nil || len(mounts) != 1 || mounts[0].root != "/root space" || mounts[0].point != "/point space" {
			t.Fatalf("canonical %s root: %+v %v", fs, mounts, err)
		}
	}
}

func TestMetadataMountNamespaceRootRefusals(t *testing.T) {
	for _, tc := range []struct{ name, root, point, fs, field string }{
		{"ordinary-relative", "relative", "/point", "ext4", "root"},
		{"nsfs-relative", "relative", "/point", "nsfs", "root"},
		{"wrong-filesystem", "net:[42]", "/point", "ext4", "root"},
		{"unknown-namespace", "unknown:[42]", "/point", "nsfs", "root"},
		{"symlink-name", "pid_for_children:[42]", "/point", "nsfs", "root"},
		{"missing-bracket", "net:[42", "/point", "nsfs", "root"},
		{"extra-suffix", "net:[42]/extra", "/point", "nsfs", "root"},
		{"empty-inode", "net:[]", "/point", "nsfs", "root"},
		{"zero-inode", "net:[0]", "/point", "nsfs", "root"},
		{"leading-zero", "net:[042]", "/point", "nsfs", "root"},
		{"signed-inode", "net:[+42]", "/point", "nsfs", "root"},
		{"negative-inode", "net:[-42]", "/point", "nsfs", "root"},
		{"hex-inode", "net:[0x42]", "/point", "nsfs", "root"},
		{"overflow", "net:[18446744073709551616]", "/point", "nsfs", "root"},
		{"escaped-label", `net:[4\0402]`, "/point", "nsfs", "root"},
		{"unclean-root", "/a/../b", "/point", "nsfs", "root"},
		{"invalid-root-escape", `/root\999`, "/point", "ext4", "root"},
		{"relative-point", "net:[42]", "point", "nsfs", "mountpoint"},
		{"label-point", "net:[42]", "net:[43]", "nsfs", "mountpoint"},
		{"unclean-point", "net:[42]", "/a/../b", "nsfs", "mountpoint"},
		{"invalid-point-escape", "net:[42]", `/point\999`, "nsfs", "mountpoint"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := fmt.Sprintf("11 10 0:4 %s %s ro - %s source ro\n", tc.root, tc.point, tc.fs)
			_, err := metadataParseMounts(strings.NewReader(record))
			if err == nil || !strings.Contains(err.Error(), "mount 11 "+tc.field+":") {
				t.Fatalf("want field-qualified refusal, got %v", err)
			}
		})
	}
}

func TestMetadataNamespaceMountsCannotSupplyDirectoryAuthority(t *testing.T) {
	for _, writer := range []bool{false, true} {
		for _, where := range []string{"unrelated", "beneath-parent", "beneath-protected", "selected-parent", "selected-protected", "effective-protected-alias"} {
			t.Run(fmt.Sprintf("writer=%t/%s", writer, where), func(t *testing.T) {
				manifest, mounts, ids := metadataMountFixture(t)
				parsed, err := metadataParseMounts(strings.NewReader("500 1 0:4 net:[4026532242] /unrelated ro - nsfs nsfs ro\n"))
				if err != nil {
					t.Fatal(err)
				}
				mount := parsed[0]
				root := manifest.Metadata.ProtectedTrees[0].Pin.Path
				parent := filepath.Dir(root)
				switch where {
				case "beneath-parent":
					mount.point = filepath.Join(parent, "namespace")
				case "beneath-protected":
					mount.point = filepath.Join(root, "namespace")
				case "selected-parent", "selected-protected":
					path := parent
					if where == "selected-protected" {
						path = root
					}
					for i, old := range mounts {
						if old.point == path {
							// Keep every old authority field plausible except the typed root.
							mount.id, mount.parent, mount.device = old.id, old.parent, old.device
							mount.point, mount.readonly = path, old.readonly
							mounts = append(mounts[:i], mounts[i+1:]...)
							break
						}
					}
				case "effective-protected-alias":
					mount.device = manifest.Metadata.ProtectedTrees[0].Device
					ids[root] = mount.id
				}
				mounts = append(mounts, mount)
				err = metadataProtectedMountPolicy(manifest, mounts, writer, func(path string) (uint64, error) { return ids[path], nil })
				if (err == nil) != (where == "unrelated") {
					t.Fatalf("namespace authority %s: %v", where, err)
				}
			})
		}
	}
}

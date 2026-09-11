package platforminstall

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestMetadataProtectedInventoryRefusals(t *testing.T) {
	for _, kind := range []string{"relative", "nested", "duplicate-path", "duplicate-inode", "unbound", "wrong-owner", "special-mode", "writable-mode", "bad-digest", "output", "stage", "evidence", "input", "tree", "absent", "link-target", "other-parent", "overlapping-parents", "bound"} {
		t.Run(kind, func(t *testing.T) {
			manifest := metadataProtectedFixture(t)
			root := manifest.Metadata.ProtectedTrees[0].Pin.Path
			tree := &manifest.Metadata.ProtectedTrees[0]
			switch kind {
			case "relative":
				tree.Pin.Path = "assets"
			case "nested":
				tree.Pin.Path = filepath.Join(root, "nested")
			case "duplicate-path":
				duplicate := *tree
				duplicate.Inode++
				manifest.Metadata.ProtectedTrees = append(manifest.Metadata.ProtectedTrees, duplicate)
			case "duplicate-inode":
				duplicate := *tree
				duplicate.Pin.Path = filepath.Join(filepath.Dir(root), "backups")
				manifest.Metadata.ProtectedTrees = append(manifest.Metadata.ProtectedTrees, duplicate)
			case "unbound":
				tree.Inode = 0
			case "wrong-owner":
				tree.UID++
			case "special-mode":
				tree.Pin.Mode = 0o1700
			case "writable-mode":
				tree.Pin.Mode = 0o777
			case "bad-digest":
				tree.Pin.SHA256 = "bad"
			case "output":
				tree.Pin.Path = DefaultManifestPath(manifest.CityPath)
			case "stage":
				tree.Pin.Path = metadataStage(manifest, DefaultManifestPath(manifest.CityPath))
			case "evidence":
				manifest.Metadata.Evidence = root
			case "input":
				manifest.Metadata.Inputs[0].Path = filepath.Join(root, "input")
			case "tree":
				manifest.Metadata.Trees = append(manifest.Metadata.Trees, FilePin{Path: root})
			case "absent":
				manifest.Metadata.Absent = append(manifest.Metadata.Absent, filepath.Join(root, "absent"))
			case "link-target":
				manifest.Metadata.Links = append(manifest.Metadata.Links, MetadataLink{Path: filepath.Join(filepath.Dir(root), "alias"), Target: "assets"})
			case "other-parent":
				manifest.Metadata.Parents = append(manifest.Metadata.Parents, MetadataParent{Path: root})
			case "overlapping-parents":
				manifest.Metadata.Parents = append(manifest.Metadata.Parents, MetadataParent{Path: manifest.CityPath})
			case "bound":
				for len(manifest.Metadata.ProtectedTrees) < 17 {
					manifest.Metadata.ProtectedTrees = append(manifest.Metadata.ProtectedTrees, *tree)
				}
			}
			if _, err := metadataProtectedChildren(manifest); err == nil {
				t.Fatal("invalid protected inventory accepted")
			}
		})
	}
}

func TestMetadataMountinfoParser(t *testing.T) {
	valid := "10 1 8:32 / / rw,relatime - ext4 /dev/test rw\n11 10 8:32 /source /protected\\040space ro - ext4 /dev/test rw\n"
	mounts, err := metadataParseMounts(strings.NewReader(valid))
	if err != nil || len(mounts) != 2 || mounts[1].point != "/protected space" || !mounts[1].readonly || mounts[0].device != unix.Mkdev(8, 32) {
		t.Fatalf("valid kernel record: %+v %v", mounts, err)
	}
	decoded, err := metadataMountPath(`/a\011b\012c\134d`)
	if err != nil || decoded != "/a\tb\nc\\d" {
		t.Fatalf("escape decoding: %q %v", decoded, err)
	}
	for _, record := range []string{"", "malformed\n", valid + "10 1 8:32 / /other ro - ext4 /dev/test rw\n", strings.Replace(valid, "11 10", "11 11", 1), strings.Replace(valid, "8:32", "bad", 1), strings.Replace(valid, "rw,relatime", "ro,rw", 1), strings.Replace(valid, "rw,relatime", "nosuid", 1), strings.Replace(valid, `protected\040space`, `protected\999space`, 1), strings.Replace(valid, " /source ", " relative ", 1), strings.Replace(valid, " - ext4 ", " ext4 ", 1), strings.Repeat("x", 128*1024)} {
		if _, err := metadataParseMounts(strings.NewReader(record)); err == nil {
			t.Fatalf("malformed record accepted: %.80q", record)
		}
	}
	var many strings.Builder
	for i := 2; i < 16388; i++ {
		fmt.Fprintf(&many, "%d 1 8:32 / /m%d rw - ext4 /dev/test rw\n", i, i)
	}
	if _, err := metadataParseMounts(strings.NewReader(many.String())); err == nil {
		t.Fatal("mount count bound absent")
	}
	// Each line is under its own bound, but the entire kernel inventory is not.
	many.Reset()
	for i := 2; i < 52; i++ {
		fmt.Fprintf(&many, "%d 1 8:32 / /%s%d rw - ext4 /dev/test rw\n", i, strings.Repeat("x", 100000), i)
	}
	if _, err := metadataParseMounts(strings.NewReader(many.String())); err == nil {
		t.Fatal("total inventory bound absent")
	}
	if _, err := metadataParseMounts(&metadataMountErrorReader{}); err == nil {
		t.Fatal("read failure ignored")
	}
}

type metadataMountErrorReader struct{}

func (*metadataMountErrorReader) Read([]byte) (int, error) {
	return 0, errors.New("injected mountinfo read failure")
}

func metadataMountFixture(t *testing.T) (Manifest, []metadataMount, map[string]uint64) {
	t.Helper()
	manifest := metadataProtectedFixture(t)
	canonical := filepath.Dir(DefaultManifestPath(manifest.CityPath))
	mounts := []metadataMount{{id: 1, parent: 99, device: manifest.Metadata.ProtectedTrees[0].Device, root: "/", point: "/", readonly: true}}
	ids := map[string]uint64{}
	for i, parent := range manifest.Metadata.Parents {
		id := uint64(i + 10)
		mounts = append(mounts, metadataMount{id: id, parent: 1, device: parent.Device, root: parent.Path, point: parent.Path})
		ids[parent.Path] = id
	}
	for i, tree := range manifest.Metadata.ProtectedTrees {
		id := uint64(i + 100)
		mounts = append(mounts, metadataMount{id: id, parent: ids[canonical], device: tree.Device, root: tree.Pin.Path, point: tree.Pin.Path, readonly: true})
		ids[tree.Pin.Path] = id
	}
	return manifest, mounts, ids
}

func TestMetadataProtectedWriterMountPolicy(t *testing.T) {
	for _, kind := range []string{"valid", "reordered", "missing", "writable-child", "propagated-child", "wrong-device", "wrong-parent", "effective-alias", "stacked", "nested-rw", "nested-ro", "output-alias", "parent-ro", "parent-propagated", "parent-device", "parent-stacked", "missing-effective", "effective-read-failure"} {
		t.Run(kind, func(t *testing.T) {
			manifest, mounts, ids := metadataMountFixture(t)
			root := manifest.Metadata.ProtectedTrees[0].Pin.Path
			canonical := filepath.Dir(root)
			child := len(mounts) - 1
			parent := 0
			for i, mount := range mounts {
				if mount.point == canonical {
					parent = i
				}
			}
			switch kind {
			case "reordered":
				for i, j := 0, len(mounts)-1; i < j; i, j = i+1, j-1 {
					mounts[i], mounts[j] = mounts[j], mounts[i]
				}
			case "missing":
				mounts = mounts[:child]
			case "writable-child":
				mounts[child].readonly = false
			case "propagated-child":
				mounts[child].propagated = true
			case "wrong-device":
				mounts[child].device++
			case "wrong-parent":
				mounts[child].parent = 1
			case "effective-alias":
				ids[root] = 1
			case "stacked":
				duplicate := mounts[child]
				duplicate.id++
				mounts = append(mounts, duplicate)
			case "nested-rw", "nested-ro":
				mounts = append(mounts, metadataMount{id: 201, parent: ids[root], point: filepath.Join(root, "nested"), readonly: kind == "nested-ro"})
			case "output-alias":
				mounts = append(mounts, metadataMount{id: 202, parent: ids[canonical], point: DefaultManifestPath(manifest.CityPath), readonly: false})
			case "parent-ro":
				mounts[parent].readonly = true
			case "parent-propagated":
				mounts[parent].propagated = true
			case "parent-device":
				mounts[parent].device++
			case "parent-stacked":
				duplicate := mounts[parent]
				duplicate.id = 203
				mounts = append(mounts, duplicate)
			case "missing-effective":
				delete(ids, root)
			}
			lookup := func(path string) (uint64, error) {
				if kind == "effective-read-failure" {
					return 0, errors.New("statx failure")
				}
				return ids[path], nil
			}
			err := metadataProtectedMountPolicy(manifest, mounts, true, lookup)
			if (err == nil) != (kind == "valid" || kind == "reordered") {
				t.Fatalf("%s: %v", kind, err)
			}
		})
	}
}

func TestMetadataProtectedHostMountPolicy(t *testing.T) {
	for _, kind := range []string{"valid", "nested-rw", "nested-ro", "propagated", "wrong-device", "missing-effective", "output-alias"} {
		t.Run(kind, func(t *testing.T) {
			manifest := metadataProtectedFixture(t)
			root := manifest.Metadata.ProtectedTrees[0].Pin.Path
			mounts := []metadataMount{{id: 1, parent: 99, device: manifest.Metadata.ProtectedTrees[0].Device, root: "/", point: "/"}}
			actual := uint64(1)
			switch kind {
			case "nested-rw", "nested-ro":
				mounts = append(mounts, metadataMount{id: 2, parent: 1, point: filepath.Join(root, "nested"), readonly: kind == "nested-ro"})
			case "propagated":
				mounts[0].propagated = true
			case "wrong-device":
				mounts[0].device++
			case "missing-effective":
				actual = 123
			case "output-alias":
				mounts = append(mounts, metadataMount{id: 2, parent: 1, point: manifest.ReceiptPath})
			}
			err := metadataProtectedMountPolicy(manifest, mounts, false, func(string) (uint64, error) { return actual, nil })
			if (err == nil) != (kind == "valid") {
				t.Fatalf("%s: %v", kind, err)
			}
		})
	}
}

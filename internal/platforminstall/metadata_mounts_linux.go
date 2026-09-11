package platforminstall

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

type metadataMount struct {
	id, parent           uint64
	device               uint64
	root, point          string
	readonly, propagated bool
	namespaceRoot        bool
}

func metadataMountPath(encoded string) (string, error) {
	var out strings.Builder
	for index := 0; index < len(encoded); index++ {
		if encoded[index] != '\\' {
			out.WriteByte(encoded[index])
			continue
		}
		if index+3 >= len(encoded) {
			return "", fmt.Errorf("truncated mount path escape")
		}
		switch encoded[index : index+4] {
		case "\\040":
			out.WriteByte(' ')
		case "\\011":
			out.WriteByte('\t')
		case "\\012":
			out.WriteByte('\n')
		case "\\134":
			out.WriteByte('\\')
		default:
			return "", fmt.Errorf("invalid mount path escape")
		}
		index += 3
	}
	path := out.String()
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", fmt.Errorf("noncanonical mount path")
	}
	return path, nil
}

func metadataMountRoot(encoded, filesystem string) (string, bool, error) {
	if filepath.IsAbs(encoded) {
		root, err := metadataMountPath(encoded)
		return root, false, err
	}
	// nsfs's kernel d_dname is <namespace>:[<inode>], not a pathname.
	// Admit only these typed labels, without decoding or normalizing them;
	// mountpoints still use the strict canonical-path parser. Labels never
	// establish writable-parent or protected-directory authority.
	name, number, ok := strings.Cut(encoded, ":[")
	if filesystem != "nsfs" || !ok || !strings.HasSuffix(number, "]") {
		return "", false, fmt.Errorf("noncanonical mount root")
	}
	switch name {
	case "net", "mnt", "uts", "ipc", "pid", "user", "cgroup", "time":
	default:
		return "", false, fmt.Errorf("unsupported namespace mount root")
	}
	number = strings.TrimSuffix(number, "]")
	inode, err := strconv.ParseUint(number, 10, 64)
	if err != nil || inode == 0 || strconv.FormatUint(inode, 10) != number {
		return "", false, fmt.Errorf("invalid namespace mount root inode")
	}
	return encoded, true, nil
}

func metadataParseMounts(input io.Reader) ([]metadataMount, error) {
	limited := &io.LimitedReader{R: input, N: 4*1024*1024 + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), 128*1024)
	var mounts []metadataMount
	seen := map[uint64]bool{}
	for scanner.Scan() {
		if len(mounts) >= 16384 {
			return nil, fmt.Errorf("mount inventory exceeds bound")
		}
		fields := strings.Fields(scanner.Text())
		separator := -1
		for index, field := range fields {
			if field == "-" {
				separator = index
				break
			}
		}
		if separator < 6 || len(fields) != separator+4 {
			return nil, fmt.Errorf("malformed mountinfo record")
		}
		id, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil || id == 0 || seen[id] {
			return nil, fmt.Errorf("invalid or duplicate mount identity")
		}
		parent, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil || parent == 0 || parent == id {
			return nil, fmt.Errorf("invalid mount parent")
		}
		majorText, minorText, ok := strings.Cut(fields[2], ":")
		if !ok {
			return nil, fmt.Errorf("invalid mount device")
		}
		major, majorErr := strconv.ParseUint(majorText, 10, 32)
		minor, minorErr := strconv.ParseUint(minorText, 10, 32)
		if majorErr != nil || minorErr != nil {
			return nil, fmt.Errorf("invalid mount device")
		}
		root, namespaceRoot, err := metadataMountRoot(fields[3], fields[separator+1])
		if err != nil {
			return nil, fmt.Errorf("mount %d root: %w", id, err)
		}
		point, err := metadataMountPath(fields[4])
		if err != nil {
			return nil, fmt.Errorf("mount %d mountpoint: %w", id, err)
		}
		ro, rw := false, false
		for _, option := range strings.Split(fields[5], ",") {
			ro = ro || option == "ro"
			rw = rw || option == "rw"
		}
		if ro == rw {
			return nil, fmt.Errorf("ambiguous mount access flags")
		}
		propagated := false
		for _, option := range fields[6:separator] {
			propagated = propagated || strings.HasPrefix(option, "shared:") || strings.HasPrefix(option, "master:") || strings.HasPrefix(option, "propagate_from:")
		}
		mounts = append(mounts, metadataMount{id: id, parent: parent, device: unix.Mkdev(uint32(major), uint32(minor)), root: root, point: point, readonly: ro, propagated: propagated, namespaceRoot: namespaceRoot})
		seen[id] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if limited.N <= 0 || len(mounts) == 0 {
		return nil, fmt.Errorf("empty or oversized mount inventory")
	}
	return mounts, nil
}

func metadataPathMountID(path string) (uint64, error) {
	var info unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, path, unix.AT_SYMLINK_NOFOLLOW|unix.AT_NO_AUTOMOUNT, unix.STATX_MNT_ID, &info); err != nil {
		return 0, err
	}
	if info.Mask&unix.STATX_MNT_ID == 0 || info.Mnt_id == 0 {
		return 0, fmt.Errorf("kernel mount identity unavailable")
	}
	return info.Mnt_id, nil
}

func metadataProtectedMountPolicy(manifest Manifest, mounts []metadataMount, writer bool, mountID func(string) (uint64, error)) error {
	children, err := metadataProtectedChildren(manifest)
	if err != nil {
		return err
	}
	if len(children) == 0 {
		return nil
	}
	canonical := filepath.Dir(DefaultManifestPath(manifest.CityPath))
	var parentMount metadataMount
	// No writable output may itself be a bind alias to a protected object.
	// Apart from the declared protected children, output parents have no
	// descendant mounts at all, on the host or in W.
	for _, parent := range manifest.Metadata.Parents {
		for _, mount := range mounts {
			_, protected := children[mount.point]
			if mount.namespaceRoot && (mount.point == parent.Path || protected) {
				return fmt.Errorf("namespace mount cannot supply metadata directory authority")
			}
			if metadataContains(parent.Path, mount.point) && mount.point != parent.Path {
				if _, allowed := children[mount.point]; !allowed {
					return fmt.Errorf("undeclared mount beneath writable metadata parent")
				}
			}
		}
	}
	if writer {
		for _, parent := range manifest.Metadata.Parents {
			var exact metadataMount
			count := 0
			for _, mount := range mounts {
				if mount.point == parent.Path {
					exact = mount
					count++
				}
			}
			if count != 1 || exact.readonly || exact.propagated || exact.device != parent.Device {
				return fmt.Errorf("writable metadata mount is absent or differs")
			}
			actual, err := mountID(parent.Path)
			if err != nil || actual != exact.id {
				return fmt.Errorf("effective writable metadata mount differs")
			}
			if parent.Path == canonical {
				parentMount = exact
			}
		}
	}
	for path, tree := range children {
		var exact metadataMount
		count := 0
		for _, mount := range mounts {
			if mount.point == path {
				exact = mount
				count++
			}
			if metadataContains(path, mount.point) && mount.point != path {
				return fmt.Errorf("nested protected-tree mount refused")
			}
		}
		if count > 1 {
			return fmt.Errorf("stacked protected-tree mounts refused")
		}
		if writer {
			if count != 1 || !exact.readonly || exact.propagated || exact.parent != parentMount.id || exact.device != tree.Device {
				return fmt.Errorf("protected child mount contract differs")
			}
			actual, err := mountID(path)
			if err != nil || actual != exact.id {
				return fmt.Errorf("effective protected mount differs")
			}
		} else {
			// A frozen host directory may already be a mount, but must not accept
			// propagation that can introduce new descendants during the handoff.
			actual, err := mountID(path)
			if err != nil {
				return err
			}
			found := false
			for _, mount := range mounts {
				if mount.id == actual {
					found = true
					if mount.namespaceRoot || mount.propagated || mount.device != tree.Device {
						return fmt.Errorf("host protected mount differs or propagates")
					}
				}
			}
			if !found {
				return fmt.Errorf("host protected mount identity absent")
			}
		}
	}
	return nil
}

func metadataCheckProtectedMounts(manifest Manifest, writer bool) (returnErr error) {
	if manifest.Metadata == nil || len(manifest.Metadata.ProtectedTrees) == 0 {
		return nil
	}
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return err
	}
	defer func() { returnErr = errors.Join(returnErr, file.Close()) }()
	mounts, err := metadataParseMounts(file)
	if err != nil {
		return err
	}
	return metadataProtectedMountPolicy(manifest, mounts, writer, metadataPathMountID)
}

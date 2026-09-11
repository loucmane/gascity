package platforminstall

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func metadataProtectedIdentity(tree MetadataProtectedTree) (unix.Stat_t, error) {
	var info unix.Stat_t
	resolved, err := filepath.EvalSymlinks(tree.Pin.Path)
	if err != nil || resolved != tree.Pin.Path {
		return info, fmt.Errorf("protected tree path alias")
	}
	if err := unix.Lstat(tree.Pin.Path, &info); err != nil {
		return info, err
	}
	if info.Mode&unix.S_IFMT != unix.S_IFDIR || info.Dev != tree.Device || info.Ino != tree.Inode || info.Uid != tree.UID || info.Gid != tree.GID || info.Mode&0o7777 != tree.Pin.Mode {
		return info, fmt.Errorf("protected tree identity/owner/mode changed")
	}
	return info, nil
}

func metadataProtectedStable(before, after unix.Stat_t) bool {
	return before.Dev == after.Dev && before.Ino == after.Ino && before.Mode == after.Mode && before.Nlink == after.Nlink && before.Uid == after.Uid && before.Gid == after.Gid && before.Size == after.Size && before.Mtim == after.Mtim && before.Ctim == after.Ctim
}

// This separate alias walk leaves the existing content-digest contract intact.
// It uses no-atime directory descriptors and never follows symbolic links.
func metadataProtectedAliases(root string, parents []MetadataParent) error {
	writable := map[[2]uint64]bool{}
	for _, parent := range parents {
		writable[[2]uint64{parent.Device, parent.Inode}] = true
	}
	seen := map[[2]uint64]bool{}
	var visit func(string) error
	visit = func(path string) error {
		if len(seen) >= 100000 {
			return fmt.Errorf("protected tree entry bound exceeded")
		}
		var before unix.Stat_t
		if err := unix.Lstat(path, &before); err != nil {
			return err
		}
		identity := [2]uint64{before.Dev, before.Ino}
		if writable[identity] {
			return fmt.Errorf("protected tree aliases writable parent identity")
		}
		if seen[identity] {
			return fmt.Errorf("protected tree inode alias")
		}
		seen[identity] = true
		if before.Mode&0o7000 != 0 {
			return fmt.Errorf("protected tree special mode")
		}
		switch before.Mode & unix.S_IFMT {
		case unix.S_IFREG:
			if before.Nlink != 1 {
				return fmt.Errorf("protected tree hardlink")
			}
			return nil
		case unix.S_IFDIR:
			fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NOATIME|unix.O_CLOEXEC, 0)
			if err != nil {
				return err
			}
			directory := os.NewFile(uintptr(fd), path)
			var opened unix.Stat_t
			statErr := unix.Fstat(fd, &opened)
			if statErr != nil || !metadataProtectedStable(before, opened) {
				return errors.Join(fmt.Errorf("protected directory changed while opening"), statErr, directory.Close())
			}
			names, readErr := directory.Readdirnames(-1)
			closeErr := directory.Close()
			if err := errors.Join(readErr, closeErr); err != nil {
				return err
			}
			for _, name := range names {
				if err := visit(filepath.Join(path, name)); err != nil {
					return err
				}
			}
			var after unix.Stat_t
			if err := unix.Lstat(path, &after); err != nil {
				return err
			}
			if !metadataProtectedStable(before, after) {
				return fmt.Errorf("protected directory changed during traversal")
			}
			return nil
		default:
			return fmt.Errorf("protected tree contains unsupported file type")
		}
	}
	return visit(root)
}

func metadataCheckProtectedTree(tree MetadataProtectedTree, parents []MetadataParent) error {
	before, err := metadataProtectedIdentity(tree)
	if err != nil {
		return err
	}
	if err := metadataProtectedAliases(tree.Pin.Path, parents); err != nil {
		return err
	}
	if err := metadataCheckPin(tree.Pin, true); err != nil {
		return err
	}
	after, err := metadataProtectedIdentity(tree)
	if err != nil {
		return err
	}
	if !metadataProtectedStable(before, after) {
		return fmt.Errorf("protected root changed during inventory")
	}
	return nil
}

func metadataProtectedHostCheckpoint(manifest Manifest) error {
	if manifest.Metadata == nil || len(manifest.Metadata.ProtectedTrees) == 0 {
		return nil
	}
	if err := metadataCheckParents(manifest); err != nil {
		return err
	}
	return metadataCheckProtectedMounts(manifest, false)
}

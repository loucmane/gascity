package platforminstall

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Cache handling is selected only by the exact reviewed GC_HOME cache root.
func metadataCacheTreeDigest(root string) (string, error) {
	return metadataTreeDigestWithCacheLinks(root, true)
}

func metadataCheckInputTree(pin FilePin, launch *MetadataLaunch) error {
	if pin.Path == filepath.Join(launch.GCHome, "cache", "repos") {
		return metadataCheckPinWithTreeDigest(pin, true, metadataCacheTreeDigest)
	}
	return metadataCheckPin(pin, true)
}

// Cache links are immutable leaves, never traversal instructions. The supported
// pack shape is one relative link to a regular file beneath this same tree.
// Rejecting link chains also rejects paths that escape and later re-enter it.
func metadataCacheLeafLink(root, path string, info os.FileInfo) (string, error) {
	fd, err := unix.Open(path, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return "", err
	}
	defer func() { _ = unix.Close(fd) }()
	var before unix.Stat_t
	if err := unix.Fstat(fd, &before); err != nil || before.Mode&unix.S_IFMT != unix.S_IFLNK {
		return "", errors.Join(fmt.Errorf("cache link identity changed"), err)
	}
	opened, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, opened) || info.Mode() != opened.Mode() || info.Size() != opened.Size() || !info.ModTime().Equal(opened.ModTime()) {
		return "", errors.Join(fmt.Errorf("cache link changed before read"), err)
	}
	// Linux readlink may update the link's atime. It never changes link bytes,
	// ownership, mtime or ctime; those authority fields remain checked below.
	buffer := make([]byte, 4097)
	n, err := unix.Readlinkat(fd, "", buffer)
	if err != nil || n == 0 || n >= len(buffer) {
		return "", errors.Join(fmt.Errorf("cache link text invalid or oversized"), err)
	}
	target := string(buffer[:n])
	if filepath.IsAbs(target) {
		return "", fmt.Errorf("cache link must be relative")
	}
	current := filepath.Dir(path)
	components := strings.Split(target, string(filepath.Separator))
	for index, component := range components {
		switch component {
		case "", ".":
			continue
		case "..":
			if current == root {
				return "", fmt.Errorf("cache link escapes tree")
			}
			current = filepath.Dir(current)
		default:
			current = filepath.Join(current, component)
		}
		if !metadataContains(root, current) {
			return "", fmt.Errorf("cache link escapes tree")
		}
		entry, err := os.Lstat(current)
		if err != nil || (!entry.IsDir() && !entry.Mode().IsRegular()) {
			return "", errors.Join(fmt.Errorf("cache link chain or unsupported target"), err)
		}
		if index < len(components)-1 && !entry.IsDir() {
			return "", fmt.Errorf("cache link intermediate component is not directory")
		}
	}
	referent, err := os.Lstat(current)
	if err != nil || !referent.Mode().IsRegular() {
		return "", errors.Join(fmt.Errorf("cache link target is not regular"), err)
	}
	var after, named unix.Stat_t
	if err := unix.Fstat(fd, &after); err != nil {
		return "", err
	}
	if err := unix.Lstat(path, &named); err != nil {
		return "", err
	}
	if !metadataProtectedStable(before, after) || !metadataProtectedStable(before, named) {
		return "", fmt.Errorf("cache link changed during read")
	}
	return target, nil
}

package platforminstall

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Null is a fixed helper dependency, never a manifest-selected device grant.
// Its read-only device mount permits null I/O but not inode metadata changes.
func metadataNullStat(stat unix.Stat_t, host bool, uid, gid uint32) error {
	if stat.Mode != unix.S_IFCHR|0o666 || stat.Rdev != unix.Mkdev(1, 3) || stat.Nlink != 1 {
		return fmt.Errorf("fixed null device type/mode/identity differs")
	}
	if host {
		if stat.Uid != 0 || stat.Gid != 0 {
			return fmt.Errorf("host null device is not root-owned")
		}
	} else if stat.Uid == uid || stat.Gid == gid {
		// Host root is unmapped in W; do not invent an overflow UID/GID pin.
		return fmt.Errorf("writer owns fixed null device")
	}
	return nil
}

func metadataNullMountFlags(device, parent int64) error {
	if device&unix.ST_RDONLY == 0 || device&unix.ST_NODEV != 0 || parent&unix.ST_RDONLY == 0 {
		return fmt.Errorf("fixed null mount is writable, nodev, or has writable parent")
	}
	return nil
}

func metadataCheckNullDevice(host bool) error {
	resolved, err := filepath.EvalSymlinks("/dev/null")
	if err != nil || resolved != "/dev/null" {
		return fmt.Errorf("fixed null device is missing or aliased")
	}
	var stat unix.Stat_t
	if err := unix.Lstat("/dev/null", &stat); err != nil {
		return err
	}
	if err := metadataNullStat(stat, host, uint32(os.Geteuid()), uint32(os.Getegid())); err != nil {
		return err
	}
	if host {
		return metadataCheckEntropyDevice(true)
	}
	var device, parent unix.Statfs_t
	if err := unix.Statfs("/dev/null", &device); err != nil {
		return err
	}
	if err := unix.Statfs("/dev", &parent); err != nil {
		return err
	}
	if err := metadataNullMountFlags(device.Flags, parent.Flags); err != nil {
		return err
	}
	entries, err := os.ReadDir("/dev")
	if err != nil {
		return err
	}
	if len(entries) != 2 || entries[0].Name() != "null" || entries[1].Name() != "urandom" {
		return fmt.Errorf("writer device directory contains unexpected entries")
	}
	return metadataCheckEntropyDevice(false)
}

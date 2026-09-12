package platforminstall

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Entropy is a fixed helper dependency, never a caller-selected device grant.
// The digest-bound single-threaded setup enforces Landlock before exec; Go
// verifies identity and readonly inode/parent mounts without thread-local policy.
func metadataEntropyStat(st unix.Stat_t, host bool, uid, gid uint32) error {
	if st.Mode != unix.S_IFCHR|0o666 || st.Rdev != unix.Mkdev(1, 9) || st.Nlink != 1 {
		return fmt.Errorf("fixed entropy identity differs")
	}
	if host {
		if st.Uid != 0 || st.Gid != 0 {
			return fmt.Errorf("host entropy not root-owned")
		}
	} else if st.Uid == uid || st.Gid == gid {
		return fmt.Errorf("writer owns entropy")
	}
	return nil
}

func metadataCheckEntropyDevice(host bool) error {
	resolved, err := filepath.EvalSymlinks("/dev/urandom")
	if err != nil || resolved != "/dev/urandom" {
		return fmt.Errorf("fixed entropy device missing or aliased")
	}
	var st unix.Stat_t
	if err := unix.Lstat("/dev/urandom", &st); err != nil {
		return err
	}
	if err := metadataEntropyStat(st, host, uint32(os.Geteuid()), uint32(os.Getegid())); err != nil {
		return err
	}
	if host {
		return nil
	}
	var device, parent unix.Statfs_t
	if err := unix.Statfs("/dev/urandom", &device); err != nil {
		return err
	}
	if err := unix.Statfs("/dev", &parent); err != nil {
		return err
	}
	return metadataNullMountFlags(device.Flags, parent.Flags)
}

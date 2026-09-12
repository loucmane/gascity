package platforminstall

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// metadataCheckSetupFile retains exact regular-file, path, mode and digest
// checks, and excludes file capabilities from the unprivileged setup artifact.
func metadataCheckSetupFile(pin FilePin) error {
	if pin.Mode != 0o755 {
		return fmt.Errorf("metadata setup executable mode differs")
	}
	if err := metadataCheckPin(pin, false); err != nil {
		return err
	}
	if _, err := unix.Lgetxattr(pin.Path, "security.capability", nil); !errors.Is(err, unix.ENODATA) {
		return errors.Join(fmt.Errorf("metadata setup file capability must be absent"), err)
	}
	return nil
}

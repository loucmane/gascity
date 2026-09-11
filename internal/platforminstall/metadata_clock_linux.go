package platforminstall

import "golang.org/x/sys/unix"

func metadataMonotonicNow() (int64, error) {
	var value unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &value); err != nil {
		return 0, err
	}
	return value.Nano(), nil
}

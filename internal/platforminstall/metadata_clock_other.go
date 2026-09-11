//go:build !linux

package platforminstall

import "fmt"

func metadataMonotonicNow() (int64, error) {
	return 0, fmt.Errorf("metadata adoption lease requires Linux CLOCK_MONOTONIC")
}

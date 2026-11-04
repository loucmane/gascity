//go:build !linux

package providerallowlist

import (
	"fmt"
	"os"
)

func openApprovalFile(_ string) (*os.File, error) {
	return nil, fmt.Errorf("provider approval descriptors require Linux")
}

func openPresenceFile(path string) (*os.File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("provider presence input must be a regular file")
	}
	return os.Open(path)
}

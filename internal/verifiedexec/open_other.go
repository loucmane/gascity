//go:build !linux

package verifiedexec

import (
	"fmt"
	"os"
)

func openExecutable(_ string) (*os.File, error) {
	return nil, fmt.Errorf("verified executable descriptors require Linux procfs")
}

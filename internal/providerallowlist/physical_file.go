package providerallowlist

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ReadApprovalReceipt retains the protected receipt's physical file identity.
// Presence-only legacy detection is separate and does not authorize execution.
func ReadApprovalReceipt(city string) ([]byte, error) {
	return readPhysicalRegular(ReceiptPath(city), 16<<20)
}

func readPhysicalRegular(path string, limit int64) ([]byte, error) {
	physical, err := filepath.EvalSymlinks(path)
	if err != nil || physical != path {
		return nil, fmt.Errorf("provider approval path must be physical: %q", path)
	}
	file, err := openApprovalFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close() //nolint:errcheck // read-only descriptor
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("provider approval must be a regular file at most %d bytes", limit)
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("provider approval exceeds %d bytes", limit)
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, current) {
		return nil, fmt.Errorf("provider approval file changed during read")
	}
	physical, err = filepath.EvalSymlinks(path)
	if err != nil || physical != path {
		return nil, fmt.Errorf("provider approval path changed during read")
	}
	return data, nil
}

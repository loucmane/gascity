package providerallowlist

import (
	"fmt"
	"io"
)

// ReadPresenceDocument reads only an opened regular file's observed size.
// This is detection, never authorization: following a regular-file alias keeps
// its declaration sticky while the subsequent physical approval read refuses it.
// There is no new size limit on unrelated legacy documents.
func ReadPresenceDocument(path string) ([]byte, error) {
	file, err := openPresenceFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close() //nolint:errcheck // read-only descriptor
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("provider presence input must be a regular file")
	}
	return io.ReadAll(io.LimitReader(file, info.Size()))
}

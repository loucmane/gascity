package managedworker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gastownhall/gascity/internal/fsys"
)

// ReadControlPolicy reads a regular, non-group/world-writable file through a
// clean path with no symlink components. It checks metadata again after reading.
// The protected installation and worker sandbox remain the write boundary:
// path checks cannot reserve a file against a concurrent authorized host writer.
func ReadControlPolicy(files fsys.FS, path string) ([]byte, error) {
	if files == nil || !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Dir(path) == path {
		return nil, errors.New("control_policy requires a filesystem and clean absolute file path")
	}
	before, err := inspectPolicyPath(files, path)
	if err != nil {
		return nil, err
	}
	reader, ok := files.(interface{ ReadRegularFile(string) ([]byte, error) })
	if !ok {
		return nil, errors.New("control_policy requires a no-follow regular-file reader")
	}
	data, err := reader.ReadRegularFile(path)
	if err != nil {
		return nil, fmt.Errorf("control_policy read: %w", err)
	}
	after, err := inspectPolicyPath(files, path)
	if err != nil {
		return nil, err
	}
	if before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) ||
		int64(len(data)) != after.Size() || !fsys.SameFileIdentity(before, after) {
		return nil, errors.New("control_policy changed while reading")
	}
	return data, nil
}

func inspectPolicyPath(files fsys.FS, path string) (os.FileInfo, error) {
	info, err := files.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("control_policy inspect: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 {
		return nil, errors.New("control_policy must be a regular, non-group/world-writable file")
	}
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		directory, err := files.Lstat(parent)
		if err != nil {
			return nil, fmt.Errorf("control_policy parent %q: %w", parent, err)
		}
		if !directory.IsDir() || directory.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("control_policy parent %q must be a non-symlink directory", parent)
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	return info, nil
}

// Package verifiedexec binds a subprocess to an opened, verified executable.
package verifiedexec

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// Open verifies the resolved identity, mode and digest on the retained descriptor.
// A zero mode requires executable permission; a nonzero mode requires exact bits.
// The caller owns the returned descriptor and must retain it until the child exits.
func Open(path, resolved, digest string, mode os.FileMode) (*os.File, error) {
	actual, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("resolve executable: %w", err)
	}
	if actual != resolved {
		return nil, fmt.Errorf("resolved path mismatch: got %q want %q", actual, resolved)
	}
	file, err := openExecutable(resolved)
	if err != nil {
		return nil, fmt.Errorf("open executable: %w", err)
	}
	fail := func(err error) (*os.File, error) { _ = file.Close(); return nil, err }
	info, err := file.Stat()
	if err != nil {
		return fail(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 ||
		info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || (mode != 0 && info.Mode().Perm() != mode) {
		return fail(fmt.Errorf("unsafe executable mode %s", info.Mode()))
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fail(err)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != digest {
		return fail(fmt.Errorf("sha256 mismatch: got %q want %q", got, digest))
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fail(err)
	}
	return file, nil
}

// Bind makes cmd execute file through its inherited descriptor, preserving argv[0].
// It never reopens the original executable pathname.
func Bind(cmd *exec.Cmd, file *os.File) {
	cmd.Path = fmt.Sprintf("/proc/self/fd/%d", 3+len(cmd.ExtraFiles))
	cmd.ExtraFiles = append(cmd.ExtraFiles, file)
}

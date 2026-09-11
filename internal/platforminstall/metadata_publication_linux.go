package platforminstall

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type metadataPublication struct {
	installer  *installer
	parent     *os.File
	parentInfo os.FileInfo
	target     string
	stage      string
	preimage   string
	postimage  string
	mode       os.FileMode
	attempted  bool
}

func (i *installer) prepareMetadataPublication(target, stage string, data []byte, preimage string) (_ *metadataPublication, returnErr error) {
	const mode os.FileMode = 0o644
	if !filepath.IsAbs(target) || filepath.Clean(target) != target || filepath.Clean(stage) != stage || filepath.Dir(target) != filepath.Dir(stage) || target == stage || len(data) == 0 || len(data) > metadataFrameLimit {
		return nil, fmt.Errorf("metadata staging contract refused")
	}
	directory := filepath.Dir(target)
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil || resolved != directory {
		return nil, fmt.Errorf("metadata parent alias or absence: %w", err)
	}
	descriptor, err := unix.Open(directory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	parent := os.NewFile(uintptr(descriptor), directory)
	defer func() {
		if returnErr != nil {
			returnErr = errors.Join(returnErr, parent.Close())
		}
	}()
	info, err := parent.Stat()
	if err != nil {
		return nil, err
	}
	publication := &metadataPublication{i, parent, info, target, stage, preimage, sha256Hex(data), mode, false}
	if err := publication.checkPreimage(); err != nil {
		return nil, err
	}
	staged, err := os.OpenFile(publication.anchored(stage), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	count, writeErr := staged.Write(data)
	if writeErr == nil && count != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = staged.Chmod(mode)
	}
	if writeErr == nil {
		writeErr = staged.Sync()
	}
	if err := errors.Join(writeErr, staged.Close()); err != nil {
		return nil, err
	}
	if err := parent.Sync(); err != nil {
		return nil, err
	}
	return publication, nil
}

func (publication *metadataPublication) anchored(path string) string {
	return fmt.Sprintf("/proc/self/fd/%d/%s", publication.parent.Fd(), filepath.Base(path))
}

func (publication *metadataPublication) checkParent() error {
	current, err := os.Lstat(filepath.Dir(publication.target))
	if err != nil {
		return err
	}
	if !current.IsDir() || !os.SameFile(publication.parentInfo, current) {
		return fmt.Errorf("metadata parent identity changed")
	}
	return nil
}

func (publication *metadataPublication) checkPreimage() error {
	if err := publication.checkParent(); err != nil {
		return err
	}
	return metadataImageMatches(publication.anchored(publication.target), publication.preimage, 0)
}

func metadataImageMatches(path, expected string, mode os.FileMode) (returnErr error) {
	descriptor, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if errors.Is(err, unix.ENOENT) && expected == "" {
		return nil
	}
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(descriptor), path)
	defer func() { returnErr = errors.Join(returnErr, file.Close()) }()
	var stat unix.Stat_t
	if err := unix.Fstat(descriptor, &stat); err != nil {
		return err
	}
	if err := metadataRestorableImage(stat, expected, mode, uint32(os.Geteuid()), uint32(os.Getegid())); err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(file, metadataFrameLimit+1))
	if err != nil || int64(len(data)) != stat.Size || sha256Hex(data) != expected {
		return fmt.Errorf("metadata image changed: %w", err)
	}
	return nil
}

func metadataRestorableImage(stat unix.Stat_t, expected string, mode os.FileMode, uid, gid uint32) error {
	if mode == 0 {
		mode = 0o644
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || stat.Uid != uid || stat.Gid != gid || stat.Size < 0 || stat.Size > metadataFrameLimit || expected == "" {
		return fmt.Errorf("metadata inode type, links, owner/group, size or preimage refused")
	}
	if stat.Mode&0o7777 != uint32(mode) || mode != 0o644 {
		return fmt.Errorf("metadata preimage requires exact 0644 restoration form")
	}
	return nil
}

func (publication *metadataPublication) close() error {
	if publication.parent == nil {
		return nil
	}
	err := publication.parent.Close()
	publication.parent = nil
	return err
}

func (publication *metadataPublication) publish() (renamed bool, returnErr error) {
	if publication.attempted || publication.parent == nil {
		return false, fmt.Errorf("metadata publication is single-use")
	}
	publication.attempted = true
	defer func() { returnErr = errors.Join(returnErr, publication.close()) }()
	if err := publication.checkPreimage(); err != nil {
		return false, err
	}
	if err := metadataImageMatches(publication.anchored(publication.stage), publication.postimage, publication.mode); err != nil {
		return false, err
	}
	if err := publication.installer.rename(publication.anchored(publication.stage), publication.anchored(publication.target)); err != nil {
		return false, err
	}
	if err := publication.installer.syncDir(fmt.Sprintf("/proc/self/fd/%d", publication.parent.Fd())); err != nil {
		return true, err
	}
	if err := publication.checkParent(); err != nil {
		return true, err
	}
	return true, metadataImageMatches(publication.anchored(publication.target), publication.postimage, publication.mode)
}

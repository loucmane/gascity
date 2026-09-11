package platforminstall

import (
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/gastownhall/gascity/internal/importsvc"
	"golang.org/x/sys/unix"
)

const metadataBwrapSHA = "52231e1caf55bcbc667b269f49c63599a6f7db4767ae6a039580d0ff853db712"

var metadataRuntimePaths = []string{"/usr/bin/bwrap", "/usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2", "/usr/lib/x86_64-linux-gnu/libcap.so.2", "/usr/lib/x86_64-linux-gnu/libc.so.6", "/usr/lib/x86_64-linux-gnu/libselinux.so.1", "/usr/lib/x86_64-linux-gnu/libpcre2-8.so.0"}

func metadataContains(root, path string) bool {
	return path == root || strings.HasPrefix(path, root+"/")
}

func metadataTreeDigest(root string) (string, error) {
	hash := sha256.New()
	count := 0
	var visit func(string) error
	visit = func(path string) error {
		count++
		if count > 100000 {
			return fmt.Errorf("metadata input tree exceeds entry bound")
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("metadata tree contains link/device/socket: %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(struct {
			Path string
			Mode uint32
			Size int64
		}{relative, uint32(info.Mode()), info.Size()})
		if err != nil {
			return err
		}
		if _, err := hash.Write(encoded); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			descriptor, err := unix.Open(path, unix.O_RDONLY|unix.O_NOATIME|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				return err
			}
			file := os.NewFile(uintptr(descriptor), path)
			opened, statErr := file.Stat()
			if statErr != nil || !os.SameFile(info, opened) {
				return errors.Join(fmt.Errorf("input inode changed"), statErr, file.Close())
			}
			fileHash := sha256.New()
			_, readErr := io.Copy(fileHash, file)
			closeErr := file.Close()
			if err := errors.Join(readErr, closeErr); err != nil {
				return err
			}
			if _, err := hash.Write(fileHash.Sum(nil)); err != nil {
				return err
			}
		} else {
			descriptor, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOATIME|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				return err
			}
			directory := os.NewFile(uintptr(descriptor), path)
			opened, statErr := directory.Stat()
			if statErr != nil || !os.SameFile(info, opened) {
				return errors.Join(fmt.Errorf("input directory changed"), statErr, directory.Close())
			}
			names, readErr := directory.Readdirnames(-1)
			closeErr := directory.Close()
			if err := errors.Join(readErr, closeErr); err != nil {
				return err
			}
			sort.Strings(names)
			for _, name := range names {
				if err := visit(filepath.Join(path, name)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	err := visit(root)
	return hex.EncodeToString(hash.Sum(nil)), err
}

func metadataCheckPin(pin FilePin, tree bool) error {
	if !filepath.IsAbs(pin.Path) || filepath.Clean(pin.Path) != pin.Path || pin.Path == "/" {
		return fmt.Errorf("noncanonical metadata input")
	}
	resolved, err := filepath.EvalSymlinks(pin.Path)
	if err != nil || resolved != pin.Path {
		return fmt.Errorf("aliased metadata input %s", pin.Path)
	}
	info, err := os.Lstat(pin.Path)
	if err != nil {
		return err
	}
	if uint32(info.Mode().Perm()) != pin.Mode || info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		return fmt.Errorf("input mode drift %s", pin.Path)
	}
	if err := validateSHA256("input digest", pin.SHA256); err != nil {
		return err
	}
	var actual string
	if tree {
		if !info.IsDir() {
			return fmt.Errorf("input tree is not directory")
		}
		actual, err = metadataTreeDigest(pin.Path)
	} else {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("input is not regular")
		}
		actual, err = digestRegularFile(pin.Path)
	}
	if err != nil {
		return err
	}
	if actual != pin.SHA256 {
		return fmt.Errorf("input digest drift %s", pin.Path)
	}
	return nil
}

func metadataCheckRuntimePin(pin FilePin, launch *MetadataLaunch) error {
	resolved, err := filepath.EvalSymlinks(pin.Path)
	if err != nil {
		return err
	}
	if resolved != pin.Path {
		declared := false
		for _, link := range launch.Links {
			if link.Path == pin.Path {
				actual, err := os.Readlink(link.Path)
				if err != nil || actual != link.Target {
					return fmt.Errorf("runtime link changed")
				}
				declared = metadataReadCovered(launch, resolved)
			}
		}
		if !declared {
			return fmt.Errorf("undeclared runtime link")
		}
		pin.Path = resolved
	}
	return metadataCheckPin(pin, false)
}

func metadataCheckFDs(channel bool) (returnErr error) {
	directory, err := os.Open("/proc/self/fd")
	if err != nil {
		return err
	}
	defer func() { returnErr = errors.Join(returnErr, directory.Close()) }()
	self := int(directory.Fd())
	names, readErr := directory.Readdirnames(-1)
	if readErr != nil {
		return readErr
	}
	for _, name := range names {
		descriptor, err := strconv.Atoi(name)
		if err != nil {
			return err
		}
		if descriptor == self {
			continue
		}
		target, err := os.Readlink("/proc/self/fd/" + name)
		if err != nil {
			return err
		}
		if descriptor <= 2 {
			continue
		}
		if channel && (descriptor == 3 || descriptor == 4) {
			if !strings.HasPrefix(target, "pipe:[") {
				return fmt.Errorf("metadata channel is not anonymous pipe")
			}
			continue
		}
		if target != "anon_inode:[eventpoll]" && target != "anon_inode:[eventfd]" {
			return fmt.Errorf("unexpected metadata descriptor %d", descriptor)
		}
	}
	return nil
}

func metadataSandboxArgv(manifest Manifest) ([]string, error) {
	if err := validateMetadataLaunch(manifest); err != nil {
		return nil, err
	}
	launch := manifest.Metadata
	if len(launch.Runtime) != len(metadataRuntimePaths) {
		return nil, fmt.Errorf("runtime closure differs")
	}
	for index, pin := range launch.Runtime {
		if pin.Path != metadataRuntimePaths[index] {
			return nil, fmt.Errorf("runtime path differs")
		}
	}
	if launch.Runtime[0].SHA256 != metadataBwrapSHA {
		return nil, fmt.Errorf("bwrap identity differs")
	}
	args := []string{metadataRuntimePaths[1], "--inhibit-cache", "--glibc-hwcaps-mask", "", "--library-path", "/usr/lib/x86_64-linux-gnu", "/usr/bin/bwrap", "--unshare-user", "--unshare-pid", "--as-pid-1", "--unshare-net", "--unshare-ipc", "--unshare-uts", "--die-with-parent", "--new-session", "--cap-drop", "ALL", "--preserve-fds", "2", "--clearenv", "--setenv", "GODEBUG", "containermaxprocs=0", "--setenv", "GC_HOME", launch.GCHome, "--setenv", "HOME", "/nonexistent", "--setenv", "PATH", "/usr/bin:/bin", "--ro-bind", launch.Writer.Path, "/metadata-writer"}
	seen := map[string]bool{}
	for _, pin := range append(append([]FilePin{}, launch.Inputs...), launch.Trees...) {
		for _, forbidden := range []string{"/proc", "/sys", "/dev", "/run"} {
			if metadataContains(forbidden, pin.Path) {
				return nil, fmt.Errorf("host authority mount refused")
			}
		}
		if seen[pin.Path] || pin.Path == "/" || pin.Path == launch.GCHome || pin.Path == manifest.CityPath || metadataContains(pin.Path, "/proc") || metadataContains(pin.Path, "/dev") || metadataContains(pin.Path, launch.GCHome) {
			return nil, fmt.Errorf("broad or duplicate metadata mount %s", pin.Path)
		}
		seen[pin.Path] = true
		args = append(args, "--ro-bind", pin.Path, pin.Path)
	}
	for _, link := range launch.Links {
		if seen[link.Path] || !filepath.IsAbs(link.Path) || filepath.Clean(link.Path) != link.Path {
			return nil, fmt.Errorf("invalid metadata symlink")
		}
		seen[link.Path] = true
		args = append(args, "--symlink", link.Target, link.Path)
	}
	parents := map[string]bool{launch.Evidence: true}
	for _, output := range metadataOutputs(manifest) {
		parents[filepath.Dir(output)] = true
	}
	ordered := make([]string, 0, len(parents))
	for parent := range parents {
		ordered = append(ordered, parent)
	}
	sort.Strings(ordered)
	cache := filepath.Join(launch.GCHome, "cache", "repos")
	if !seen[cache] {
		return nil, fmt.Errorf("exact cache tree not mounted")
	}
	for _, parent := range ordered {
		if parent == "/" || parent == manifest.CityPath || metadataContains(parent, cache) || metadataContains(cache, parent) {
			return nil, fmt.Errorf("metadata writable parent overlaps protected authority")
		}
		for _, pin := range append(append([]FilePin{}, launch.Inputs...), launch.Trees...) {
			if metadataContains(parent, pin.Path) || metadataContains(pin.Path, parent) {
				return nil, fmt.Errorf("writable and readonly mounts overlap")
			}
		}
		args = append(args, "--bind", parent, parent)
	}
	args = append(args, "--proc", "/proc", "--remount-ro", "/proc", "--remount-ro", "/", "--chdir", launch.Evidence, "--", "/metadata-writer", "__metadata-writer-v1")
	return args, nil
}

func validateMetadataInputs(manifest Manifest, host bool) (returnErr error) {
	launch := manifest.Metadata
	if launch == nil {
		return fmt.Errorf("missing metadata launch")
	}
	if err := metadataCheckParents(manifest, true); err != nil {
		return err
	}
	for _, pin := range launch.Inputs {
		if err := metadataCheckPin(pin, false); err != nil {
			return err
		}
	}
	for _, pin := range launch.Trees {
		if err := metadataCheckPin(pin, true); err != nil {
			return err
		}
	}
	for _, path := range launch.Absent {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("frozen absence changed: %s", path)
		}
	}
	for _, link := range launch.Links {
		target, err := os.Readlink(link.Path)
		if err != nil || target != link.Target {
			return fmt.Errorf("metadata symlink drift")
		}
		resolved, err := filepath.EvalSymlinks(link.Path)
		if err != nil || !metadataReadCovered(launch, resolved) {
			return fmt.Errorf("metadata symlink escapes input closure")
		}
	}
	cache := filepath.Join(launch.GCHome, "cache", "repos")
	found := false
	for _, pin := range launch.Trees {
		if pin.Path == cache && pin.SHA256 == launch.CacheSHA256 {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("cache digest unbound")
	}
	imports, err := importsvc.CollectAllImports(metadataInputFS{launch: launch}, manifest.CityPath)
	if err != nil {
		return fmt.Errorf("frozen import input closure: %w", err)
	}
	importsData, err := json.Marshal(imports)
	if err != nil {
		return err
	}
	if sha256Hex(importsData) != launch.ImportsSHA256 {
		return fmt.Errorf("host/writer import graph digest differs")
	}
	if host {
		for _, pin := range launch.Runtime {
			if err := metadataCheckRuntimePin(pin, launch); err != nil {
				return err
			}
		}
		if err := metadataCheckPin(launch.Writer, false); err != nil {
			return err
		}
		image, err := elf.Open(launch.Writer.Path)
		if err != nil {
			return err
		}
		defer func() { returnErr = errors.Join(returnErr, image.Close()) }()
		for _, program := range image.Progs {
			if program.Type == elf.PT_INTERP {
				return fmt.Errorf("metadata writer must be static")
			}
		}
		if _, err := os.Lstat("/etc/ld.so.preload"); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("ambient loader preload refused")
		}
	}
	return nil
}

func metadataCheckParents(manifest Manifest, contents bool) error {
	wanted := map[string]bool{manifest.Metadata.Evidence: true}
	for _, path := range metadataOutputs(manifest) {
		wanted[filepath.Dir(path)] = true
	}
	if len(wanted) != len(manifest.Metadata.Parents) {
		return fmt.Errorf("metadata parent set differs")
	}
	for _, parent := range manifest.Metadata.Parents {
		if !wanted[parent.Path] {
			return fmt.Errorf("unexpected metadata parent")
		}
		delete(wanted, parent.Path)
		resolved, err := filepath.EvalSymlinks(parent.Path)
		if err != nil || resolved != parent.Path {
			return fmt.Errorf("metadata parent alias")
		}
		var stat unix.Stat_t
		if err := unix.Lstat(parent.Path, &stat); err != nil {
			return err
		}
		if stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Dev != parent.Device || stat.Ino != parent.Inode || stat.Uid != parent.UID || stat.Gid != parent.GID || stat.Mode&0o7777 != parent.Mode || stat.Uid != uint32(os.Geteuid()) || parent.Mode&0o022 != 0 {
			return fmt.Errorf("metadata parent identity/owner/mode changed")
		}
		if contents {
			entries, err := os.ReadDir(parent.Path)
			if err != nil {
				return err
			}
			allowed := map[string]bool{}
			for _, path := range metadataOutputs(manifest) {
				if filepath.Dir(path) == parent.Path {
					allowed[filepath.Base(path)] = true
					allowed[filepath.Base(metadataStage(manifest, path))] = true
					allowed[filepath.Base(metadataStage(manifest, path))+".rollback"] = true
				}
			}
			if parent.Path == manifest.Metadata.Evidence {
				for _, name := range []string{"intent.json", "final.json"} {
					allowed[name] = true
					allowed[filepath.Base(metadataStage(manifest, filepath.Join(parent.Path, name)))] = true
				}
			}
			present := map[string]bool{}
			for _, entry := range entries {
				if !allowed[entry.Name()] {
					return fmt.Errorf("unexpected metadata parent entry")
				}
				var entryStat unix.Stat_t
				if err := unix.Lstat(filepath.Join(parent.Path, entry.Name()), &entryStat); err != nil {
					return err
				}
				if entryStat.Mode&unix.S_IFMT != unix.S_IFREG || entryStat.Nlink != 1 || entryStat.Uid != uint32(os.Geteuid()) {
					return fmt.Errorf("metadata output alias/type/owner refused")
				}
				present[entry.Name()] = true
			}
			for _, name := range parent.Entries {
				if !present[name] {
					return fmt.Errorf("frozen metadata parent entry disappeared")
				}
			}
		}
	}
	return nil
}

func metadataReadCovered(launch *MetadataLaunch, path string) bool {
	for _, pin := range launch.Inputs {
		if pin.Path == path {
			return true
		}
	}
	for _, pin := range launch.Trees {
		if metadataContains(pin.Path, path) {
			return true
		}
	}
	return false
}

type metadataInputFS struct {
	fsys.OSFS
	launch *MetadataLaunch
}

func metadataImportFS(launch *MetadataLaunch) fsys.FS { return metadataInputFS{launch: launch} }
func (input metadataInputFS) allowed(path string) error {
	if metadataReadCovered(input.launch, path) {
		return nil
	}
	for _, link := range input.launch.Links {
		if link.Path == path {
			return nil
		}
	}
	for _, absent := range input.launch.Absent {
		if absent == path {
			return nil
		}
	}
	return fmt.Errorf("undeclared metadata input %s (not an absence)", path)
}

func (input metadataInputFS) ReadFile(path string) ([]byte, error) {
	if err := input.allowed(path); err != nil {
		return nil, err
	}
	descriptor, err := unix.Open(path, unix.O_RDONLY|unix.O_NOATIME|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(descriptor), path)
	data, readErr := io.ReadAll(io.LimitReader(file, metadataFrameLimit+1))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, err
	}
	if len(data) > metadataFrameLimit {
		return nil, fmt.Errorf("metadata import input exceeds frame limit")
	}
	return data, nil
}

func (input metadataInputFS) Stat(path string) (os.FileInfo, error) {
	if err := input.allowed(path); err != nil {
		return nil, err
	}
	return os.Stat(path)
}

func (input metadataInputFS) Lstat(path string) (os.FileInfo, error) {
	if err := input.allowed(path); err != nil {
		return nil, err
	}
	return os.Lstat(path)
}

func (input metadataInputFS) ReadDir(path string) ([]os.DirEntry, error) {
	if err := input.allowed(path); err != nil {
		return nil, err
	}
	descriptor, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOATIME|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	directory := os.NewFile(uintptr(descriptor), path)
	entries, readErr := directory.ReadDir(-1)
	closeErr := directory.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, err
	}
	sort.Slice(entries, func(first, second int) bool { return entries[first].Name() < entries[second].Name() })
	return entries, nil
}

func metadataWriterBoundary(manifest Manifest) error {
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return err
	}
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		return err
	}
	if os.Getpid() != 1 || os.Geteuid() == 0 || os.Getenv("GODEBUG") != "containermaxprocs=0" || os.Getenv("GC_HOME") != manifest.Metadata.GCHome {
		return fmt.Errorf("confined writer identity/environment refused")
	}
	if len(os.Environ()) != 4 || os.Getenv("HOME") != "/nonexistent" || os.Getenv("PATH") != "/usr/bin:/bin" {
		return fmt.Errorf("unexpected writer environment")
	}
	for space, host := range manifest.Metadata.Namespaces {
		actual, err := os.Readlink("/proc/self/ns/" + space)
		if err != nil {
			return err
		}
		if space == "time" {
			if actual != host {
				return fmt.Errorf("monotonic domain changed")
			}
		} else if actual == host {
			return fmt.Errorf("writer retains host namespace %s", space)
		}
	}
	if err := metadataCheckFDs(true); err != nil {
		return err
	}
	image, err := os.ReadFile("/proc/self/exe")
	if err != nil {
		return err
	}
	if sha256Hex(image) != manifest.Metadata.Writer.SHA256 {
		return fmt.Errorf("actual writer executable differs")
	}
	if err := metadataCheckParents(manifest, true); err != nil {
		return err
	}
	for _, descriptor := range []int{3, 4} {
		unix.CloseOnExec(descriptor)
	}
	for _, pin := range append(append([]FilePin{}, manifest.Metadata.Inputs...), manifest.Metadata.Trees...) {
		var stat unix.Statfs_t
		if err := unix.Statfs(pin.Path, &stat); err != nil {
			return err
		}
		if stat.Flags&unix.ST_RDONLY == 0 {
			return fmt.Errorf("input mount is writable")
		}
	}
	return validateMetadataInputs(manifest, false)
}

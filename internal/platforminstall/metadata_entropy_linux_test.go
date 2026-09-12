package platforminstall

import (
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestMetadataEntropyProjection(t *testing.T) {
	args, err := metadataSandboxArgv(metadataContractFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, "\x00")
	want := "--ro-bind-null\x00--ro-bind-urandom\x00--proc\x00/proc\x00"
	if strings.Count(joined, want) != 1 {
		t.Fatal("fixed devices must precede readonly seal")
	}
	for _, path := range []string{"/dev/urandom", "/dev/random"} {
		m := metadataContractFixture(t)
		m.Metadata.Inputs = append(m.Metadata.Inputs, FilePin{Path: path, Mode: 0o666, SHA256: strings.Repeat("a", 64)})
		if _, err := metadataSandboxArgv(m); err == nil {
			t.Fatal("manifest-selected entropy accepted")
		}
	}
}

func TestMetadataEntropyIdentity(t *testing.T) {
	valid := unix.Stat_t{Mode: unix.S_IFCHR | 0o666, Rdev: unix.Mkdev(1, 9), Nlink: 1}
	if err := metadataEntropyStat(valid, true, 1000, 1000); err != nil {
		t.Fatal(err)
	}
	for _, owner := range []uint32{65534, 12345} {
		mapped := valid
		mapped.Uid = owner
		mapped.Gid = owner
		if err := metadataEntropyStat(mapped, false, 1000, 1000); err != nil {
			t.Fatal(err)
		}
		if err := metadataEntropyStat(mapped, true, 1000, 1000); err == nil {
			t.Fatal("unmapped ID accepted as host root")
		}
	}
	for _, kind := range []string{"regular", "symlink", "rdev", "mode", "setuid", "nlink", "uid", "gid"} {
		for _, host := range []bool{true, false} {
			bad := valid
			switch kind {
			case "regular":
				bad.Mode = unix.S_IFREG | 0o666
			case "symlink":
				bad.Mode = unix.S_IFLNK | 0o666
			case "rdev":
				bad.Rdev = unix.Mkdev(1, 8)
			case "mode":
				bad.Mode = unix.S_IFCHR | 0o600
			case "setuid":
				bad.Mode |= unix.S_ISUID
			case "nlink":
				bad.Nlink = 2
			case "uid":
				bad.Uid = 1000
			case "gid":
				bad.Gid = 1000
			}
			if err := metadataEntropyStat(bad, host, 1000, 1000); err == nil {
				t.Fatalf("invalid entropy accepted %s host=%v", kind, host)
			}
		}
	}
}

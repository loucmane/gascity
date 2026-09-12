package platforminstall

import (
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestMetadataNullDeviceProjection(t *testing.T) {
	args, err := metadataSandboxArgv(metadataContractFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, "\x00")
	want := "--ro-bind-null\x00"
	if strings.Count(joined, want) != 1 || strings.Index(joined, want) > strings.Index(joined, "--remount-ro\x00/\x00") {
		t.Fatal("fixed null-only device bind must precede root readonly seal")
	}
	if strings.Contains(joined, "--dev-bind\x00") || strings.Contains(joined, "--dev\x00") || strings.Contains(joined, "--remount-ro\x00/dev/null") {
		t.Fatal("general device exposure")
	}
}

func TestMetadataPinnedSetupArtifactMayRelocateWithoutChangingAuthority(t *testing.T) {
	m := metadataContractFixture(t)
	m.Metadata.Runtime[0].Path = "/fixture/reviewed-bwrap"
	m.Metadata.Runtime[0].Mode = 0o755
	m.Metadata.Inputs[len(m.Metadata.Inputs)-1] = m.Metadata.Runtime[0]
	args, err := metadataSandboxArgv(m)
	if err != nil {
		t.Fatal(err)
	}
	if args[6] != m.Metadata.Runtime[0].Path || strings.Contains(strings.Join(args, "\x00"), "/usr/bin/bwrap") {
		t.Fatal("setup must execute the exact pinned relocated artifact")
	}
	for _, kind := range []string{"digest", "path", "mode", "missing-closure", "closure-drift", "library-path"} {
		t.Run(kind, func(t *testing.T) {
			bad := metadataContractFixture(t)
			switch kind {
			case "digest":
				bad.Metadata.Runtime[0].SHA256 = strings.Repeat("a", 64)
			case "path":
				bad.Metadata.Runtime[0].Path = "/fixture/../setup"
			case "mode":
				bad.Metadata.Runtime[0].Mode = 0o777
			case "missing-closure":
				bad.Metadata.Inputs = bad.Metadata.Inputs[:len(bad.Metadata.Inputs)-1]
			case "closure-drift":
				bad.Metadata.Inputs[len(bad.Metadata.Inputs)-1].SHA256 = strings.Repeat("b", 64)
			case "library-path":
				bad.Metadata.Runtime[1].Path = "/fixture/other-loader"
			}
			if _, err := metadataSandboxArgv(bad); err == nil {
				t.Fatal("unreviewed setup authority accepted")
			}
		})
	}
}

func TestMetadataNullIdentityPolicy(t *testing.T) {
	valid := unix.Stat_t{Mode: unix.S_IFCHR | 0o666, Rdev: unix.Mkdev(1, 3), Nlink: 1}
	if err := metadataNullStat(valid, true, 1000, 1000); err != nil {
		t.Fatal(err)
	}
	for _, owner := range []uint32{65534, 12345} {
		mapped := valid
		mapped.Uid, mapped.Gid = owner, owner
		if err := metadataNullStat(mapped, false, 1000, 1000); err != nil {
			t.Fatal("unmapped host owner need not be a hardcoded overflow ID", err)
		}
	}
	for _, kind := range []string{"regular", "symlink", "other-device", "mode", "setuid", "setgid", "sticky", "links", "uid", "gid"} {
		for _, host := range []bool{true, false} {
			bad := valid
			switch kind {
			case "regular":
				bad.Mode = unix.S_IFREG | 0o666
			case "symlink":
				bad.Mode = unix.S_IFLNK | 0o666
			case "other-device":
				bad.Rdev = unix.Mkdev(1, 5)
			case "mode":
				bad.Mode = unix.S_IFCHR | 0o660
			case "setuid":
				bad.Mode |= unix.S_ISUID
			case "setgid":
				bad.Mode |= unix.S_ISGID
			case "sticky":
				bad.Mode |= unix.S_ISVTX
			case "links":
				bad.Nlink = 2
			case "uid":
				bad.Uid = 1000
			case "gid":
				bad.Gid = 1000
			}
			if err := metadataNullStat(bad, host, 1000, 1000); err == nil {
				t.Fatalf("invalid null accepted: %s host=%v", kind, host)
			}
		}
	}
}

func TestMetadataNullMountPolicy(t *testing.T) {
	for _, row := range []struct {
		device, parent int64
		valid          bool
	}{{unix.ST_RDONLY, unix.ST_RDONLY | unix.ST_NODEV, true}, {0, unix.ST_RDONLY, false}, {unix.ST_RDONLY | unix.ST_NODEV, unix.ST_RDONLY, false}, {unix.ST_RDONLY, 0, false}} {
		err := metadataNullMountFlags(row.device, row.parent)
		if (err == nil) != row.valid {
			t.Fatalf("null mount policy mismatch: %+v %v", row, err)
		}
	}
}

func TestMetadataDeviceInputStillRefused(t *testing.T) {
	for _, path := range []string{"/dev", "/dev/null", "/dev/zero", "/dev/fd/1"} {
		for _, tree := range []bool{false, true} {
			m := metadataContractFixture(t)
			pin := FilePin{Path: path, Mode: 0o666, SHA256: strings.Repeat("a", 64)}
			if tree {
				m.Metadata.Trees = append(m.Metadata.Trees, pin)
			} else {
				m.Metadata.Inputs = append(m.Metadata.Inputs, pin)
			}
			if _, err := metadataSandboxArgv(m); err == nil {
				t.Fatalf("manifest device mount accepted: %s tree=%v", path, tree)
			}
		}
	}
}

package main

import (
	"encoding/binary"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func fixtureACL(mask uint16) []byte {
	data := make([]byte, 44)
	binary.LittleEndian.PutUint32(data, 2)
	entries := []struct {
		tag, permissions uint16
		id               uint32
	}{{1, 6, 0xffffffff}, {2, 4, 65534}, {4, 0, 0xffffffff}, {16, mask, 0xffffffff}, {32, 0, 0xffffffff}}
	for index, entry := range entries {
		offset := 4 + index*8
		binary.LittleEndian.PutUint16(data[offset:], entry.tag)
		binary.LittleEndian.PutUint16(data[offset+2:], entry.permissions)
		binary.LittleEndian.PutUint32(data[offset+4:], entry.id)
	}
	return data
}

func prepareACL(root string) error {
	if err := os.WriteFile(root+"/acl", []byte("fixture ACL"), 0o600); err != nil {
		return err
	}
	if err := os.Mkdir(root+"/acl-dir", 0o700); err != nil {
		return err
	}
	for _, target := range []struct{ path, name string }{{root + "/acl", "system.posix_acl_access"}, {root + "/acl-dir", "system.posix_acl_default"}} {
		if err := unix.Setxattr(target.path, target.name, fixtureACL(4), 0); err != nil {
			return fmt.Errorf("supported ACL positive unavailable: %w", err)
		}
	}
	return nil
}

func aclChanges(root string) map[string]attempt {
	return map[string]attempt{
		"access":             observe(func() error { return unix.Setxattr(root+"/acl", "system.posix_acl_access", fixtureACL(0), 0) }),
		"descendant_default": observe(func() error { return unix.Setxattr(root+"/acl-dir", "system.posix_acl_default", fixtureACL(0), 0) }),
	}
}

func validateACL(values map[string]attempt, positive bool) error {
	if len(values) != 2 {
		return fmt.Errorf("ACL inventory incomplete")
	}
	for _, name := range []string{"access", "descendant_default"} {
		value, ok := values[name]
		if !ok {
			return fmt.Errorf("ACL witness missing")
		}
		if positive {
			if !value.OK || value.Errno != 0 {
				return fmt.Errorf("ACL positive unsupported/inconclusive: %s: %s", name, value.Error)
			}
		} else if value.OK || (value.Errno != 1 && value.Errno != 13 && value.Errno != 30) {
			return fmt.Errorf("ACL denial inconclusive: %s", name)
		}
	}
	return nil
}

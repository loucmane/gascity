//go:build linux

package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
)

const (
	runRoot    = "/tmp/ga-tmgr-m5-run-r3-20260910"
	binaryPath = "/tmp/ga-tmgr-m5-candidate-r3-20260910/m5probe"
	bwrapSHA   = "52231e1caf55bcbc667b269f49c63599a6f7db4767ae6a039580d0ff853db712"
)

var dependencyPaths = []string{
	"/usr/bin/bwrap", binaryPath,
	"/usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2",
	"/usr/lib/x86_64-linux-gnu/libcap.so.2",
	"/usr/lib/x86_64-linux-gnu/libc.so.6",
	"/usr/lib/x86_64-linux-gnu/libselinux.so.1",
	"/usr/lib/x86_64-linux-gnu/libpcre2-8.so.0",
}

var mutationNames = []string{"create", "overwrite", "truncate", "unlink", "rename", "mkdir", "symlink", "chmod", "utime", "descendant_thread", "descendant_process", "chown", "xattr"}

type pin struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type plan struct {
	Schema         int      `json:"schema"`
	RunRoot        string   `json:"run_root"`
	Binary         string   `json:"binary"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	Pins           []pin    `json:"pins"`
	Argv           []string `json:"argv"`
}

type attempt struct {
	OK          bool   `json:"ok"`
	Errno       int    `json:"errno"`
	Error       string `json:"error,omitempty"`
	FailureKind string `json:"failure_kind,omitempty"`
}

func probeArgv() []string {
	return []string{"/usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2", "--inhibit-cache", "--glibc-hwcaps-mask", "", "--library-path", "/usr/lib/x86_64-linux-gnu", "/usr/bin/bwrap", "--unshare-user", "--unshare-pid", "--as-pid-1", "--unshare-net", "--unshare-ipc", "--unshare-uts", "--die-with-parent", "--new-session", "--cap-drop", "ALL", "--clearenv", "--ro-bind", binaryPath, "/probe", "--ro-bind", runRoot + "/cache", "/cache", "--bind", runRoot + "/metadata", "/metadata", "--proc", "/proc", "--remount-ro", "/proc", "--remount-ro", "/", "--chdir", "/metadata", "--", "/probe", "child"}
}

func validatePlan(value plan, uid int) error {
	if uid == 0 {
		return fmt.Errorf("root execution refused")
	}
	if value.Schema != 1 || value.RunRoot != runRoot || value.Binary != binaryPath || value.TimeoutSeconds != 120 {
		return fmt.Errorf("plan identity/deadline differs from reviewed constants")
	}
	if !reflect.DeepEqual(value.Argv, probeArgv()) || len(value.Pins) != len(dependencyPaths) {
		return fmt.Errorf("argv/dependency inventory differs")
	}
	for index, item := range value.Pins {
		decoded, err := hex.DecodeString(item.SHA256)
		if item.Path != dependencyPaths[index] || err != nil || len(decoded) != 32 {
			return fmt.Errorf("invalid dependency pin %d", index)
		}
	}
	if value.Pins[0].SHA256 != bwrapSHA {
		return fmt.Errorf("unreviewed bwrap")
	}
	return nil
}

func rejectDuplicateKeys(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	if delim != '{' && delim != '[' {
		return fmt.Errorf("unexpected JSON delimiter")
	}
	seen := map[string]bool{}
	for decoder.More() {
		if delim == '{' {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("duplicate/invalid JSON key")
			}
			seen[name] = true
		}
		if err := rejectDuplicateKeys(decoder); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

func strictJSON(data []byte, target any) error {
	if len(data) > 65536 {
		return fmt.Errorf("oversized JSON")
	}
	if err := rejectDuplicateKeys(json.NewDecoder(bytes.NewReader(data))); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}

func decodePlan(data []byte) (plan, error) {
	var value plan
	err := strictJSON(data, &value)
	return value, err
}

func verdict(positive, negative map[string]attempt) string {
	if len(positive) != len(mutationNames) || len(negative) != len(mutationNames) {
		return "INCONCLUSIVE"
	}
	for _, name := range mutationNames {
		if !positive[name].OK {
			return "INCONCLUSIVE"
		}
	}
	for _, name := range mutationNames {
		result, exists := negative[name]
		if !exists {
			return "INCONCLUSIVE"
		}
		if result.OK {
			return "FAIL"
		}
		if result.Errno != 1 && result.Errno != 13 && result.Errno != 30 {
			return "INCONCLUSIVE"
		}
	}
	return "PASS"
}

//go:build linux

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPlanRejectsAuthorityDrift(t *testing.T) {
	if err := validatePlan(examplePlan(), 1000); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*plan){
		"root":                 func(_ *plan) {},
		"runroot":              func(value *plan) { value.RunRoot = "/tmp/other" },
		"binary":               func(value *plan) { value.Binary = "/bin/sh" },
		"bwrap":                func(value *plan) { value.Pins[0].SHA256 = strings.Repeat("0", 64) },
		"timeout":              func(value *plan) { value.TimeoutSeconds = 0 },
		"argv":                 func(value *plan) { value.Argv = append(value.Argv, "--unshare-user-try") },
		"missing dependency":   func(value *plan) { value.Pins = value.Pins[:1] },
		"duplicate dependency": func(value *plan) { value.Pins = append(value.Pins, value.Pins[0]) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			value := examplePlan()
			change(&value)
			uid := 1000
			if name == "root" {
				uid = 0
			}
			if err := validatePlan(value, uid); err == nil {
				t.Fatal("accepted changed authority")
			}
		})
	}
}

func TestNamespaceArgvIsClosed(t *testing.T) {
	args := strings.Join(probeArgv(), " ")
	for _, flag := range []string{"--unshare-user", "--unshare-pid", "--unshare-net", "--unshare-ipc", "--proc /proc", "--cap-drop ALL", "--new-session", "--die-with-parent", "--clearenv"} {
		if !strings.Contains(args, flag) {
			t.Fatalf("missing %s", flag)
		}
	}
	for _, forbidden := range []string{"--unshare-user-try", "--ro-bind / /", "--share-net", "--dev-bind", "--setenv", "--preserve-fds"} {
		if strings.Contains(args, forbidden) {
			t.Fatalf("unsafe argv: %s", forbidden)
		}
	}
}

func TestDecodeRejectsUnknownTrailingAndDuplicateFields(t *testing.T) {
	data, err := json.Marshal(examplePlan())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodePlan(data); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{
		append(append([]byte{}, data...), []byte(" {}")...),
		[]byte(strings.Replace(string(data), "{", "{\"unknown\":true,", 1)),
		[]byte(strings.Replace(string(data), "{", "{\"schema\":1,", 1)),
	} {
		if _, err := decodePlan(bad); err == nil {
			t.Fatal("accepted malformed plan")
		}
	}
}

func TestDenialRequiresWorkingControlAndCompleteMatrix(t *testing.T) {
	positive, negative := map[string]attempt{}, map[string]attempt{}
	for _, name := range mutationNames {
		positive[name] = attempt{OK: true}
		negative[name] = attempt{Errno: 30}
	}
	if verdict(positive, negative) != "PASS" {
		t.Fatal("complete matrix rejected")
	}
	positive[mutationNames[0]] = attempt{Errno: 1}
	if verdict(positive, negative) != "INCONCLUSIVE" {
		t.Fatal("denied control treated as proof")
	}
	positive[mutationNames[0]] = attempt{OK: true}
	negative[mutationNames[0]] = attempt{Errno: 2}
	if verdict(positive, negative) != "INCONCLUSIVE" {
		t.Fatal("missing path treated as enforcement")
	}
	negative[mutationNames[0]] = attempt{OK: true}
	if verdict(positive, negative) != "FAIL" {
		t.Fatal("write escape accepted")
	}
	delete(negative, mutationNames[0])
	if verdict(positive, negative) != "INCONCLUSIVE" {
		t.Fatal("missing case accepted")
	}
}

func examplePlan() plan {
	value := plan{Schema: 1, RunRoot: runRoot, Binary: binaryPath, TimeoutSeconds: 120, Argv: probeArgv()}
	for _, path := range dependencyPaths {
		value.Pins = append(value.Pins, pin{Path: path, SHA256: strings.Repeat("a", 64)})
	}
	value.Pins[0].SHA256 = bwrapSHA
	return value
}

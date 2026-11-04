package platforminstall

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func closedSourceFixture(t *testing.T, city string, pin ProviderPin) string {
	t.Helper()
	name := filepath.Base(pin.Path)
	family := strings.TrimSuffix(strings.TrimPrefix(name, "gct-claude-"), "-worker")
	root := filepath.Dir(filepath.Dir(pin.Path))
	label := family
	if family == "signing" {
		label = "core-signing"
	}
	paths := []string{
		"templates/claude/" + label + "-control-policy.json", "bin/" + name,
		"lib/gct_claude_" + strings.ReplaceAll(family, "-", "_") + "_worker.py",
	}
	if family != "signing" {
		paths = append(paths, "lib/gct_claude_signing_worker.py")
	}
	paths = append(paths, "lib/gct_claude_subscription.py")
	if family == "project-candidate" {
		paths = append(paths, "lib/gct_claude_workspace_trust.py")
	}
	paths = append(paths, "templates/claude/"+family+"-provider.toml", "lib/gct_provider_allowlist.py")
	records := []map[string]string{}
	for _, relative := range paths {
		path := filepath.Join(root, relative)
		if path != pin.Path {
			writeProviderFixture(t, path, []byte("fixture source"))
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, map[string]string{"path": path, "sha256": testSHA256(data)})
	}
	data, err := json.Marshal(map[string]any{
		"dependencies": records, "provider": "claude",
		"entrypoint": filepath.Join(filepath.Dir(city), "bin/claude"), "allowlist": filepath.Join(city, "managed/providers.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return name + " 2 sources_sha256=" + testSHA256(data) + " cli_sha256=" + strings.Repeat("a", 64)
}

func TestClosedSourceRejectsParentAliasAndHelperDriftBeforeProbe(t *testing.T) {
	for _, fault := range []string{"parent", "helper", "helper-link"} {
		t.Run(fault, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "template/bin/gct-claude-signing-worker")
			marker := filepath.Join(root, "ran")
			data := []byte("#!/bin/sh\ntouch " + marker + "\n")
			writeProviderFixture(t, path, data)
			if err := os.Chmod(path, 0o755); err != nil {
				t.Fatal(err)
			}
			pin := ProviderPin{Path: path, ResolvedPath: path, SHA256: testSHA256(data), VersionArgs: []string{"--version"}}
			city := filepath.Join(root, "gascity/city")
			pin.Version = closedSourceFixture(t, city, pin)
			if fault == "parent" {
				alias := filepath.Join(root, "alias")
				if err := os.Symlink(filepath.Dir(path), alias); err != nil {
					t.Fatal(err)
				}
				pin.Path = filepath.Join(alias, filepath.Base(path))
			} else {
				helper := filepath.Join(root, "template/lib/gct_claude_signing_worker.py")
				if fault == "helper" {
					writeProviderFixture(t, helper, []byte("changed"))
				} else {
					if err := os.Remove(helper); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(path, helper); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := verifyClosedSource(city, pin); err == nil {
				_ = VerifyProviderPin(context.Background(), pin)
				t.Fatal("accepted unpinned source chain")
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("unapproved probe executed")
			}
		})
	}
}

func TestClosedSourcePreservesLiteralUnicodeEscapeInPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), `literal\u2028`)
	path := filepath.Join(root, "template/bin/gct-claude-signing-worker")
	data := []byte("#!/bin/sh\nexit 0\n")
	writeProviderFixture(t, path, data)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	pin := ProviderPin{Path: path, ResolvedPath: path, SHA256: testSHA256(data), VersionArgs: []string{"--version"}}
	city := filepath.Join(root, "gascity/city")
	pin.Version = closedSourceFixture(t, city, pin)
	if err := verifyClosedSource(city, pin); err != nil {
		t.Fatal(err)
	}
}

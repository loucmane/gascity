package providerallowlist

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) (string, []byte, []Binding) {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, "toolchains", "sample", "1.0.0", "sample")
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("never execute me"), 0o555); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "sample")
	if err := os.Symlink(binary, link); err != nil {
		t.Fatal(err)
	}
	data := []byte(fmt.Sprintf(`{"schema":"gc.provider-allowlist.v1","entries":[{"provider":"sample","version":"1.0.0","path":%q,"sha256":%q,"active":true}],"previous":{}}`, binary, fmt.Sprintf("%x", sha256.Sum256([]byte("never execute me")))))
	path := filepath.Join(dir, "providers.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, data, []Binding{{Provider: "sample", Path: link}}
}

func TestVerifyActiveBinary(t *testing.T) {
	for _, change := range []string{"valid", "missing", "changed", "writable", "link drift", "indirect link", "relative link", "required missing"} {
		t.Run(change, func(t *testing.T) {
			_, data, bindings := fixture(t)
			list, err := Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			binary := list.Entries[0].Path
			switch change {
			case "missing":
				err = os.Remove(binary)
			case "changed":
				err = os.Chmod(binary, 0o755)
				if err == nil {
					err = os.WriteFile(binary, []byte("changed"), 0o555)
				}
				if err == nil {
					err = os.Chmod(binary, 0o555)
				}
			case "writable":
				err = os.Chmod(binary, 0o755)
			case "link drift":
				err = os.Remove(bindings[0].Path)
				if err == nil {
					err = os.Symlink("/missing", bindings[0].Path)
				}
			case "indirect link", "relative link":
				target := "toolchains/sample/1.0.0/sample"
				if change == "indirect link" {
					target = filepath.Join(filepath.Dir(bindings[0].Path), "alias")
					if err := os.Symlink(binary, target); err != nil {
						t.Fatal(err)
					}
				}
				err = os.Remove(bindings[0].Path)
				if err == nil {
					err = os.Symlink(target, bindings[0].Path)
				}
			case "required missing":
				bindings[0].Provider = "other"
			}
			if err != nil {
				t.Fatal(err)
			}
			err = list.Verify(bindings)
			if change == "valid" && err != nil {
				t.Fatal(err)
			}
			if change != "valid" && err == nil {
				t.Fatal("accepted provider drift")
			}
		})
	}
}

func TestParseRejectsAmbiguousAllowlist(t *testing.T) {
	_, data, _ := fixture(t)
	for _, mutate := range []func(string) string{
		func(s string) string { return strings.Replace(s, `"active":true`, `"active":false`, 1) },
		func(s string) string { return strings.Replace(s, `"active":true`, `"active":true,"active":false`, 1) },
		func(s string) string {
			return strings.Replace(s, `"previous":{}`, `"previous":{"sample":"missing"}`, 1)
		},
		func(s string) string { return strings.Replace(s, `"previous":{}`, `"unknown":true`, 1) },
		func(s string) string { return s + `{}` },
		func(s string) string { return strings.Replace(s, `"active":true`, `"active":"true"`, 1) },
		func(s string) string { return strings.Replace(s, `"active":true`, `"active":true,"Active":false`, 1) },
		func(s string) string { return strings.Replace(s, `"previous":{}`, `"previous":null`, 1) },
	} {
		if _, err := Parse([]byte(mutate(string(data)))); err == nil {
			t.Fatal("accepted malformed allowlist")
		}
	}
}

func TestReceiptApprovalBindsExactBytes(t *testing.T) {
	path, data, _ := fixture(t)
	receipt := map[string]any{
		"schema": "gc.provisioning-receipt.v2", "permission_revision": strings.Repeat("b", 64),
		"provider_allowlist": map[string]string{"path": path, "sha256": fmt.Sprintf("%x", sha256.Sum256(data))},
	}
	approve := func() []byte {
		delete(receipt, "receipt_sha256")
		encoded, err := json.Marshal(receipt)
		if err != nil {
			t.Fatal(err)
		}
		receipt["receipt_sha256"] = fmt.Sprintf("%x", sha256.Sum256(encoded))
		encoded, err = json.Marshal(receipt)
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	encoded := approve()
	if _, err := ReadApproved(path, encoded, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadApproved(path, encoded, strings.Repeat("c", 64)); err == nil {
		t.Fatal("accepted stale revision")
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadApproved(path, encoded, ""); err == nil {
		t.Fatal("accepted unapproved file bytes")
	}
	receipt["receipt_sha256"] = strings.Repeat("c", 64)
	bad, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadApproved(path, bad, ""); err == nil {
		t.Fatal("accepted changed receipt")
	}
	delete(receipt, "provider_allowlist")
	if _, err := ReadApproved(path, approve(), ""); err == nil {
		t.Fatal("accepted absent approval")
	}
}

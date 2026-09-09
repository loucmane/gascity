package managedworker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/fsys"
)

func TestReadControlPolicyRefusesUnsafeNodeBeforeReading(t *testing.T) {
	for _, shape := range []string{"symlink", "parent symlink", "directory", "fifo", "socket", "device", "writable", "parent error"} {
		t.Run(shape, func(t *testing.T) {
			files := fsys.NewFake()
			files.Files["/policies/policy.json"] = []byte(candidatePolicyBytes)
			switch shape {
			case "symlink":
				delete(files.Files, "/policies/policy.json")
				files.Symlinks["/policies/policy.json"] = "/target.json"
				files.Files["/target.json"] = []byte(candidatePolicyBytes)
			case "parent symlink":
				files.Symlinks["/policies"] = "/elsewhere"
			case "directory":
				delete(files.Files, "/policies/policy.json")
				files.Dirs["/policies/policy.json"] = true
			case "fifo":
				files.Modes["/policies/policy.json"] = os.ModeNamedPipe | 0o600
			case "socket":
				files.Modes["/policies/policy.json"] = os.ModeSocket | 0o600
			case "device":
				files.Modes["/policies/policy.json"] = os.ModeDevice | 0o600
			case "writable":
				files.Modes["/policies/policy.json"] = 0o666
			case "parent error":
				files.Errors["/policies"] = errors.New("inaccessible parent")
			}
			if _, err := ReadControlPolicy(files, "/policies/policy.json"); err == nil {
				t.Fatal("unsafe file accepted")
			}
			for _, call := range files.Calls {
				if call.Method == "ReadFile" || call.Method == "ReadRegularFile" {
					t.Fatalf("unsafe node was read: %+v", call)
				}
			}
		})
	}
}

type policyReadMutationFS struct {
	fsys.FS
	afterRead func()
}

func (files policyReadMutationFS) ReadRegularFile(path string) ([]byte, error) {
	data, err := files.FS.(interface{ ReadRegularFile(string) ([]byte, error) }).ReadRegularFile(path)
	files.afterRead()
	return data, err
}

func TestReadControlPolicyRejectsObservedMutationDuringRead(t *testing.T) {
	for _, mutation := range []string{"mode", "size", "symlink", "parent symlink"} {
		t.Run(mutation, func(t *testing.T) {
			files := fsys.NewFake()
			path := "/policies/policy.json"
			files.Files[path] = []byte(candidatePolicyBytes)
			wrapped := policyReadMutationFS{FS: files, afterRead: func() {
				switch mutation {
				case "mode":
					files.Modes[path] = 0o400
				case "size":
					files.Files[path] = []byte("replacement")
				case "symlink":
					delete(files.Files, path)
					files.Symlinks[path] = "/target.json"
				case "parent symlink":
					files.Symlinks["/policies"] = "/elsewhere"
				}
			}}
			if _, err := ReadControlPolicy(wrapped, path); err == nil || !strings.Contains(err.Error(), "control_policy") {
				t.Fatalf("read mutation not refused: %v", err)
			}
		})
	}
}

func TestReadControlPolicyRejectsRealTimestampMutation(t *testing.T) {
	// Fake.Lstat does not expose ModTimes; exercise this check
	// against real FileInfo rather than mistaking an invisible fake mutation
	// for evidence that the production reader ignored timestamp drift.
	path := filepath.Join(t.TempDir(), "policy.json")
	if err := os.WriteFile(path, []byte(candidatePolicyBytes), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, time.Unix(100, 0), time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	files := policyReadMutationFS{FS: fsys.OSFS{}, afterRead: func() {
		if err := os.Chtimes(path, time.Unix(200, 0), time.Unix(200, 0)); err != nil {
			t.Fatal(err)
		}
	}}
	if _, err := ReadControlPolicy(files, path); err == nil || !strings.Contains(err.Error(), "changed while reading") {
		t.Fatalf("timestamp mutation was not refused: %v", err)
	}
}

func TestReadControlPolicyKeepsValidFileAndRequiresExplicitReader(t *testing.T) {
	files := fsys.NewFake()
	files.Files[candidatePolicyPath] = []byte(candidatePolicyBytes)
	data, err := ReadControlPolicy(files, candidatePolicyPath)
	if err != nil || string(data) != candidatePolicyBytes {
		t.Fatalf("valid file: %v", err)
	}
	for _, path := range []string{"", ".", "/", "relative.json", "/policies/../policy.json"} {
		if _, err := ReadControlPolicy(files, path); err == nil {
			t.Errorf("invalid path accepted: %q", path)
		}
	}
	if _, err := ReadControlPolicy(nil, candidatePolicyPath); err == nil {
		t.Fatal("missing filesystem accepted")
	}
	receipt, err := LoadProvisioningReceipt(typedReceiptWire(t, "candidate", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyControlPolicy(nil, receipt.Profiles[0]); err == nil {
		t.Fatal("missing safe reader accepted")
	}
	if err := VerifyControlPolicy(nil, testProfile()); err != nil {
		t.Fatalf("legacy profile gained policy requirement: %v", err)
	}
}

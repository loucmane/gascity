package platforminstall

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func metadataContractFixture(t *testing.T) Manifest {
	t.Helper()
	manifest := testManifest(t, t.TempDir(), []byte("candidate"), []byte("old"))
	manifest.Schema = MetadataManifestSchema
	manifest.Activation = &ActivationSpec{ExpectedCommit: strings.Repeat("a", 40), ExpectedVersion: "version", PreviousCommit: strings.Repeat("b", 40), PreviousVersion: "old"}
	manifest.Metadata = &MetadataLaunch{Transaction: strings.Repeat("a", 64), Attempt: strings.Repeat("b", 64), Host: MetadataHost{PID: 42, Boot: "boot", Start: "123", Executable: "/fixture/gc", Address: "127.0.0.1:4400", Listener: "123", City: "fixture"}, Namespaces: map[string]string{"user": "u", "pid": "p", "mnt": "m", "net": "n", "ipc": "i", "time": "t"}, Writer: FilePin{Path: "/fixture/writer", SHA256: strings.Repeat("c", 64)}, GCHome: "/fixture/home", CacheSHA256: strings.Repeat("d", 64), ImportsSHA256: sha256Hex([]byte("{}")), Evidence: "/fixture/evidence", Parents: []MetadataParent{{Path: "/fixture/evidence"}}, Inputs: []FilePin{{Path: "/fixture/config", SHA256: strings.Repeat("e", 64)}}, Trees: []FilePin{{Path: "/fixture/home/cache/repos", SHA256: strings.Repeat("d", 64)}}}
	for index, path := range metadataRuntimePaths {
		pin := FilePin{Path: path, SHA256: strings.Repeat("f", 64)}
		if index == 0 {
			pin.SHA256 = metadataBwrapSHA
		}
		manifest.Metadata.Runtime = append(manifest.Metadata.Runtime, pin)
	}
	for _, path := range metadataOutputs(manifest) {
		manifest.Metadata.Preimages = append(manifest.Metadata.Preimages, MetadataPreimage{Path: path})
	}
	return manifest
}

func TestMetadataSandboxExactNamespacesEnvironmentAndChannel(t *testing.T) {
	manifest := metadataContractFixture(t)
	args, err := metadataSandboxArgv(manifest)
	if err != nil {
		t.Fatal(err)
	}
	joined := "\x00" + strings.Join(args, "\x00") + "\x00"
	for _, required := range []string{"--unshare-user", "--unshare-pid", "--as-pid-1", "--unshare-net", "--unshare-ipc", "--unshare-uts", "--cap-drop\x00ALL", "--clearenv", "--setenv\x00GODEBUG\x00containermaxprocs=0", "--proc\x00/proc", "--remount-ro\x00/proc", "--remount-ro\x00/", "__metadata-writer-v1"} {
		if !strings.Contains(joined, "\x00"+required+"\x00") {
			t.Fatalf("missing %q", required)
		}
	}
	for _, forbidden := range []string{"--preserve-fds", "--sync-fd", "--args", "--unshare-user-try", "--unshare-time", "--ro-bind\x00/\x00/", "--share-net"} {
		if strings.Contains(joined, "\x00"+forbidden+"\x00") {
			t.Fatalf("unexpected %q", forbidden)
		}
	}
	for _, change := range []string{"runtime", "cache", "host-proc", "whole-home", "output-overlap", "duplicate"} {
		t.Run(change, func(t *testing.T) {
			candidate := metadataContractFixture(t)
			switch change {
			case "runtime":
				candidate.Metadata.Runtime[0].SHA256 = strings.Repeat("0", 64)
			case "cache":
				candidate.Metadata.Trees = nil
			case "host-proc":
				candidate.Metadata.Inputs[0].Path = "/proc/42/stat"
			case "whole-home":
				candidate.Metadata.Inputs[0].Path = "/fixture"
			case "output-overlap":
				candidate.Metadata.Inputs[0].Path = manifest.ReceiptPath
				candidate.ReceiptPath = manifest.ReceiptPath
				candidate.Metadata.Preimages[1].Path = manifest.ReceiptPath
			case "duplicate":
				candidate.Metadata.Inputs = append(candidate.Metadata.Inputs, candidate.Metadata.Inputs[0])
			}
			if _, err := metadataSandboxArgv(candidate); err == nil {
				t.Fatal("invalid mount contract accepted")
			}
		})
	}
}

func TestMetadataInputAbsenceMustBeDeclared(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "missing.toml")
	input := metadataInputFS{launch: &MetadataLaunch{}}
	if _, err := input.ReadFile(path); err == nil || os.IsNotExist(err) {
		t.Fatalf("unmounted input became absence: %v", err)
	}
	input.launch.Absent = []string{path}
	if _, err := input.ReadFile(path); !os.IsNotExist(err) {
		t.Fatalf("declared absence not preserved: %v", err)
	}
}

func TestMetadataRuntimeSymlinkRequiresPinnedCompleteClosure(t *testing.T) {
	root := t.TempDir()
	target, link := filepath.Join(root, "library.1.2"), filepath.Join(root, "library.1")
	if err := os.WriteFile(target, []byte("runtime"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("library.1.2", link); err != nil {
		t.Fatal(err)
	}
	pin := FilePin{Path: link, SHA256: sha256Hex([]byte("runtime")), Mode: 0o600}
	launch := &MetadataLaunch{Inputs: []FilePin{{Path: target}}, Links: []MetadataLink{{Path: link, Target: "library.1.2"}}}
	if err := metadataCheckRuntimePin(pin, launch); err != nil {
		t.Fatal(err)
	}
	launch.Links[0].Target = "wrong"
	if err := metadataCheckRuntimePin(pin, launch); err == nil {
		t.Fatal("link drift accepted")
	}
	launch.Links[0].Target = "library.1.2"
	launch.Inputs = nil
	if err := metadataCheckRuntimePin(pin, launch); err == nil {
		t.Fatal("missing runtime closure accepted")
	}
}

func TestMetadataLaunchRefusesAmbiguousAbsenceAndImportDigest(t *testing.T) {
	for _, kind := range []string{"relative", "duplicate", "digest"} {
		t.Run(kind, func(t *testing.T) {
			manifest := metadataContractFixture(t)
			switch kind {
			case "relative":
				manifest.Metadata.Absent = []string{"missing"}
			case "duplicate":
				manifest.Metadata.Absent = []string{"/missing", "/missing"}
			case "digest":
				manifest.Metadata.ImportsSHA256 = "unbound"
			}
			if err := validateMetadataLaunch(manifest); err == nil {
				t.Fatal("ambiguous input contract accepted")
			}
		})
	}
}

func TestMetadataParentsRefuseUnrelatedEntriesAndHardLinks(t *testing.T) {
	for _, kind := range []string{"valid", "unrelated", "hardlink", "symlink", "directory", "writable"} {
		t.Run(kind, func(t *testing.T) {
			manifest := metadataContractFixture(t)
			manifest.Metadata.Evidence = t.TempDir()
			manifest.Metadata.Parents = nil
			parents := map[string]bool{manifest.Metadata.Evidence: true}
			for _, path := range metadataOutputs(manifest) {
				parents[filepath.Dir(path)] = true
			}
			for parent := range parents {
				if err := os.MkdirAll(parent, 0o700); err != nil {
					t.Fatal(err)
				}
				var stat unix.Stat_t
				if err := unix.Stat(parent, &stat); err != nil {
					t.Fatal(err)
				}
				manifest.Metadata.Parents = append(manifest.Metadata.Parents, MetadataParent{Path: parent, Device: stat.Dev, Inode: stat.Ino, UID: stat.Uid, GID: stat.Gid, Mode: stat.Mode & 0o7777})
			}
			path := filepath.Join(manifest.Metadata.Evidence, "intent.json")
			var err error
			switch kind {
			case "unrelated":
				err = os.WriteFile(filepath.Join(manifest.Metadata.Evidence, "unknown"), []byte("protected"), 0o600)
			case "hardlink":
				outside := filepath.Join(t.TempDir(), "cache")
				if err := os.WriteFile(outside, []byte("protected"), 0o600); err != nil {
					t.Fatal(err)
				}
				err = os.Link(outside, path)
			case "symlink":
				err = os.Symlink("/missing", path)
			case "directory":
				err = os.Mkdir(path, 0o700)
			case "writable":
				err = os.Chmod(manifest.Metadata.Evidence, 0o777)
			case "valid":
				err = os.WriteFile(path, []byte("evidence"), 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			err = metadataCheckParents(manifest)
			if (err == nil) != (kind == "valid") {
				t.Fatalf("%s: %v", kind, err)
			}
		})
	}
}

func TestMetadataChannelBoundsAndIdentity(t *testing.T) {
	binding := MetadataBinding{Protocol: metadataProtocol, Transaction: strings.Repeat("a", 64), Attempt: strings.Repeat("b", 64), Nonce: strings.Repeat("c", 64), Channel: strings.Repeat("d", 64), Request: strings.Repeat("e", 64), Instance: strings.Repeat("f", 64), Deadline: 5, LeaseEnd: 3_000_000_005}
	frame := metadataFrame{Binding: binding, Kind: "PREPARED", Sequence: 4}
	if err := metadataCheckFrame(frame, binding, "PREPARED", 4, 1); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*metadataFrame){func(value *metadataFrame) { value.Kind = "FINAL" }, func(value *metadataFrame) { value.Sequence++ }, func(value *metadataFrame) { value.Binding.Nonce = strings.Repeat("0", 64) }, func(value *metadataFrame) { value.Binding.Request = strings.Repeat("0", 64) }, func(value *metadataFrame) { value.Binding.Instance = strings.Repeat("0", 64) }} {
		changed := frame
		mutate(&changed)
		if err := metadataCheckFrame(changed, binding, "PREPARED", 4, 1); err == nil {
			t.Fatal("cross-frame identity accepted")
		}
	}
	if err := metadataCheckFrame(frame, binding, "PREPARED", 4, 5); err == nil {
		t.Fatal("expired frame accepted")
	}
	for _, size := range []uint32{0, metadataFrameLimit + 1} {
		var header [4]byte
		binary.BigEndian.PutUint32(header[:], size)
		if err := metadataDecode(bytes.NewReader(header[:]), &frame); err == nil {
			t.Fatal("bad frame size accepted")
		}
	}
	var encoded bytes.Buffer
	if err := metadataEncode(&encoded, frame); err != nil {
		t.Fatal(err)
	}
	data := encoded.Bytes()
	if err := metadataDecode(bytes.NewReader(data[:len(data)-1]), &frame); err == nil {
		t.Fatal("truncated frame accepted")
	}
}

func TestMetadataOutputBoundsCannotUseReaderFromBypass(t *testing.T) {
	for _, output := range []io.Writer{&metadataBoundedOutput{}, &metadataInspectionOutput{}} {
		if _, ok := output.(io.ReaderFrom); ok {
			t.Fatal("unbounded ReaderFrom promoted")
		}
		if _, err := io.Copy(output, strings.NewReader(strings.Repeat("x", 2*1024*1024))); err == nil {
			t.Fatal("oversize output accepted")
		}
	}
}

func TestMetadataTreeInventoryDoesNotWriteAccessTimes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "input")
	if err := os.WriteFile(path, []byte("frozen"), 0o400); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{root, path} {
		if err := os.Chtimes(target, time.Unix(1, 0), time.Unix(2, 0)); err != nil {
			t.Fatal(err)
		}
	}
	before := map[string]unix.Stat_t{}
	for _, target := range []string{root, path} {
		var stat unix.Stat_t
		if err := unix.Stat(target, &stat); err != nil {
			t.Fatal(err)
		}
		before[target] = stat
	}
	first, err := metadataTreeDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := metadataTreeDigest(root)
	if err != nil || first != second {
		t.Fatal("inventory drift", err)
	}
	input := metadataInputFS{launch: &MetadataLaunch{Trees: []FilePin{{Path: root}}}}
	if _, err := input.ReadFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := input.ReadDir(root); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{root, path} {
		var stat unix.Stat_t
		if err := unix.Stat(target, &stat); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(stat, before[target]) {
			t.Fatalf("inventory mutated %s", target)
		}
	}
}

func TestMetadataNewReceiptBindsFrozenRequestAndForbidsGeneralRollback(t *testing.T) {
	manifest := metadataContractFixture(t)
	manifest.ManifestSHA256 = strings.Repeat("e", 64)
	receipt := Receipt{Schema: metadataReceiptSchema, ManifestSHA256: manifest.ManifestSHA256, ReleaseID: manifest.ReleaseID, ArtifactSHA256: manifest.Core.SHA256, PreviousSHA256: manifest.PreviousSHA256, Activation: &RuntimeProof{manifest.Core.SHA256, manifest.Activation.ExpectedCommit, manifest.Activation.ExpectedVersion}, Metadata: &MetadataCommit{Host: manifest.Metadata.Host, Binding: MetadataBinding{Request: manifest.ManifestSHA256, Transaction: manifest.Metadata.Transaction, Attempt: manifest.Metadata.Attempt}}}
	if !receiptMatchesManifest(receipt, manifest) {
		t.Fatal("bound receipt refused")
	}
	receipt.Metadata.Binding.Request = strings.Repeat("0", 64)
	if receiptMatchesManifest(receipt, manifest) {
		t.Fatal("unrelated request accepted")
	}
	if err := Rollback(manifest); err == nil {
		t.Fatal("irreversible receipt rollback accepted")
	}
	if _, err := newInstaller().adoptPrepared(context.Background(), manifest, nil, true); err == nil {
		t.Fatal("unconfined old writer accepted versioned manifest")
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Manifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(manifest, decoded) {
		t.Fatal("contract fields lost")
	}
}

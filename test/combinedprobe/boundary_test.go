//go:build linux

package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

type shortWriter struct{}

func (shortWriter) Write(data []byte) (int, error) { return len(data) - 1, nil }

func TestBoundedPipeFrames(t *testing.T) {
	value := frame{Kind: "HELLO", Sequence: 1}
	var wire bytes.Buffer
	if err := send(&wire, value); err != nil {
		t.Fatal(err)
	}
	var decoded frame
	if err := receive(&wire, &decoded); err != nil || decoded != value {
		t.Fatal(decoded, err)
	}
	if err := send(shortWriter{}, value); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
	for _, data := range [][]byte{{0, 0, 0}, {0, 1, 0, 0}, {0, 0, 0, 5, '{'}} {
		if err := receive(bytes.NewReader(data), &decoded); err == nil {
			t.Fatal("truncation/oversize accepted")
		}
	}
}

func TestClosedConfinementArgv(t *testing.T) {
	for _, name := range cases {
		args := strings.Join(writerArgv(name), " ")
		for _, required := range []string{"--unshare-user", "--unshare-pid", "--as-pid-1", "--unshare-net", "--unshare-ipc", "--proc /proc", "--remount-ro /proc", "--clearenv", "--cap-drop ALL"} {
			if !strings.Contains(args, required) {
				t.Fatal(required)
			}
		}
		for _, forbidden := range []string{"--unshare-user-try", "--unshare-time", "--preserve-fds", "--bind / ", "--ro-bind / ", "--share-net", "sudo"} {
			if strings.Contains(args, forbidden) {
				t.Fatal(forbidden)
			}
		}
	}
}

func TestFailureProvenanceAndExitRefusal(t *testing.T) {
	if decodeAttempt([]byte(`{"ok":true,"errno":0}`), errors.New("exit1")).OK {
		t.Fatal("nonzero child accepted")
	}
	if processDenial("ptrace", attempt{Errno: 3, FailureKind: "post_attach"}) {
		t.Fatal("post-attach accepted")
	}
	if !processDenial("ptrace", attempt{Errno: 3, FailureKind: "attach_denied"}) {
		t.Fatal("attach-denial refused")
	}
	for _, kind := range []string{"pre_attach", "non_errno", ""} {
		if processDenial("ptrace", attempt{Errno: 3, FailureKind: kind}) {
			t.Fatal(kind)
		}
	}
}

func TestUnsupportedReceiptAndExpiredGrant(t *testing.T) {
	manifest := []byte("manifest")
	unsupported := []byte(`{"schema":"gc.synthetic-committed-pair.v2","transaction":"tx","epoch":"epoch","manifest":"x"}`)
	if committedPair(unsupported, manifest, unsupported) == nil {
		t.Fatal("unsupported schema accepted")
	}
	value := binding{Protocol: domain, Deadline: 100}
	if checkFrame(frame{Binding: value, Kind: "RECEIPT_GRANT", Sequence: 6}, value, "RECEIPT_GRANT", 6, 100) == nil {
		t.Fatal("expired grant accepted")
	}
	if disposition(true, errors.New("directory sync failed")) != "committed_evidence_incomplete" {
		t.Fatal("postcommit durability misclassified")
	}
}

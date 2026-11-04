package providerallowlist

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestApprovalReaderRejectsFIFOWithoutBlocking(t *testing.T) {
	city := t.TempDir()
	path := ReceiptPath(city)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := ReadApprovalReceipt(city)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("FIFO approval result: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("approval read blocked on a FIFO")
	}
}

func TestApprovalReaderBoundsTheOpenedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receipt")
	if err := os.WriteFile(path, []byte("1234"), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := readPhysicalRegular(path, 4); err != nil || string(data) != "1234" {
		t.Fatalf("exact bound = %q, %v", data, err)
	}
	if _, err := readPhysicalRegular(path, 3); err == nil {
		t.Fatal("accepted oversized approval")
	}
}

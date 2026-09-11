package platforminstall

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMetadataFailedOuterCompletionCannotReplayMatchingFinal(t *testing.T) {
	manifest := metadataContractFixture(t)
	manifest.Metadata.Evidence = t.TempDir()
	manifest.Metadata.Parents[0].Path = manifest.Metadata.Evidence
	manifest = finalizeManifest(t, manifest)
	binding := MetadataBinding{Protocol: metadataProtocol, Transaction: manifest.Metadata.Transaction, Attempt: manifest.Metadata.Attempt, Nonce: strings.Repeat("c", 64), Channel: strings.Repeat("d", 64), Request: manifest.ManifestSHA256, Instance: strings.Repeat("f", 64), Deadline: 1000, LeaseEnd: 3_000_000_000}
	receipt := Receipt{Schema: metadataReceiptSchema, ManifestSHA256: manifest.ManifestSHA256, ReleaseID: manifest.ReleaseID, ArtifactSHA256: manifest.Core.SHA256, PreviousSHA256: manifest.PreviousSHA256, Result: ResultInstalled, Activation: &RuntimeProof{manifest.Core.SHA256, manifest.Activation.ExpectedCommit, manifest.Activation.ExpectedVersion}, Metadata: &MetadataCommit{Host: manifest.Metadata.Host, Binding: binding}}
	if err := finalizeReceipt(&receipt); err != nil {
		t.Fatal(err)
	}
	receiptBytes, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	final := metadataFrame{Binding: binding, Kind: "FINAL", Sequence: 9, Digest: receipt.ReceiptSHA256}
	finalBytes, err := json.Marshal(final)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{DefaultManifestPath(manifest.CityPath): marshalManifest(t, manifest), manifest.ReceiptPath: receiptBytes, filepath.Join(manifest.Metadata.Evidence, "final.json"): finalBytes, manifest.Core.Destination: []byte("candidate"), manifest.BackupPath: []byte("old")}
	for path, data := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if path == manifest.Core.Destination || path == manifest.BackupPath {
			mode = os.FileMode(manifest.Core.Mode)
		}
		if err := os.WriteFile(path, data, mode); err != nil {
			t.Fatal(err)
		}
	}
	if _, actual, err := ReadCommittedPair(manifest); err != nil || actual.ReceiptSHA256 != final.Digest || actual.Metadata.Binding != final.Binding {
		t.Fatal("fixture is not a committed pair with matching FINAL", err)
	}
	waitFailure := errors.New("injected outer expiry wait failure")
	if err := metadataWaitLeaseExpiry(context.Background(), binding.LeaseEnd, func() (int64, error) { return 100, nil }, func(context.Context, time.Duration) error { return waitFailure }); !errors.Is(err, waitFailure) {
		t.Fatal("missing outer failure", err)
	}
	state, err := preflightManifest(manifest)
	if err != nil || state.noopReceipt == nil {
		t.Fatal("later invocation did not find committed pair", err)
	}
	if err := metadataReplayDisposition(state.noopReceipt); !errors.Is(err, errMetadataOuterDispositionUnknown) {
		t.Fatal("failed outer completion became automatic NOOP from matching FINAL", err)
	}
	if err := metadataReplayDisposition(nil); err != nil {
		t.Fatal("fresh transaction blocked as replay", err)
	}
	for path, expected := range files {
		actual, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(actual, expected) {
			t.Fatalf("persistent evidence changed: %s: %v", path, err)
		}
	}
	if _, _, err := ReadCommittedPair(manifest); err != nil {
		t.Fatal("read-only diagnosis unavailable", err)
	}
}

package platforminstall

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCombinedAcceptanceExactOldDecoders(t *testing.T) {
	manifest := finalizeManifest(t, Manifest{
		Schema: ManifestSchemaV1, ReleaseID: "synthetic-old-decoder-positive", CityPath: "/tmp/ga-tmgr-decoder-fixture",
		Core:           Artifact{Name: "gc", Source: "/tmp/ga-tmgr-decoder-fixture/candidate", Destination: "/tmp/ga-tmgr-decoder-fixture/gc", SHA256: testSHA256([]byte("candidate")), Mode: 0o755},
		PreviousSHA256: testSHA256([]byte("previous")), BackupPath: "/tmp/ga-tmgr-decoder-fixture/backup", ReceiptPath: "/tmp/ga-tmgr-decoder-fixture/receipt",
	})
	oldManifest := marshalManifest(t, manifest)
	if _, err := LoadManifest(oldManifest); err != nil {
		t.Fatalf("valid exact old manifest refused: %v", err)
	}
	old := Receipt{Schema: receiptSchemaV1, ReleaseID: manifest.ReleaseID, ManifestSHA256: manifest.ManifestSHA256, ArtifactSHA256: manifest.Core.SHA256, Result: ResultInstalled}
	var err error
	old.ReceiptSHA256, err = receiptDigest(old)
	if err != nil {
		t.Fatal(err)
	}
	oldReceipt, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeReceipt(oldReceipt); err != nil {
		t.Fatalf("valid exact old receipt refused: %v", err)
	}
	newManifest := []byte(`{"schema":"synthetic-manifest.v1","value":"new"}`)
	newReceipt := []byte(`{"schema":"gc.synthetic-committed-pair.v1","transaction":"tx","epoch":"epoch","manifest":"digest"}`)
	for _, data := range [][]byte{newManifest, bytes.Replace(oldManifest, []byte(ManifestSchemaV1), []byte("gc.platform-install-manifest.v2"), 1)} {
		if _, err := LoadManifest(data); err == nil {
			t.Fatal("old Core accepted new manifest")
		}
	}
	for _, data := range [][]byte{newReceipt, bytes.Replace(oldReceipt, []byte(receiptSchemaV1), []byte("gc.platform-install-receipt.v2"), 1)} {
		if _, err := decodeReceipt(data); err == nil {
			t.Fatal("old Core accepted new receipt")
		}
	}
}

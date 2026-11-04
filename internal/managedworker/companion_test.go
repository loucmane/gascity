package managedworker

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestCompanionReceiptRejectsExplicitNull(t *testing.T) {
	_, data := finalizedReceipt(t)
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	doc["provider_allowlist"] = nil
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProvisioningReceipt(data); err == nil {
		t.Fatal("accepted null provider_allowlist")
	}
}

func TestCompanionPythonCanonicalReceipt(t *testing.T) {
	data, err := os.ReadFile("testdata/companion-receipt.json")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := LoadProvisioningReceipt(data)
	if err != nil {
		t.Fatal(err)
	}
	receipt.ReceiptSHA256 = ""
	for i := range receipt.Profiles {
		receipt.Profiles[i].WorkerProfileSHA256 = ""
	}
	_, canonical, err := FinalizeProvisioningReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, canonical) {
		t.Fatal("Python/Go canonical receipt disagreement")
	}
}

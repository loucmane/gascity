package managedworker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

// legacyReceiptDecode preserves the c31fb80bf receipt loader's decoding and
// validation sequence, before provider authority added an optional JSON gate.
func legacyReceiptDecode(data []byte) (ProvisioningReceipt, error) {
	var receipt ProvisioningReceipt
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return ProvisioningReceipt{}, err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return ProvisioningReceipt{}, err
	}
	if err := validateReceiptContent(receipt); err != nil {
		return ProvisioningReceipt{}, err
	}
	if err := validateSHA256("receipt_sha256", receipt.ReceiptSHA256); err != nil {
		return ProvisioningReceipt{}, err
	}
	want, err := provisioningReceiptDigest(receipt)
	if err != nil {
		return ProvisioningReceipt{}, err
	}
	if receipt.ReceiptSHA256 != want {
		return ProvisioningReceipt{}, fmt.Errorf("receipt_sha256 mismatch")
	}
	return receipt, nil
}

func TestReceiptLegacyDecoderBaseline(t *testing.T) {
	receipt, data := finalizedReceipt(t)
	// U+FFFD is how encoding/json normalizes an invalid byte; finalize that
	// normalized value so the malformed-UTF8 case has a valid canonical digest.
	receipt.Profiles[0].Provider.Version = "legacy-\ufffd"
	receipt.ReceiptSHA256 = ""
	for i := range receipt.Profiles {
		receipt.Profiles[i].WorkerProfileSHA256 = ""
	}
	_, unicode, err := FinalizeProvisioningReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	wide := func(width int) []byte {
		encoded := []byte{0xff, 0xfe}
		if width == 4 {
			encoded = append(encoded, 0, 0)
		}
		for _, b := range data {
			encoded = append(encoded, b)
			for i := 1; i < width; i++ {
				encoded = append(encoded, 0)
			}
		}
		return encoded
	}
	for name, raw := range map[string][]byte{
		"ordinary":        data,
		"duplicate field": append([]byte(`{"schema":"ignored",`), data[1:]...),
		"invalid UTF8":    bytes.Replace(unicode, []byte("\ufffd"), []byte{0xff}, 1),
		"BOM":             append([]byte{0xef, 0xbb, 0xbf}, data...),
		"UTF16":           wide(2),
		"UTF32":           wide(4),
	} {
		t.Run(name, func(t *testing.T) {
			want, oldErr := legacyReceiptDecode(raw)
			got, newErr := LoadProvisioningReceipt(raw)
			if (oldErr == nil) != (newErr == nil) || (oldErr == nil && !reflect.DeepEqual(want, got)) {
				t.Fatalf("legacy receipt changed: baseline error=%v; candidate error=%v", oldErr, newErr)
			}
		})
	}
}

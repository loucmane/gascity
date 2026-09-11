package platforminstall

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCapturedMetadataReadsReceiptManifestReceipt(t *testing.T) {
	var calls []string
	read := func(path, _ string) ([]byte, bool, error) {
		calls = append(calls, path)
		return []byte(path), true, nil
	}
	manifest, receipt, manifestExists, receiptExists, err := captureMetadataPair("manifest", "receipt", read)
	if err != nil || !manifestExists || !receiptExists || string(manifest) != "manifest" || string(receipt) != "receipt" {
		t.Fatalf("capture: %q %q %v %v %v", manifest, receipt, manifestExists, receiptExists, err)
	}
	if !reflect.DeepEqual(calls, []string{"receipt", "manifest", "receipt"}) {
		t.Fatalf("read order = %v", calls)
	}
}

func TestCapturedMetadataRefusesReceiptDriftAndReadFailure(t *testing.T) {
	for _, failure := range []string{"replacement", "appeared", "disappeared", "first", "manifest", "last"} {
		t.Run(failure, func(t *testing.T) {
			count := 0
			read := func(path, _ string) ([]byte, bool, error) {
				count++
				if failure == "first" && count == 1 || failure == "manifest" && count == 2 || failure == "last" && count == 3 {
					return nil, false, errors.New("injected read failure")
				}
				if failure == "appeared" && count == 1 || failure == "disappeared" && count == 3 {
					return nil, false, nil
				}
				if failure == "replacement" && count == 3 {
					return []byte("changed"), true, nil
				}
				return []byte(path), true, nil
			}
			if _, _, _, _, err := captureMetadataPair("manifest", "receipt", read); err == nil {
				t.Fatal("accepted unavailable or crossing receipt evidence")
			}
		})
	}
}

func TestDecodeCommittedPairUsesCapturedBytesAndRefusesMismatch(t *testing.T) {
	manifest := testManifest(t, t.TempDir(), []byte("candidate"), []byte("old"))
	receipt := Receipt{Schema: receiptSchemaV1, ReleaseID: manifest.ReleaseID, ManifestSHA256: manifest.ManifestSHA256, ArtifactSHA256: manifest.Core.SHA256, PreviousSHA256: manifest.PreviousSHA256, Result: ResultInstalled}
	if err := finalizeReceipt(&receipt); err != nil {
		t.Fatal(err)
	}
	manifestBytes := marshalManifest(t, manifest)
	receiptBytes, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := decodeCommittedPair(manifestBytes, receiptBytes); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"release", "digest", "schema", "missing", "duplicate"} {
		t.Run(mutation, func(t *testing.T) {
			changed := receipt
			switch mutation {
			case "release":
				changed.ReleaseID = "other"
			case "digest":
				changed.ManifestSHA256 = testSHA256([]byte("other"))
			case "schema":
				changed.Schema = "gc.platform-install-receipt.unsupported"
			}
			if err := finalizeReceipt(&changed); err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(changed)
			if err != nil {
				t.Fatal(err)
			}
			if mutation == "missing" {
				data = nil
			}
			if mutation == "duplicate" {
				data = bytes.Replace(data, []byte(`"result":"installed"`), []byte(`"result":"bad","result":"installed"`), 1)
			}
			if _, _, err := decodeCommittedPair(manifestBytes, data); err == nil {
				t.Fatal("accepted invalid committed pair")
			}
		})
	}
}

func TestInstalledConsumerRefusesReplacedCanonicalManifest(t *testing.T) {
	manifest := testManifest(t, t.TempDir(), []byte("candidate"), []byte("old"))
	if _, err := Install(manifest); err != nil {
		t.Fatal(err)
	}
	replacement := manifest
	replacement.ReleaseID = "different-captured-release"
	replacement = finalizeManifest(t, replacement)
	if err := os.WriteFile(DefaultManifestPath(manifest.CityPath), marshalManifest(t, replacement), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := InspectIntegrity(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Drifts) == 0 {
		t.Fatal("installed consumer accepted a replaced canonical manifest")
	}
	if _, _, err := ReadCommittedPair(replacement); err == nil {
		t.Fatal("replacement manifest accepted the old receipt")
	}
	if _, err := LoadManifest(marshalManifest(t, replacement)); err != nil {
		t.Fatalf("candidate input parsing must remain distinct: %v", err)
	}
}

func TestManifestRejectsDuplicateFieldsBeforeDigestValidation(t *testing.T) {
	manifest := testManifest(t, t.TempDir(), []byte("candidate"), []byte("old"))
	data := marshalManifest(t, manifest)
	data = bytes.Replace(data, []byte(`"schema":`), []byte(`"schema":"ignored","schema":`), 1)
	if _, err := LoadManifest(data); err == nil {
		t.Fatal("accepted duplicate manifest schema")
	}
}

func TestMetadataRejectsCaseAliasesAtEveryDepth(t *testing.T) {
	for _, data := range []string{
		`{"schema":"first","SCHEMA":"second"}`,
		`{"core":{"sha256":"first","SHA256":"second"}}`,
		`{"managed_files":[{"name":"first","NAME":"second"}]}`,
		`{"Schema":"only-alias"}`,
		`{"core":{"Source":"only-alias"}}`,
	} {
		if err := rejectDuplicateMetadataFields([]byte(data)); err == nil {
			t.Fatalf("accepted noncanonical metadata keys: %s", data)
		}
	}
}

func TestMetadataCanonicalFieldsPreserveDynamicKeys(t *testing.T) {
	var target struct {
		Values map[string]Artifact `json:"values"`
	}
	data := []byte(`{"values":{"Case.Key":{"name":"one"},"case.key":{"name":"two"},"K":{"name":"three"}}}`)
	if err := rejectDuplicateMetadataFields(data, &target); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		`{"Values":{}}`,
		`{"values":{"Case.Key":{"NAME":"bad"}}}`,
		`{"values":{"same":{},"same":{}}}`,
	} {
		if err := rejectDuplicateMetadataFields([]byte(invalid), &target); err == nil {
			t.Fatalf("accepted %s", invalid)
		}
	}
}

func TestMetadataFrameCanonicalPlanAndNamespaceMap(t *testing.T) {
	frame := metadataFrame{Kind: "PREPARED", Steps: []PlanStep{{Order: 1, Action: "verify"}}}
	var encoded bytes.Buffer
	if err := metadataEncode(&encoded, frame); err != nil {
		t.Fatal(err)
	}
	var decoded metadataFrame
	if err := metadataDecode(&encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(frame, decoded) {
		t.Fatal("canonical frame changed")
	}
	var launch MetadataLaunch
	if err := rejectDuplicateMetadataFields([]byte(`{"namespaces":{"Case":"one","case":"two"}}`), &launch); err != nil {
		t.Fatal(err)
	}
}

func TestCommittedPairRejectsAliasesWithValidOriginalDigests(t *testing.T) {
	manifest := testManifest(t, t.TempDir(), []byte("candidate"), []byte("old"))
	receipt := Receipt{Schema: receiptSchemaV1, ReleaseID: manifest.ReleaseID, ManifestSHA256: manifest.ManifestSHA256, ArtifactSHA256: manifest.Core.SHA256, PreviousSHA256: manifest.PreviousSHA256, Result: ResultInstalled}
	if err := finalizeReceipt(&receipt); err != nil {
		t.Fatal(err)
	}
	manifestData := marshalManifest(t, manifest)
	receiptData, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := decodeCommittedPair(manifestData, receiptData); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, original, alias string
		receipt               bool
	}{
		{"manifest-schema", `"schema":"` + ManifestSchemaV1 + `"`, `"SCHEMA":"unsupported"`, false},
		{"nested-core", `"name":"` + manifest.Core.Name + `"`, `"NAME":"unsupported"`, false},
		{"receipt-schema", `"schema":"` + receiptSchemaV1 + `"`, `"SCHEMA":"unsupported"`, true},
		{"receipt-result", `"result":"installed"`, `"RESULT":"unsupported"`, true},
		{"escaped-schema", `"schema":"` + ManifestSchemaV1 + `"`, `"\u0073chema":"unsupported"`, false},
	} {
		for _, before := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/alias-first=%v", test.name, before), func(t *testing.T) {
				input := manifestData
				if test.receipt {
					input = receiptData
				}
				if !bytes.Contains(input, []byte(test.original)) {
					t.Fatal("fixture field absent")
				}
				replacement := test.original + "," + test.alias
				if before {
					replacement = test.alias + "," + test.original
				}
				changed := []byte(strings.Replace(string(input), test.original, replacement, 1))
				changedManifest, changedReceipt := manifestData, receiptData
				if test.receipt {
					changedReceipt = changed
				} else {
					changedManifest = changed
				}
				if _, _, err := decodeCommittedPair(changedManifest, changedReceipt); err == nil {
					t.Fatal("accepted ambiguous bytes retaining original digest")
				}
			})
		}
	}
}

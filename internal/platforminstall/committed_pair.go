package platforminstall

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

func captureMetadataPair(manifestPath, receiptPath string, read func(string, string) ([]byte, bool, error)) ([]byte, []byte, bool, bool, error) {
	first, firstExists, err := read(receiptPath, "install receipt")
	if err != nil {
		return nil, nil, false, false, err
	}
	manifest, manifestExists, err := read(manifestPath, "canonical manifest")
	if err != nil {
		return nil, nil, false, false, err
	}
	last, lastExists, err := read(receiptPath, "install receipt recheck")
	if err != nil {
		return nil, nil, false, false, err
	}
	if firstExists != lastExists || !bytes.Equal(first, last) {
		return nil, nil, false, false, fmt.Errorf("platform receipt changed across manifest capture")
	}
	return manifest, first, manifestExists, firstExists, nil
}

func decodeCommittedPair(manifestData, receiptData []byte) (Manifest, Receipt, error) {
	manifest, err := LoadManifest(manifestData)
	if err != nil {
		return Manifest{}, Receipt{}, err
	}
	receipt, err := decodeReceipt(receiptData)
	if err != nil {
		return Manifest{}, Receipt{}, err
	}
	if !receiptMatchesManifest(receipt, manifest) {
		return Manifest{}, Receipt{}, fmt.Errorf("platform manifest and receipt are not a committed pair")
	}
	return manifest, receipt, nil
}

// ReadCommittedPair captures receipt, manifest, and receipt again, refusing
// missing, changing, unsupported, or mismatched metadata. It proves no liveness.
func ReadCommittedPair(expected Manifest) (Manifest, Receipt, error) {
	manifestData, receiptData, manifestExists, receiptExists, err := captureMetadataPair(DefaultManifestPath(expected.CityPath), expected.ReceiptPath, readOptionalRegularFile)
	if err != nil {
		return Manifest{}, Receipt{}, err
	}
	if !manifestExists || !receiptExists {
		return Manifest{}, Receipt{}, fmt.Errorf("committed platform metadata pair is incomplete")
	}
	manifest, receipt, err := decodeCommittedPair(manifestData, receiptData)
	if err != nil {
		return Manifest{}, Receipt{}, err
	}
	if manifest.CityPath != expected.CityPath || manifest.ReceiptPath != expected.ReceiptPath || manifest.ManifestSHA256 != expected.ManifestSHA256 {
		return Manifest{}, Receipt{}, fmt.Errorf("committed platform manifest differs from the captured authority")
	}
	return manifest, receipt, nil
}

func rejectDuplicateMetadataFields(data []byte, targets ...any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	schema := reflect.TypeOf(Manifest{})
	if len(targets) == 1 {
		schema = reflect.TypeOf(targets[0])
	} else if len(targets) > 1 {
		return fmt.Errorf("multiple metadata schemas")
	}
	var walk func(reflect.Type) error
	walk = func(current reflect.Type) error {
		for current != nil && current.Kind() == reflect.Pointer {
			current = current.Elem()
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, compound := token.(json.Delim)
		if !compound {
			return nil
		}
		if delimiter != '{' && delimiter != '[' {
			return fmt.Errorf("invalid metadata JSON delimiter")
		}
		seen := make(map[string]bool)
		for decoder.More() {
			var child reflect.Type
			if delimiter == '{' {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, valid := key.(string)
				if !valid || seen[name] {
					return fmt.Errorf("duplicate metadata JSON field %q", name)
				}
				if current != nil && current.Kind() == reflect.Map && current.Key().Kind() == reflect.String {
					child = current.Elem()
				} else if current != nil && current.Kind() == reflect.Struct {
					for index := 0; index < current.NumField(); index++ {
						field := current.Field(index)
						canonical := strings.Split(field.Tag.Get("json"), ",")[0]
						if canonical == "" {
							canonical = field.Name
						}
						if canonical != "" && canonical != "-" && canonical == name && field.PkgPath == "" {
							child = field.Type
							break
						}
					}
				}
				if child == nil {
					return fmt.Errorf("unknown field or noncanonical metadata JSON spelling %q", name)
				}
				seen[name] = true
			} else if current != nil && (current.Kind() == reflect.Slice || current.Kind() == reflect.Array) {
				child = current.Elem()
			}
			if err := walk(child); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	}
	if err := walk(schema); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

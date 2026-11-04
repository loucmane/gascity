package api

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/gastownhall/gascity/internal/platforminstall"
	"github.com/gastownhall/gascity/internal/providerallowlist"
	"github.com/gastownhall/gascity/internal/testutil/providerlaunch"
)

func TestDelegatedEncodedApprovalCannotRestoreLegacyExecution(t *testing.T) {
	encodings := []string{"utf8", "utf8-bom", "utf16-le", "utf16-be", "utf16-le-bom", "utf16-be-bom", "utf32-le", "utf32-be", "utf32-le-bom", "utf32-be-bom"}
	for _, kind := range []string{"manifest", "receipt"} {
		for _, encoding := range encodings {
			for _, ending := range []string{"complete", "truncated", "malformed suffix", "malformed prefix", "malformed key prefix", "key without colon"} {
				t.Run(kind+"/"+encoding+"/"+ending, func(t *testing.T) {
					home := t.TempDir()
					t.Setenv("HOME", home)
					marker := filepath.Join(home, "ran")
					fixture := providerlaunch.New(t, "claude")
					for _, path := range []string{providerallowlist.Path(fixture.City), providerallowlist.ReceiptPath(fixture.City), platforminstall.DefaultManifestPath(fixture.City)} {
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
					}
					document, closeObject := `{"note":"😀","provider_allowliſt":`, `null}`
					path := providerallowlist.ReceiptPath(fixture.City)
					if kind == "manifest" {
						document, closeObject = `{"integrity":`+document, closeObject+`}`
						path = platforminstall.DefaultManifestPath(fixture.City)
					}
					if ending == "key without colon" {
						document = strings.TrimSuffix(document, ":")
					} else if ending != "truncated" {
						document += closeObject
					}
					if ending == "malformed prefix" {
						document = `{"unrelated":invalid,` + document[1:]
					}
					if ending == "malformed key prefix" {
						document = `{"\q":0,` + document[1:]
					}
					data := encodedApprovalFixture(document, encoding)
					if ending == "malformed suffix" {
						data = append(data, 0xff)
					}
					if err := os.WriteFile(path, data, 0o600); err != nil {
						t.Fatal(err)
					}
					generic := filepath.Join(home, ".local/bin/claude")
					if err := os.MkdirAll(filepath.Dir(generic), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(generic, []byte("#!/bin/sh\necho generic > "+marker+"\necho '{\"loggedIn\":true}'\n"), 0o755); err != nil {
						t.Fatal(err)
					}
					if _, err := platforminstall.ResolveCityProviderLaunch(context.Background(), fixture.City, "", "legacy-command"); err == nil {
						t.Error("encoded surviving declaration restored legacy launch")
					}
					_, err := ProbeCityProvider(context.Background(), fixture.City, "", "claude")
					if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
						t.Fatalf("host-discovered provider executed: marker=%v error=%v", statErr, err)
					}
					if err == nil {
						t.Fatal("encoded surviving declaration restored legacy readiness")
					}
				})
			}
		}
	}
}

func encodedApprovalFixture(text, encoding string) []byte {
	if encoding == "utf8" {
		return []byte(text)
	}
	if encoding == "utf8-bom" {
		return append([]byte{0xef, 0xbb, 0xbf}, []byte(text)...)
	}
	var order binary.AppendByteOrder = binary.BigEndian
	if strings.Contains(encoding, "-le") {
		order = binary.LittleEndian
	}
	if strings.HasSuffix(encoding, "-bom") {
		text = "\ufeff" + text
	}
	var data []byte
	if strings.HasPrefix(encoding, "utf16") {
		for _, unit := range utf16.Encode([]rune(text)) {
			data = order.AppendUint16(data, unit)
		}
	} else {
		for _, point := range text {
			data = order.AppendUint32(data, uint32(point))
		}
	}
	return data
}

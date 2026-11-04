package providerallowlist

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestCompanionParserCorpus(t *testing.T) {
	data, err := os.ReadFile("testdata/companion-corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name     string `json:"name"`
		JSON     string `json:"json"`
		Hex      string `json:"hex"`
		Target   string `json:"target"`
		Accepted bool   `json:"accepted"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, item := range cases {
		t.Run(item.Name, func(t *testing.T) {
			data := []byte(item.JSON)
			if item.Hex != "" {
				data, err = hex.DecodeString(item.Hex)
				if err != nil {
					t.Fatal(err)
				}
			}
			if item.Target == "" {
				_, err = Parse(data)
			} else {
				err = ValidateOptionalJSON(data, item.Target == "manifest")
			}
			if (err == nil) != item.Accepted {
				t.Fatalf("accepted=%v, error=%v", item.Accepted, err)
			}
		})
	}
}

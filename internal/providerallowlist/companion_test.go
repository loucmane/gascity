package providerallowlist

import (
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
		Accepted bool   `json:"accepted"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, item := range cases {
		t.Run(item.Name, func(t *testing.T) {
			_, err := Parse([]byte(item.JSON))
			if (err == nil) != item.Accepted {
				t.Fatalf("accepted=%v, error=%v", item.Accepted, err)
			}
		})
	}
}

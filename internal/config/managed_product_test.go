package config

import (
	"strings"
	"testing"
)

func TestRigManagedProductRoundTripsAndPatches(t *testing.T) {
	cfg, err := Parse([]byte(`[workspace]
name = "test-city"

[[rigs]]
name = "product"
path = "/srv/product"
managed_product = true

[[rigs]]
name = "control"
path = "/srv/control"
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !cfg.Rigs[0].ManagedProduct {
		t.Fatal("product rig managed_product = false, want true")
	}
	if cfg.Rigs[1].ManagedProduct {
		t.Fatal("control rig managed_product = true, want default false")
	}

	encoded, err := cfg.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Count(string(encoded), "managed_product = true") != 1 {
		t.Fatalf("managed_product encoding =\n%s\nwant exactly one true field", encoded)
	}

	disabled := false
	if err := ApplyPatches(cfg, Patches{Rigs: []RigPatch{{Name: "product", ManagedProduct: &disabled}}}); err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	if cfg.Rigs[0].ManagedProduct {
		t.Fatal("patched product rig managed_product = true, want false")
	}
}

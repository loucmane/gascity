package config

import (
	"testing"

	"github.com/gastownhall/gascity/internal/fsys"
)

func TestRevisionTracksProviderAllowlistAndSnapshot(t *testing.T) {
	fs := fsys.NewFake()
	cfg := &City{}
	prov := &Provenance{}
	before := Revision(fs, prov, cfg, "/city")
	fs.Files["/city/managed/providers.json"] = []byte("first")
	after := Revision(fs, prov, cfg, "/city")
	if before == after {
		t.Fatal("provider allowlist is absent from permission_revision")
	}
	prov.captureRevisionSnapshot(fs, cfg, "/city")
	snapshot := Revision(fs, prov, cfg, "/city")
	fs.Files["/city/managed/providers.json"] = []byte("second")
	if snapshot != Revision(fs, prov, cfg, "/city") {
		t.Fatal("loaded snapshot changed underfoot")
	}
	if snapshot == Revision(fs, &Provenance{}, cfg, "/city") {
		t.Fatal("reload ignored provider allowlist")
	}
	found := false
	for _, target := range WatchTargets(prov, cfg, "/city") {
		if target.Path == "/city/managed/providers.json" {
			found = true
		}
	}
	if !found {
		t.Fatal("provider allowlist is not watched")
	}
}

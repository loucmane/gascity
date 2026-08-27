package main

import (
	"os"
	"testing"

	"github.com/gastownhall/gascity/internal/commandcensus"
)

// ga-d54: the platform-canary train allocated command ID 200 while the
// retry-drain-item branch independently allocated 200 to the canonical
// convoy command and its workflow compatibility alias. Pin the resolved
// allocations so the collision cannot silently return through a future
// merge: platform canary keeps 200, both retry-drain-item census rows
// move together to 201, platform adopt takes the next fresh allocation,
// and next_id advances past every allocation.
func TestRetryDrainItemCensusAllocationSurvivesPlatformCanary(t *testing.T) {
	data, err := os.ReadFile("productmetrics_command_census.json")
	if err != nil {
		t.Fatalf("read census manifest: %v", err)
	}
	manifest, err := commandcensus.DecodeManifest(data)
	if err != nil {
		t.Fatalf("decode census manifest: %v", err)
	}
	if err := commandcensus.ValidateManifest(manifest); err != nil {
		t.Fatalf("validate census manifest: %v", err)
	}
	wantIDs := map[string]uint16{
		"gc platform adopt":            202,
		"gc platform canary":           200,
		"gc convoy retry-drain-item":   201,
		"gc workflow retry-drain-item": 201,
	}
	got := make(map[string]uint16, len(wantIDs))
	for _, row := range manifest.Commands {
		if _, tracked := wantIDs[row.Path]; tracked {
			got[row.Path] = row.ID
		}
	}
	for path, want := range wantIDs {
		id, ok := got[path]
		if !ok {
			t.Errorf("census missing command %q", path)
			continue
		}
		if id != want {
			t.Errorf("census %q = id %d, want %d", path, id, want)
		}
	}
	if want := uint16(203); manifest.NextID != want {
		t.Errorf("census next_id = %d, want %d", manifest.NextID, want)
	}
}

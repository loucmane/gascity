package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/rollout/gate"
)

func TestVerifiedExactEnvStoreRetainsRelocatedClassRefusal(t *testing.T) {
	city := t.TempDir()
	writeMinimalCityToml(t, city)
	writeExecutable(t, filepath.Join(city, "bd"), "#!/bin/sh\nexit 99\n")
	t.Setenv("PATH", city)
	old := openStoreFactoryForCity
	t.Cleanup(func() { openStoreFactoryForCity = old })
	var store *beads.BdStore
	openStoreFactoryForCity = func(_ context.Context, opts beads.StoreOpenOptions) (beads.StoreOpenResult, error) {
		opened, err := opts.OpenBdStore()
		if err == nil {
			store = opened.(*beads.BdStore)
		}
		return beads.StoreOpenResult{Store: opened}, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // An unguarded command must fail without launching a process.
	_, err := openStoreResultAtForCityContext(ctx, city, city, splitCityConfig(), gate.ModeUnset, false, false, false, map[string]string{"PATH": city})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReleaseIfCurrent("gcg-abc", "worker-1"); !errors.Is(err, beads.ErrBdSQLClassRelocated) {
		t.Fatalf("exact-env store lost relocation refusal: %v", err)
	}
}

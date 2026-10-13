package main

import (
	"io"
	"path/filepath"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/fsys"
)

// loadCityConfigWithProvenance retains the exact revision snapshot used by
// policy gates. Ordinary loaders discard provenance and skip this snapshot.
func loadCityConfigWithProvenance(cityPath string, warningWriter ...io.Writer) (*config.City, *config.Provenance, error) {
	loadCityConfigCalls.Add(1)
	tomlPath := filepath.Join(cityPath, "city.toml")
	if err := ensureBuiltinPacksForConfigLoad(fsys.OSFS{}, tomlPath, resolveLoadCityConfigWarningWriter(warningWriter...)); err != nil {
		return nil, nil, err
	}
	cfg, prov, err := config.LoadWithIncludes(fsys.OSFS{}, tomlPath)
	if err != nil {
		return nil, nil, err
	}
	emitLoadCityConfigWarnings(resolveLoadCityConfigWarningWriter(warningWriter...), prov)
	warnMissingRequiredBuiltinImports(fsys.OSFS{}, cfg, tomlPath, resolveLoadCityConfigWarningWriter(warningWriter...))
	if err := validatePackRuntimeRegistrations(cfg); err != nil {
		return nil, nil, err
	}
	applyFeatureFlags(cfg)
	return cfg, prov, nil
}

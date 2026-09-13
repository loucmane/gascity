package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/session"
	"github.com/gastownhall/gascity/internal/taskattempt"
)

func attemptWorkStoreAccess(cityPath string, cfg *config.City) session.AttemptWorkStoreAccess {
	return func(ref string, use func(beads.Store) error) error {
		if ref == "" || ref == "city" {
			ref = "city:" + loadedCityName(cfg, cityPath)
		}
		if ref != "city:"+loadedCityName(cfg, cityPath) && !strings.HasPrefix(ref, "rig:") {
			return fmt.Errorf("%w: noncanonical store identity %q", taskattempt.ErrRefused, ref)
		}
		s, err := makeStoreRefResolver(cityPath, cfg)(ref)
		if err != nil {
			return err
		}
		// Always open the configured work authority, including city work. The
		// factory's session store may be relocated and contain same-ID decoys.
		return errors.Join(use(s), closeBeadStoreHandle(s))
	}
}

func reservePoolTaskAttempt(bp *agentBuildParams, template string, metadata map[string]string) (string, error) {
	id := metadata[beadmeta.TriggerBeadIDMetadataKey]
	if id == "" {
		return "", nil
	} // Undriven floor demand has no task contract.
	if bp == nil {
		return "", fmt.Errorf("%w: build context unavailable", taskattempt.ErrRefused)
	}
	if bp.attemptWorkStore == nil {
		return "", fmt.Errorf("%w: task store resolver unavailable", taskattempt.ErrRefused)
	}
	ref := metadata[beadmeta.TriggerBeadStoreRefMetadataKey]
	var token string
	err := bp.attemptWorkStore(ref, func(s beads.Store) error {
		var err error
		token, err = taskattempt.Reserve(s, id, ref, template)
		return err
	})
	return token, err
}

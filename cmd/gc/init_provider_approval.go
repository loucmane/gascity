package main

import (
	"context"

	"github.com/gastownhall/gascity/internal/api"
	"github.com/gastownhall/gascity/internal/platforminstall"
)

// Init also resumes existing cities, whose provider approval remains in force.
// A fresh undelegated city keeps the original host-discovery preflight.
func probeInitCityProviders(ctx context.Context, city string, names []string) (map[string]api.ReadinessItem, error) {
	delegated, err := platforminstall.CityDelegatesProviderApproval(city)
	if err != nil {
		return nil, err
	}
	if delegated {
		return api.ProbeCityProviders(ctx, city, names, true)
	}
	return initProbeProvidersReadiness(ctx, names, true)
}

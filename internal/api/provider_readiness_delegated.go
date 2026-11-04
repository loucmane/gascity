package api

import (
	"context"
	"fmt"

	"github.com/gastownhall/gascity/internal/platforminstall"
)

// ProbeCityProvider checks readiness using the approved native descriptor when
// the city delegates provider approval. Undelegated cities keep generic discovery.
func ProbeCityProvider(ctx context.Context, city, revision, provider string) (ReadinessItem, error) {
	file, delegated, err := platforminstall.OpenCityProviderExecutable(city, revision, provider)
	if err != nil {
		return ReadinessItem{}, err
	}
	if !delegated {
		results, err := ProbeProviders(ctx, []string{provider}, true)
		return results[provider], err
	}
	defer file.Close() //nolint:errcheck // read-only descriptor
	home, err := workspaceHomeDir()
	if err != nil {
		return ReadinessItem{}, err
	}
	var result providerProbeResult
	switch provider {
	case "claude":
		result = probeClaudeExecutable(ctx, home, file.Name(), file)
	case "codex":
		result = probeCodexAuth(home)
	default:
		return ReadinessItem{}, fmt.Errorf("provider %q has no delegated readiness probe", provider)
	}
	spec := readinessProbeSpecs[provider]
	return ReadinessItem{Name: provider, Kind: spec.kind, DisplayName: spec.displayName, Status: result.status, Detail: result.detail}, nil
}

func buildCityReadinessResponse(ctx context.Context, city string, items []string, fresh bool) (readinessResponse, error) {
	delegated, err := platforminstall.CityDelegatesProviderApproval(city)
	if err != nil {
		return readinessResponse{}, err
	}
	if !delegated {
		return buildReadinessResponse(ctx, items, fresh)
	}
	home, err := workspaceHomeDir()
	if err != nil {
		return readinessResponse{}, err
	}
	response := readinessResponse{Items: make(map[string]ReadinessItem, len(items))}
	for _, name := range items {
		spec, ok := readinessProbeSpecs[name]
		if !ok {
			return readinessResponse{}, fmt.Errorf("unsupported readiness item %q", name)
		}
		if spec.kind == ProbeKindProvider {
			item, err := ProbeCityProvider(ctx, city, "", name)
			if err != nil {
				item = ReadinessItem{Name: name, Kind: spec.kind, DisplayName: spec.displayName, Status: ProbeStatusProbeError, Detail: err.Error()}
			}
			response.Items[name] = item
		} else {
			result := probeReadinessItem(ctx, home, name, fresh)
			response.Items[name] = ReadinessItem{Name: name, Kind: spec.kind, DisplayName: spec.displayName, Status: result.status, Detail: result.detail}
		}
	}
	return response, nil
}

// ProbeCityProviders returns readiness under a city's delegated provider policy.
func ProbeCityProviders(ctx context.Context, city string, providers []string, fresh bool) (map[string]ReadinessItem, error) {
	items, err := validateRequestedReadinessItems(providers, supportedProviderReadiness, "provider")
	if err != nil {
		return nil, err
	}
	response, err := buildCityReadinessResponse(ctx, city, items, fresh)
	return response.Items, err
}

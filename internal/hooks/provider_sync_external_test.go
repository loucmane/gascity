package hooks_test

import (
	"testing"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/hooks"
)

// TestSupportsHooksSyncWithProviderSpec verifies that the hooks supported list
// stays in sync with ProviderSpec.SupportsHooks across all builtin providers.
// It lives in the external test package so the config -> runtime graph can use
// hooks as the final session-projection writer without an in-package test cycle.
func TestSupportsHooksSyncWithProviderSpec(t *testing.T) {
	sup := make(map[string]bool, len(hooks.SupportedProviders()))
	for _, provider := range hooks.SupportedProviders() {
		sup[provider] = true
	}

	providers := config.BuiltinProviders()
	for name, spec := range providers {
		supports := spec.SupportsHooks != nil && *spec.SupportsHooks
		if supports && !sup[name] {
			t.Errorf("provider %q has SupportsHooks=true but is not in hooks.SupportedProviders()", name)
		}
		if !supports && sup[name] {
			t.Errorf("provider %q is in hooks.SupportedProviders() but has SupportsHooks=false", name)
		}
	}
	for _, provider := range hooks.SupportedProviders() {
		if _, ok := providers[provider]; !ok {
			t.Errorf("hooks.SupportedProviders() contains %q which is not a builtin provider", provider)
		}
	}
}

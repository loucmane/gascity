package api

import (
	"fmt"
	"strings"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/worker"
)

func (s *Server) workerFactory(store beads.Store) (*worker.Factory, error) {
	cfg := s.state.Config()
	var resolveTransport func(template, provider string) string
	if cfg != nil {
		resolveTransport = func(template, provider string) string {
			return configuredSessionTransport(cfg, template, provider)
		}
	}
	return worker.NewFactory(worker.FactoryConfig{
		Store:                 store,
		Provider:              s.state.SessionProvider(),
		CityPath:              s.state.CityPath(),
		SearchPaths:           s.sessionLogPaths(),
		Recorder:              s.state.EventProvider(),
		UsageSink:             s.state.UsageSink(),
		ResolveTransport:      resolveTransport,
		ResolveSessionRuntime: s.resolveWorkerSessionRuntimeWithMetadata,
		AttemptWorkStore: func(ref string, use func(beads.Store) error) error {
			if ref == "" || ref == "city" || (s.state.CityName() != "" && ref == "city:"+s.state.CityName()) {
				return use(s.state.CityBeadStore())
			}
			if !strings.HasPrefix(ref, "rig:") {
				return fmt.Errorf("noncanonical task store identity %q", ref)
			}
			workStore := s.state.BeadStore(strings.TrimPrefix(ref, "rig:"))
			if workStore == nil {
				return fmt.Errorf("task store %q unavailable", ref)
			}
			return use(workStore) // Borrowed controller store: do not close.
		},
		Pricing: cfg.PricingRegistry(),
	})
}

func (s *Server) workerSessionCatalog(store beads.Store) (*worker.SessionCatalog, error) {
	factory, err := s.workerFactory(store)
	if err != nil {
		return nil, err
	}
	return factory.Catalog()
}

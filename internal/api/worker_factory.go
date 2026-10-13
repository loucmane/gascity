package api

import (
	"fmt"
	"strings"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/coordclass"
	"github.com/gastownhall/gascity/internal/storeref"
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
		AttemptWorkStore: func(ref, id string, use func(beads.Store) error) error {
			cityRef := ref == "" || ref == "city" || (s.state.CityName() != "" && ref == "city:"+s.state.CityName())
			if !cityRef && (!strings.HasPrefix(ref, "rig:") || s.state.BeadStore(strings.TrimPrefix(ref, "rig:")) == nil) {
				return fmt.Errorf("noncanonical or unavailable task store identity %q", ref)
			}
			if id != "" {
				// Driving tasks may move into the graph class. A separate session
				// store is not task authority, even if it contains the same ID.
				topology := s.residencyTopology()
				var bindings []storeref.ClassBinding
				for _, binding := range topology.Bindings {
					if containsClass(binding.Classes, coordclass.ClassGraph) {
						bindings = append(bindings, binding)
					}
				}
				topology.Bindings = bindings
				plan, err := storeref.Plan(storeref.ByID{ID: id}, topology)
				if err != nil {
					return err
				}
				owner, bound, err := storeref.ResolveBindingOwner(plan, id)
				if err != nil {
					return err
				}
				if bound {
					return use(owner.Store)
				}
			}
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
		Pricing:      cfg.PricingRegistry(),
		ActivityMemo: s.activityMemo,
	})
}

func (s *Server) workerSessionCatalog(store beads.Store) (*worker.SessionCatalog, error) {
	factory, err := s.workerFactory(store)
	if err != nil {
		return nil, err
	}
	return factory.Catalog()
}

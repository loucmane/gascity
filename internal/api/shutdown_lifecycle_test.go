package api

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/testutil"
)

type mutableCityResolver struct {
	mu     sync.RWMutex
	states map[string]State
}

type gatedConfigState struct {
	*fakeState
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *gatedConfigState) Config() *config.City {
	s.once.Do(func() { close(s.entered) })
	<-s.release
	return s.fakeState.Config()
}

func (r *mutableCityResolver) ListCities() []CityInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]CityInfo, 0, len(r.states))
	for name, state := range r.states {
		items = append(items, CityInfo{Name: name, Path: state.CityPath(), Running: true})
	}
	return items
}

func (r *mutableCityResolver) CityState(name string) State {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.states[name]
}

func (r *mutableCityResolver) set(name string, state State) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if state == nil {
		delete(r.states, name)
		return
	}
	r.states[name] = state
}

type cancelThenBlockStartProvider struct {
	*runtime.Fake
	started     chan struct{}
	canceled    chan struct{}
	release     chan struct{}
	finished    chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

func newCancelThenBlockStartProvider() *cancelThenBlockStartProvider {
	return &cancelThenBlockStartProvider{
		Fake:     runtime.NewFake(),
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
		finished: make(chan struct{}),
	}
}

func (p *cancelThenBlockStartProvider) Start(ctx context.Context, _ string, _ runtime.Config) error {
	p.startedOnce.Do(func() { close(p.started) })
	<-ctx.Done()
	close(p.canceled)
	<-p.release
	close(p.finished)
	return ctx.Err()
}

func (p *cancelThenBlockStartProvider) unblock() {
	p.releaseOnce.Do(func() { close(p.release) })
}

func TestSupervisorPerCityReplacementQuiescesBlockedSessionStart(t *testing.T) {
	for _, tc := range []struct {
		name    string
		replace func(*testing.T, *mutableCityResolver, string)
		wantNil bool
	}{
		{
			name: "unregister",
			replace: func(_ *testing.T, resolver *mutableCityResolver, name string) {
				resolver.set(name, nil)
			},
			wantNil: true,
		},
		{
			name: "state replacement",
			replace: func(t *testing.T, resolver *mutableCityResolver, name string) {
				replacement := newSessionFakeState(t)
				replacement.cityName = name
				resolver.set(name, replacement)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := newSessionFakeState(t)
			state.cityName = "lifecycle-city"
			provider := newCancelThenBlockStartProvider()
			wrapped := &stateWithSessionProvider{fakeState: state, provider: provider}
			resolver := &mutableCityResolver{states: map[string]State{state.cityName: wrapped}}
			sm := NewSupervisorMux(resolver, nil, false, "test", "", time.Now())
			t.Cleanup(func() {
				provider.unblock()
				ctx, cancel := context.WithTimeout(context.Background(), testutil.GoroutineRaceTimeout)
				defer cancel()
				if err := sm.Shutdown(ctx); err != nil {
					t.Errorf("Shutdown: %v", err)
				}
			})

			oldServer := sm.resolveCityServer(state.cityName)
			if oldServer == nil {
				t.Fatal("initial city server is nil")
			}
			if _, err := oldServer.humaCreateProviderSession(context.Background(), state.SessionsBeadStore(), sessionCreateBody{
				Kind: "provider",
				Name: "test-agent",
			}, "test-agent"); err != nil {
				t.Fatalf("humaCreateProviderSession: %v", err)
			}
			select {
			case <-provider.started:
			case <-time.After(testutil.GoroutineRaceTimeout):
				t.Fatal("timed out waiting for blocked session start")
			}

			tc.replace(t, resolver, state.cityName)
			resolved := make(chan *Server, 1)
			go func() { resolved <- sm.resolveCityServer(state.cityName) }()

			select {
			case <-provider.canceled:
			case <-time.After(testutil.GoroutineRaceTimeout):
				t.Fatal("per-city replacement did not cancel the old Server's session start")
			}
			select {
			case got := <-resolved:
				t.Fatalf("per-city replacement returned Server %p before joining the canceled start", got)
			default:
			}

			provider.unblock()
			var got *Server
			select {
			case got = <-resolved:
			case <-time.After(testutil.GoroutineRaceTimeout):
				t.Fatal("timed out waiting for per-city replacement to join the canceled start")
			}
			select {
			case <-provider.finished:
			default:
				t.Fatal("per-city replacement returned before the old start finished")
			}
			if tc.wantNil && got != nil {
				t.Fatalf("resolved Server = %p after unregister, want nil", got)
			}
			if !tc.wantNil && (got == nil || got == oldServer) {
				t.Fatalf("resolved Server = %p after state replacement, want a new Server (old %p)", got, oldServer)
			}
		})
	}

	t.Run("unregister serializes pending replacement publication", func(t *testing.T) {
		oldState := newSessionFakeState(t)
		oldState.cityName = "transition-city"
		resolver := &mutableCityResolver{states: map[string]State{oldState.cityName: oldState}}
		sm := NewSupervisorMux(resolver, nil, false, "test", "", time.Now())
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), testutil.GoroutineRaceTimeout)
			defer cancel()
			if err := sm.Shutdown(ctx); err != nil {
				t.Errorf("Shutdown: %v", err)
			}
		})
		if srv := sm.resolveCityServer(oldState.cityName); srv == nil {
			t.Fatal("initial city server is nil")
		}

		replacementState := &gatedConfigState{
			fakeState: newSessionFakeState(t),
			entered:   make(chan struct{}),
			release:   make(chan struct{}),
		}
		replacementState.cityName = oldState.cityName
		resolver.set(oldState.cityName, replacementState)
		resolved := make(chan *Server, 1)
		go func() { resolved <- sm.resolveCityServer(oldState.cityName) }()
		select {
		case <-replacementState.entered:
		case <-time.After(testutil.GoroutineRaceTimeout):
			t.Fatal("timed out waiting for replacement Server construction")
		}

		resolver.set(oldState.cityName, nil)
		unregistered := make(chan *Server, 1)
		go func() { unregistered <- sm.resolveCityServer(oldState.cityName) }()
		select {
		case srv := <-unregistered:
			t.Fatalf("unregister resolution returned Server %p before pending replacement publication completed", srv)
		default:
		}

		close(replacementState.release)
		select {
		case <-resolved:
		case <-time.After(testutil.GoroutineRaceTimeout):
			t.Fatal("timed out waiting for replacement Server publication")
		}
		select {
		case srv := <-unregistered:
			if srv != nil {
				t.Fatalf("resolved Server = %p after unregister, want nil", srv)
			}
		case <-time.After(testutil.GoroutineRaceTimeout):
			t.Fatal("timed out waiting for unregister quiescence")
		}

		sm.cacheMu.RLock()
		_, cached := sm.cache[oldState.cityName]
		sm.cacheMu.RUnlock()
		if cached {
			t.Fatal("pending replacement Server was cached after unregister quiescence returned")
		}
	})
}

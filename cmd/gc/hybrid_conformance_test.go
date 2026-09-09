//go:build integration

package main

import (
	"sync/atomic"
	"testing"

	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/runtime/runtimetest"
)

// Mixed names make concurrent starts and discovery cross both real adapters.
func TestHybridConformance(t *testing.T) {
	var counter int64
	runtimetest.RunProviderTests(t, func(t *testing.T) (runtime.Provider, runtime.Config, string) {
		provider, err := newHybridProvider(hybridConformanceConfig(t), "unused-fixture-city", "")
		if err != nil {
			t.Fatal(err)
		}
		return provider, runtime.Config{Command: "sleep 300", WorkDir: t.TempDir()}, hybridConformanceName("mixed", atomic.AddInt64(&counter, 1))
	})
}

// Neither complete single-route suite substitutes for the mixed-route proof.
func TestHybridLocalConformance(t *testing.T) {
	var counter int64
	runtimetest.RunProviderTests(t, func(t *testing.T) (runtime.Provider, runtime.Config, string) {
		provider, err := newHybridProvider(hybridConformanceConfig(t), "unused-fixture-city", "")
		if err != nil {
			t.Fatal(err)
		}
		return provider, runtime.Config{Command: "sleep 300", WorkDir: t.TempDir()}, hybridConformanceName("local", atomic.AddInt64(&counter, 1))
	})
}

func TestHybridRemoteConformance(t *testing.T) {
	var counter int64
	runtimetest.RunProviderTests(t, func(t *testing.T) (runtime.Provider, runtime.Config, string) {
		provider, err := newHybridProvider(hybridConformanceConfig(t), "unused-fixture-city", "")
		if err != nil {
			t.Fatal(err)
		}
		return provider, runtime.Config{Command: "sleep 300", WorkDir: t.TempDir()}, hybridConformanceName("remote", atomic.AddInt64(&counter, 1))
	})
}

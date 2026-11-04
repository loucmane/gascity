package main

import (
	"context"
	"io"
	"testing"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/testutil/providerlaunch"
)

func TestDelegatedPromptRefusesUnapprovedCommand(t *testing.T) {
	f := providerlaunch.New(t, "claude")
	called := false
	runner := func(context.Context, *config.ResolvedProvider, string, string) (string, error) {
		called = true
		return "prompt", nil
	}
	err := runDirectSynth(context.Background(), promptSynthOpts{}, f.City, "worker", "test", metaPromptCtx{CityPath: f.City}, &config.ResolvedProvider{Command: "/unapproved"}, runner, io.Discard, io.Discard)
	if called || err == nil {
		t.Fatalf("unapproved prompt executed: called=%v, err=%v", called, err)
	}
}

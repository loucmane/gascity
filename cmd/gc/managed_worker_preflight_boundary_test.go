package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/runtime"
	sessionpkg "github.com/gastownhall/gascity/internal/session"
)

func TestStartPreparedStartCandidateRefusesProviderStartWhenManagedPreflightFails(t *testing.T) {
	sp := runtime.NewFake()
	item := preparedStart{
		candidate: startCandidate{
			info: sessionpkg.Info{
				SessionName:         "managed-worker",
				SessionNameMetadata: "managed-worker",
			},
			tp: TemplateParams{TemplateName: "worker"},
		},
		cfg: runtime.Config{
			Command: "codex --ask-for-approval never --sandbox workspace-write",
			WorkDir: t.TempDir(),
		},
		preflight: func(context.Context) error {
			return errors.New("rules sha256 mismatch")
		},
	}

	started, err := startPreparedStartCandidate(context.Background(), item, t.TempDir(), nil, sp, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "managed-worker preflight") || !strings.Contains(err.Error(), "rules sha256 mismatch") {
		t.Fatalf("start error = %v", err)
	}
	if started {
		t.Fatal("started = true, want false when preflight rejects")
	}
	for _, call := range sp.Calls {
		if call.Method == "Start" {
			t.Fatalf("runtime calls = %#v, provider Start must not run after failed preflight", sp.Calls)
		}
	}
}

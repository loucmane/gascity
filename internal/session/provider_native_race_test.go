package session

import (
	"context"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/shellquote"
	"github.com/gastownhall/gascity/internal/testutil/providerlaunch"
)

type nativeExecRaceProvider struct {
	*runtime.Fake
	t              *testing.T
	fixture        providerlaunch.Fixture
	replace, stale bool
	armed          bool
	commands       []string
	output         string
	err            error
}

func (p *nativeExecRaceProvider) Start(ctx context.Context, name string, cfg runtime.Config) error {
	if !p.armed {
		return p.Fake.Start(ctx, name, cfg)
	}
	p.commands = append(p.commands, cfg.Command)
	if p.stale {
		p.stale = false
		return runtime.ErrSessionDiedDuringStartup
	}
	p.output, p.err = p.fixture.Run(p.t, cfg.Command, p.replace)
	if p.err != nil {
		return p.err
	}
	return p.Fake.Start(ctx, name, cfg)
}

func TestNativeLaunchRuntimeReplacement(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		for _, path := range []string{"fresh", "resume", "fallback", "runtime-only fallback"} {
			for _, replace := range []bool{false, true} {
				name := provider + "/" + path + "/approved"
				if replace {
					name = provider + "/" + path + "/replacement"
				}
				t.Run(name, func(t *testing.T) {
					f := providerlaunch.New(t, provider)
					sp := &nativeExecRaceProvider{Fake: runtime.NewFake(), t: t, fixture: f, replace: replace}
					mgr := NewManagerWithOptions(beads.NewMemStore(), sp, WithCityPath(f.City), WithStaleKeyDetectionWaiter(immediateStaleKeyDetectionWaiter))
					sp.armed = path == "fresh"
					info, err := mgr.CreateSession(context.Background(), CreateOptions{
						Template: "worker", Command: shellquote.Quote(f.Stable),
						WorkDir: f.City, Provider: provider, Resume: ProviderResume{ResumeFlag: "--resume", SessionIDFlag: "--session-id"},
						ExtraMeta: map[string]string{"session_origin": "manual"},
					})
					if path != "fresh" {
						if err != nil {
							t.Fatal(err)
						}
						if err := mgr.Suspend(info.ID); err != nil {
							t.Fatal(err)
						}
						sp.armed = true
						sp.stale = strings.Contains(path, "fallback")
						command := shellquote.Join([]string{f.Stable, "--resume", info.SessionKey})
						if path == "runtime-only fallback" {
							err = mgr.StartRuntimeOnly(context.Background(), info.ID, command, runtime.Config{WorkDir: f.City})
						} else {
							err = mgr.Send(context.Background(), info.ID, "hello", command, runtime.Config{WorkDir: f.City})
						}
					}
					wantCalls := 1
					if strings.Contains(path, "fallback") {
						wantCalls = 2
					}
					if len(sp.commands) != wantCalls {
						t.Fatalf("runtime calls = %d, want %d; error %v", len(sp.commands), wantCalls, err)
					}
					if replace {
						if strings.Contains(sp.output, "UNAPPROVED") || err == nil || sp.err == nil {
							t.Fatalf("runtime executed replacement: output=%q error=%v", sp.output, err)
						}
					} else if err != nil || !strings.HasPrefix(sp.output, "APPROVED\n") {
						t.Fatalf("approved runtime launch: %q, %v", sp.output, err)
					}
					if strings.Contains(path, "fallback") && strings.Contains(sp.commands[1], "--resume") {
						t.Fatalf("fallback retained resume: %q", sp.commands[1])
					}
				})
			}
		}
	}
}

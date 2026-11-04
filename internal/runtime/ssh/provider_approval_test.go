package ssh

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/testutil/providerlaunch"
)

func TestDelegatedSSHRefusesBeforeRemoteLaunch(t *testing.T) {
	for _, mode := range []string{"start", "resume", "relaunch", "provision", "launch"} {
		t.Run(mode, func(t *testing.T) {
			fixture := providerlaunch.New(t, "codex")
			fake := &fakeRunner{respond: func([]string) ([]byte, int, error) {
				return nil, -1, errors.New("fixture transport stop")
			}}
			p := providerWith(fake)
			cfg := runtime.Config{Command: fixture.Stable, Env: map[string]string{"GC_CITY_PATH": fixture.City}}
			var err error
			switch mode {
			case "start", "resume":
				if mode == "resume" {
					cfg.Command += " resume fixture"
				}
				err = p.Start(context.Background(), "approval", cfg)
			case "relaunch":
				err = p.Relaunch(context.Background(), "approval", cfg)
			case "provision":
				rt, _ := p.Seams()
				_, err = rt.Provision(context.Background(), "approval", runtime.ProvisionRequest{Config: cfg})
			case "launch":
				_, tp := p.Seams()
				_, err = tp.Launch(context.Background(), &sshPlace{p: p, name: "approval"}, runtime.LaunchSpec{Config: cfg})
			}
			if err == nil || !strings.Contains(err.Error(), "ssh") || !strings.Contains(err.Error(), "provider allowlist") {
				t.Errorf("wanted explicit delegated SSH refusal, got %v", err)
			}
			if len(fake.calls) != 0 {
				t.Errorf("delegated launch reached remote operations: %v", fake.calls)
			}
		})
	}
}

func TestUndelegatedSSHStillReachesRemoteLaunch(t *testing.T) {
	fake := &fakeRunner{respond: func(argv []string) ([]byte, int, error) {
		if isTmux("has-session")(argv) {
			return nil, 1, nil
		}
		return nil, 0, nil
	}}
	p := providerWith(fake)
	if err := p.Start(context.Background(), "legacy", runtime.Config{Command: "/fixture/cli", Env: map[string]string{"GC_CITY_PATH": t.TempDir()}}); err != nil {
		t.Fatal(err)
	}
	if firstCall(fake, isTmux("new-session")) == nil {
		t.Fatal("undelegated startup no longer reaches remote tmux creation")
	}
}

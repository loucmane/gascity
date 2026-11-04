package exec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/shellquote"
	"github.com/gastownhall/gascity/internal/testutil/providerlaunch"
)

func TestDelegatedExecRefusesBeforeAdapterLaunch(t *testing.T) {
	for _, mode := range []string{"start", "resume", "relaunch", "provision", "launch", "seam-start"} {
		t.Run(mode, func(t *testing.T) {
			fixture := providerlaunch.New(t, "codex")
			dir := t.TempDir()
			marker := filepath.Join(dir, "adapter-called")
			script := writeScript(t, dir, "echo called >> "+shellquote.Quote(marker)+"\nexit 1\n")
			p := NewProvider(script)
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
				_, err = tp.Launch(context.Background(), &execPlace{p: p, name: "approval"}, runtime.LaunchSpec{Config: cfg})
			case "seam-start":
				err = NewSeamBacked(script).Start(context.Background(), "approval", cfg)
			}
			if err == nil || !strings.Contains(err.Error(), "exec") || !strings.Contains(err.Error(), "provider allowlist") {
				t.Errorf("wanted explicit delegated exec refusal, got %v", err)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Errorf("delegated launch invoked the adapter: %v", err)
			}
		})
	}
}

func TestUndelegatedExecStillReachesAdapterLaunch(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "adapter-called")
	script := writeScript(t, dir, "echo called >> "+shellquote.Quote(marker)+"\nexit 1\n")
	_ = NewProvider(script).Start(context.Background(), "legacy", runtime.Config{Command: "/fixture/cli", Env: map[string]string{"GC_CITY_PATH": t.TempDir()}})
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("undelegated startup no longer reaches the adapter: %v", err)
	}
}

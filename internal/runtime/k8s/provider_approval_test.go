package k8s

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/testutil/providerlaunch"
)

func TestDelegatedK8sRefusesBeforeRemoteLaunch(t *testing.T) {
	for _, mode := range []string{"start", "resume", "relaunch", "provision", "launch"} {
		t.Run(mode, func(t *testing.T) {
			fixture := providerlaunch.New(t, "codex")
			fake := newFakeK8sOps()
			fake.createErr = errors.New("fixture stops before waiting for a pod")
			p := &Provider{ops: fake, image: "fixture", stderr: io.Discard}
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
				_, err = tp.Launch(context.Background(), &k8sPlace{p: p, name: "approval"}, runtime.LaunchSpec{Config: cfg})
			}
			if err == nil || !strings.Contains(err.Error(), "k8s") || !strings.Contains(err.Error(), "provider allowlist") {
				t.Errorf("wanted explicit delegated k8s refusal, got %v", err)
			}
			if len(fake.calls) != 0 {
				t.Errorf("delegated launch reached remote operations: %v", fake.calls)
			}
		})
	}
}

func TestUndelegatedK8sStillReachesRemoteLaunch(t *testing.T) {
	fake := newFakeK8sOps()
	fake.createErr = errors.New("fixture stops before waiting for a pod")
	p := &Provider{ops: fake, image: "fixture", stderr: io.Discard}
	_ = p.Start(context.Background(), "legacy", runtime.Config{Command: "/fixture/cli", Env: map[string]string{"GC_CITY_PATH": t.TempDir()}})
	for _, call := range fake.calls {
		if call.method == "createPod" {
			return
		}
	}
	t.Fatal("undelegated startup no longer reaches pod creation")
}

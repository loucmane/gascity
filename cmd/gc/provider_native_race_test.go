package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/managedworker"
	"github.com/gastownhall/gascity/internal/providerallowlist"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/shellquote"
	"github.com/gastownhall/gascity/internal/testutil/providerlaunch"
)

type nativeRelaunchRaceProvider struct {
	*runtime.Fake
	t       *testing.T
	fixture providerlaunch.Fixture
	replace bool
	called  bool
	output  string
	err     error
}

func (p *nativeRelaunchRaceProvider) Relaunch(ctx context.Context, name string, cfg runtime.Config) error {
	p.called = true
	p.output, p.err = p.fixture.Run(p.t, cfg.Command, p.replace)
	if p.err != nil {
		return p.err
	}
	return p.Fake.Relaunch(ctx, name, cfg)
}

func TestNativeRelaunchRuntimeReplacement(t *testing.T) {
	for _, replace := range []bool{false, true} {
		name := "approved"
		if replace {
			name = "replacement"
		}
		t.Run(name, func(t *testing.T) {
			f := providerlaunch.New(t, "claude")
			env, tp, bead := setupLaunchDriftResumeEnv(t)
			tp.Command = shellquote.Quote(f.Stable)
			env.desiredState["worker"] = tp
			receipt, _, _ := dispatchGateFixture(t)
			// This unrelated valid profile lets the real managed preflight parse
			// the approval receipt without invoking its infrastructure probes.
			list, err := os.ReadFile(providerallowlist.Path(f.City))
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(list)
			receipt.ProviderAllowlist = &managedworker.FilePin{Path: providerallowlist.Path(f.City), SHA256: hex.EncodeToString(hash[:])}
			receipt.ReceiptSHA256 = ""
			for i := range receipt.Profiles {
				receipt.Profiles[i].WorkerProfileSHA256 = ""
			}
			_, data, err := managedworker.FinalizeProvisioningReceipt(receipt)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(providerallowlist.ReceiptPath(f.City), data, 0o600); err != nil {
				t.Fatal(err)
			}
			sp := &nativeRelaunchRaceProvider{Fake: env.sp, t: t, fixture: f, replace: replace}
			reconcileSessionBeadsAtPath(context.Background(), f.City, []beads.Bead{bead}, env.desiredState,
				configuredSessionNames(env.cfg, "", env.store), env.cfg, sp, env.store,
				nil, nil, nil, nil, env.dt, map[string]int{"worker": 1}, false, nil, "",
				nil, env.clk, env.rec, 0, 0, &env.stdout, &env.stderr, env.startOptions...)
			if !sp.called {
				t.Fatalf("relaunch never reached runtime: %s", env.stderr.String())
			}
			if replace {
				if strings.Contains(sp.output, "UNAPPROVED") || sp.err == nil {
					t.Fatalf("relaunch executed replacement: %q, %v", sp.output, sp.err)
				}
			} else if sp.err != nil || !strings.HasPrefix(sp.output, "APPROVED\n") {
				t.Fatalf("approved relaunch: %q, %v", sp.output, sp.err)
			}
		})
	}
}

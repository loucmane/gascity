package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/session"
)

// VerifiedSession is the immutable identity and work-store scope of one hook
// invocation. Session access always uses the same routed front door and exact
// ID. No helper may resolve a runtime name or reload identity from the env.
type VerifiedSession struct {
	session.VerifiedSession
	Rig               string
	AllowedWorkStores []hookStore
	front             *session.Store
	cityPath          string
	cfg               *config.City
	drainRequested    bool
}

var openHookSessionFrontDoor = func(cityPath string, cfg *config.City) (*session.Store, error) {
	store, err := openCityStoreAt(cityPath)
	if err != nil {
		return nil, err
	}
	return cliSessionFrontDoor(store, cfg, cityPath), nil
}

func (v *VerifiedSession) identifiers() []string {
	return hookClaimIdentityCandidates(v.ID, v.SessionName, v.Alias, v.AgentName, v.ConfiguredNamedIdentity)
}

func (v *VerifiedSession) assignee() string {
	if v.Alias != "" {
		return v.Alias
	}
	return v.ID
}

func (v *VerifiedSession) checkEnvironment(env []string, args []string) error {
	for _, key := range []string{"GC_SESSION_NAME", "GC_TMUX_SESSION"} {
		if value := hookClaimEnvValue(env, key); value != "" && value != v.SessionName {
			return fmt.Errorf("%s disagrees with verified session", key)
		}
	}
	if value := hookClaimEnvValue(env, "GC_TEMPLATE"); value != "" && value != v.Template {
		return fmt.Errorf("GC_TEMPLATE disagrees with verified session")
	}
	for _, key := range []string{"GC_ALIAS", "GC_AGENT"} {
		value := hookClaimEnvValue(env, key)
		if value != "" && !slices.Contains(v.identifiers(), value) && (key != "GC_AGENT" || value != v.Template) {
			return fmt.Errorf("%s disagrees with verified session", key)
		}
	}
	if len(args) > 0 && args[0] != v.Template && !slices.Contains(v.identifiers(), args[0]) {
		return fmt.Errorf("positional agent is outside verified session")
	}
	return nil
}

func (v *VerifiedSession) drainPending() (bool, error) {
	info, err := v.front.ReadVerified(v.VerifiedSession)
	return v.drainRequested || info.State == session.StateDraining, err
}

func (v *VerifiedSession) mayClaim() error {
	pending, err := v.drainPending()
	if err != nil {
		return err
	}
	if pending {
		return session.ErrStaleSession
	}
	return nil
}

func (v *VerifiedSession) workDir() string {
	info, err := v.front.ReadVerified(v.VerifiedSession)
	if err != nil {
		return ""
	}
	return sessionStampableWorkDir(info)
}

// bindVerifiedHookOps preserves the upstream test/legacy seams while ensuring
// every managed operation closes over the verified identity, never its string
// argument. The conditional stamp is mandatory in writeHookClaimWorkResult.
func bindVerifiedHookOps(ops *hookClaimOps, v *VerifiedSession) {
	if v == nil {
		return
	}
	ops.applyDefaults()
	claim, restamp, stamp := ops.Claim, ops.RestampAdopted, ops.StampWorkMeta
	ops.Claim = func(ctx context.Context, dir string, env []string, id, assignee string) (beads.Bead, bool, error) {
		if err := v.mayClaim(); err != nil {
			return beads.Bead{}, false, err
		}
		return claim(ctx, dir, env, id, assignee)
	}
	ops.RestampAdopted = func(ctx context.Context, dir string, env []string, id, from, to string) (bool, error) {
		if err := v.mayClaim(); err != nil {
			return false, err
		}
		return restamp(ctx, dir, env, id, from, to)
	}
	ops.StampWorkMeta = func(ctx context.Context, dir string, env []string, id, assignee string, patch map[string]string) error {
		if err := v.mayClaim(); err != nil {
			return err
		}
		return stamp(ctx, dir, env, id, assignee, patch)
	}
	ops.DrainPending = func(string) (bool, error) { return v.drainPending() }
	ops.ResolveSessionWorkDir = func(string) string { return v.workDir() }
	ops.DrainAck = func(stderr io.Writer) error { return v.drainAck(ops.ClassRoute, stderr) }
}

func verifiedHookFailure(err error, opts hookClaimOptions, stdout, stderr io.Writer) int {
	fmt.Fprintf(stderr, "gc hook --claim: verified session refused: %v; no session stamp published\n", err) //nolint:errcheck
	reason := hookClaimReasonClaimsErrored
	if errors.Is(err, session.ErrStaleSession) {
		reason = hookClaimReasonStaleSession
	}
	return writeHookClaimDrain(hookClaimLabel, reason, opts.JSON, false, nil, stdout, stderr)
}

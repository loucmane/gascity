package main

import (
	"fmt"
	"io"

	"github.com/gastownhall/gascity/internal/nudgequeue"
)

func (t nudgeTarget) queueScope() (nudgequeue.Scope, error) {
	mode := ""
	if t.cfg != nil {
		mode = t.cfg.Session.NudgeQueueScope
	}
	return nudgequeue.NewScope(mode, t.sessionID, t.continuationEpoch)
}

// The optional argument preserves existing unscoped callers. Production target
// paths always pass the scope resolved before their first mutation.
func firstNudgeScope(scopes []nudgequeue.Scope) nudgequeue.Scope {
	if len(scopes) > 1 {
		panic("multiple nudge scopes")
	}
	if len(scopes) == 1 {
		return scopes[0]
	}
	return nudgequeue.Scope{}
}

func enqueueNudgeForTarget(target nudgeTarget, item queuedNudge) error {
	scope, err := target.queueScope()
	if err != nil {
		return err
	}
	return enqueueQueuedNudge(target.cityPath, item, scope)
}

func withdrawWaitNudgesForScope(cityPath string, ids []string, scope nudgequeue.Scope) error {
	if !scope.Strict() {
		return nudgeWithdrawQueuedWaitNudges(cityPath, ids)
	}
	store := openNudgeBeadStore(cityPath)
	defer closeBeadStoreHandle(store.Store) //nolint:errcheck // best-effort
	return scope.WithdrawWaitNudges(store.Store, cityPath, ids)
}

func validateNudgePollScope(target nudgeTarget, pinned nudgequeue.Scope) error {
	cfg, err := loadCityConfigWithoutBuiltinPackRefresh(target.cityPath, io.Discard)
	if err != nil {
		return fmt.Errorf("rechecking pinned nudge queue scope: %w", err)
	}
	if cfg == nil {
		return fmt.Errorf("pinned nudge queue configuration missing")
	}
	current, err := nudgequeue.NewScope(cfg.Session.NudgeQueueScope, target.sessionID, target.continuationEpoch)
	if err != nil {
		return err
	}
	if current != pinned {
		return fmt.Errorf("pinned nudge queue scope changed; refusing downgrade")
	}
	return nil
}

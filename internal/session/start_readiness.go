package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
)

const sessionIdentityProjectionPollInterval = 50 * time.Millisecond

// waitForStartIdentityReadable gates provider startup on the authoritative
// store projection of the session identity. Manager lifecycle reads normally
// use the controller's write-through cache, which can contain a just-created
// session before the live store has committed it. The live handle bypasses
// that cache; once the row is readable, the controller publishes the
// worker-readable claim-fence projection before Provider.Start.
//
// A missing row is the asynchronous projection case and waits under the
// caller's existing startup context. Other read failures fail the start
// immediately. There is deliberately no independent elapsed-time budget here:
// reconciler starts already carry session.startup_timeout, and direct callers
// retain control through their context.
func (m *Manager) waitForStartIdentityReadable(ctx context.Context, id, instanceToken string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	live := beads.HandlesFor(m.store).Live
	if live == nil {
		return fmt.Errorf("session %q identity live reader is unavailable", id)
	}

	ticker := time.NewTicker(sessionIdentityProjectionPollInterval)
	defer ticker.Stop()
	for {
		b, err := live.Get(id)
		if err == nil {
			switch {
			case !IsSessionBeadOrRepairable(b):
				return fmt.Errorf("session %q identity resolved to non-session bead type %q", id, b.Type)
			case b.Status == "closed":
				return fmt.Errorf("%w: %s", ErrSessionClosed, id)
			case strings.TrimSpace(instanceToken) == "":
				return fmt.Errorf("session %q has no instance token before provider start", id)
			case b.Metadata["instance_token"] != instanceToken:
				return fmt.Errorf("session %q identity changed before provider start", id)
			}
			projectionState := State(b.Metadata["state"])
			projection := SessionFenceProjection{State: string(projectionState)}
			if !projection.ClaimEligible() {
				projectionState = StateCreating
			}
			if err := m.publishSessionFenceProjection(id, instanceToken, sessionFenceGeneration(b.Metadata["generation"]), projectionState); err != nil {
				return fmt.Errorf("publishing session %q claim-fence projection: %w", id, err)
			}
			return nil
		}
		if !errors.Is(err, beads.ErrNotFound) {
			return fmt.Errorf("reading session %q identity from live store: %w", id, err)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for session %q identity projection: %w (last read: %w)", id, ctx.Err(), err)
		case <-ticker.C:
		}
	}
}

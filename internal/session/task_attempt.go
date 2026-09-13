package session

import (
	"context"
	"fmt"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/taskattempt"
)

// AttemptWorkStoreAccess resolves configured store identities, never paths from
// task metadata. The owner closes any newly opened handle after use returns;
// borrowed controller handles remain owned by the controller.
type AttemptWorkStoreAccess func(ref string, use func(beads.Store) error) error

// WithAttemptWorkStoreAccess supplies the configured driving-task authority.
func WithAttemptWorkStoreAccess(access AttemptWorkStoreAccess) ManagerOption {
	return func(m *Manager) { m.attemptWorkStore = access }
}

// startRuntime is the sole native provider-start boundary, including fresh
// fallback after a stale resume key. Read current session metadata here so
// deleting a token or changing a trigger does not bypass a cached decision.
func (m *Manager) startRuntime(ctx context.Context, id, name string, cfg runtime.Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.store == nil {
		return fmt.Errorf("%w: authoritative session store unavailable", taskattempt.ErrRefused)
	}
	live := beads.HandlesFor(m.store).Live
	if live == nil {
		return fmt.Errorf("%w: authoritative session reader unavailable", taskattempt.ErrRefused)
	}
	b, err := live.Get(id)
	if err != nil {
		return fmt.Errorf("native attempt session read: %w", err)
	}
	trigger, ref := b.Metadata[beadmeta.TriggerBeadIDMetadataKey], b.Metadata[beadmeta.TriggerBeadStoreRefMetadataKey]
	token, marked := b.Metadata[beadmeta.NativeAttemptTokenMetadataKey]
	if marked && token == "" {
		return fmt.Errorf("%w: empty bounded-session token", taskattempt.ErrRefused)
	}
	if trigger == "" {
		if marked {
			return fmt.Errorf("%w: bounded session lost trigger identity", taskattempt.ErrRefused)
		}
		return m.sp.Start(ctx, name, cfg)
	}
	use := func(s beads.Store) error {
		return taskattempt.Start(s, trigger, ref, b.Metadata["template"], token, id)
	}
	switch {
	case m.attemptWorkStore != nil:
		err = m.attemptWorkStore(ref, use)
	case ref == "" || ref == "city":
		err = use(m.store) // Legacy same-store task associations only.
	default:
		err = fmt.Errorf("%w: task store resolver unavailable", taskattempt.ErrRefused)
	}
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	} // A canceled consumed attempt stays spent.
	return m.sp.Start(ctx, name, cfg)
}

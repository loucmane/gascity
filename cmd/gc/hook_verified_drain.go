package main

import (
	"fmt"
	"io"
	"slices"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/session"
)

// Provider construction receives only the authenticated row, with its raw token
// removed by ReadVerified. It never loads the all-session provider snapshot.
var newVerifiedHookProvider = func(v *VerifiedSession, info session.Info) (runtime.Provider, error) {
	ctx := sessionProviderContextForCity(v.cfg, v.cityPath, "")
	return withSessionProviderConstructionContext(newSessionProviderFromContext(ctx,
		newSessionBeadSnapshotFromInfos([]session.Info{info})))
}

func (v *VerifiedSession) drainAck(route *hookClaimClassRoute, stderr io.Writer) error {
	if _, err := v.front.ReadVerified(v.VerifiedSession); err != nil {
		return err
	}
	seen := make(map[string]bool)
	for _, leg := range v.AllowedWorkStores {
		if seen[leg.dir] {
			continue
		}
		seen[leg.dir] = true
		store, err := openStoreAtForCity(leg.dir, v.cityPath)
		if err != nil {
			return err
		}
		if err := releaseVerifiedHookClaims(v, store); err != nil {
			return err
		}
	}
	if route != nil && route.class != nil {
		if err := releaseVerifiedHookClaims(v, route.class); err != nil {
			return err
		}
	}
	info, err := v.front.ReadVerified(v.VerifiedSession)
	if err != nil {
		return err
	}
	sp, err := newVerifiedHookProvider(v, info)
	if err != nil {
		return err
	}
	if err := acknowledgeVerifiedHookProvider(v, sp); err != nil {
		return err
	}
	_ = pokeController(v.cityPath)
	_ = stderr
	return nil
}

// releaseVerifiedHookClaims is deliberately separate from the controller's
// administrative release sweep: workers have neither name-resolution authority
// nor its unconditional fallback. Each release guards the whole observed row,
// including status, assignee and gc.session_id, in one revision CAS.
func releaseVerifiedHookClaims(v *VerifiedSession, store beads.Store) error {
	if _, err := v.front.ReadVerified(v.VerifiedSession); err != nil {
		return err
	}
	wa := workAssignmentForStore(beads.WorkStore{Store: store})
	for _, assignee := range v.identifiers() {
		items, err := wa.OpenAssignedTo(assignee, "in_progress", beads.TierBoth, true)
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.Metadata[beadmeta.SessionIDMetadataKey] != v.ID || !slices.Contains(v.identifiers(), item.Assignee) {
				continue
			}
			if _, err := v.front.ReadVerified(v.VerifiedSession); err != nil {
				return err
			}
			live, err := beads.HandlesFor(store).Live.Get(item.ID)
			if err != nil {
				return err
			}
			if live.ID != item.ID || live.Status != "in_progress" || live.Metadata[beadmeta.SessionIDMetadataKey] != v.ID || !slices.Contains(v.identifiers(), live.Assignee) {
				continue
			}
			writer, ok := beads.RequiredConditionalWriterFor(store)
			if !ok {
				return beads.ErrConditionalWriteUnsupported
			}
			// This is the last read before the work CAS; a stale incarnation
			// cannot use the controller's wider release authority.
			if _, err := v.front.ReadVerified(v.VerifiedSession); err != nil {
				return err
			}
			status, empty := "open", ""
			err = writer.UpdateIfMatch(live.ID, live.Revision, beads.UpdateOpts{
				Status: &status, Assignee: &empty,
				Metadata: map[string]string{beadmeta.SessionAffinityMetadataKey: "", beadmeta.ContinuationGroupMetadataKey: ""},
			})
			if beads.IsPreconditionFailed(err) {
				continue
			}
			if err != nil {
				return err
			}
			if live.ID == v.CurrentClaim {
				if err := v.front.CompareAndSetCurrentClaim(v.VerifiedSession, live.ID, ""); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func acknowledgeVerifiedHookProvider(v *VerifiedSession, sp runtime.Provider) error {
	if v.SessionName == "" {
		return fmt.Errorf("verified session has no provider target")
	}
	// Runtime names can be recycled independently of the bead. Verify the
	// provider occupant too; never borrow the current occupant by its name.
	for _, mutation := range []struct {
		key, value string
		remove     bool
	}{
		{reconcilerDrainAckReasonKey, "", true},
		{reconcilerDrainAckGenerationKey, "", true},
		{drainAckRequesterInstanceTokenKey, v.InstanceTokenSHA256, false},
		{reconcilerDrainAckSourceKey, drainAckSourceAgentValue, false},
		{"GC_DRAIN_ACK", "1", false},
	} {
		if _, err := v.front.ReadVerified(v.VerifiedSession); err != nil {
			return err
		}
		token, err := sp.GetMeta(v.SessionName, "GC_INSTANCE_TOKEN")
		if err != nil {
			return err
		}
		if drainAckInstanceTokenDigest(token) != v.InstanceTokenSHA256 {
			return session.ErrStaleSession
		}
		if mutation.remove {
			err = sp.RemoveMeta(v.SessionName, mutation.key)
		} else {
			err = sp.SetMeta(v.SessionName, mutation.key, mutation.value)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

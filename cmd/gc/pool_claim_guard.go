package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/session"
)

var errPoolSessionClaimConflict = errors.New("pool session claim prevents binding")

type poolClaimObservation struct {
	row           beads.Bead
	err           error
	held, checked bool
}

// poolClaimGuard is scoped to one build/tick. Selection memoizes evidence,
// while mutations always read anew and fence the whole patch by that revision.
// Only diagnostics are shared with async warm delivery; the memo is serial.
type poolClaimGuard struct {
	store     beads.Store
	cityPath  string
	cfg       *config.City
	rigStores map[string]beads.Store
	stderr    io.Writer
	observed  map[string]*poolClaimObservation
	mu        sync.Mutex
	reported  map[string]bool
}

func (bp *agentBuildParams) poolClaims() *poolClaimGuard {
	if bp.claimGuard == nil {
		bp.claimGuard = &poolClaimGuard{
			store: bp.beadStore, cityPath: bp.cityPath, cfg: bp.city,
			rigStores: bp.sessionCensusRigStores, stderr: bp.stderr,
		}
	}
	return bp.claimGuard
}

func (g *poolClaimGuard) report(id string, err error) {
	if err == nil || g.stderr == nil || beads.IsPreconditionFailed(err) {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.reported == nil {
		g.reported = make(map[string]bool)
	}
	kind := "claim check/write failed"
	if errors.Is(err, beads.ErrConditionalWriteUnsupported) {
		kind = "conditional writes unsupported; pool binding and idle markers disabled"
	} else if errors.Is(err, errPoolSessionClaimConflict) {
		kind = "held claim prevents binding"
	}
	key := id + ":" + kind
	if g.reported[key] {
		return
	}
	g.reported[key] = true
	fmt.Fprintf(g.stderr, "pool session %s %s (deferred): %v\n", id, kind, err) //nolint:errcheck
}

func (g *poolClaimGuard) check(id, target string, memo bool) (beads.Bead, bool, error) {
	if g.store == nil || id == "" {
		return beads.Bead{}, false, nil
	}
	var o *poolClaimObservation
	if memo {
		o = g.observed[id]
	}
	if o == nil {
		o = &poolClaimObservation{}
		o.row, o.err = liveBeadRead(g.store, id)
		if memo {
			if g.observed == nil {
				g.observed = make(map[string]*poolClaimObservation)
			}
			g.observed[id] = o
		}
	}
	claim := strings.TrimSpace(o.row.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey])
	if o.err == nil && (claim == "" || claim == strings.TrimSpace(target)) {
		return o.row, false, nil
	}
	if o.err == nil && !o.checked {
		// Use the identities and incarnation from the SAME row as the fence.
		info := session.ReconcileRowsFromBeads([]beads.Bead{o.row})[0].Info
		o.held, o.err = sessionStillHoldsClaim(g.cityPath, g.cfg, g.store, g.rigStores, info, claim)
		o.checked = true
	}
	g.report(id, o.err)
	return o.row, o.err != nil || o.held, o.err
}

func (g *poolClaimGuard) write(id, target string, metadata map[string]string) error {
	row, conflict, err := g.check(id, target, false)
	if conflict {
		if err != nil {
			return fmt.Errorf("%w: %w", errPoolSessionClaimConflict, err)
		}
		err = fmt.Errorf("%w: session %s target %q", errPoolSessionClaimConflict, id, target)
		g.report(id, err)
		return err
	}
	writer, ok := beads.RequiredConditionalWriterFor(g.store)
	if !ok {
		err = beads.ErrConditionalWriteUnsupported
	} else {
		// ga-gwav: a claim committed after check changes this revision. Defer
		// on contention; never retry just the write or use an unconditional path.
		err = writer.UpdateIfMatch(id, row.Revision, beads.UpdateOpts{Metadata: metadata})
	}
	if err != nil {
		g.report(id, err)
		return fmt.Errorf("%w: %w", errPoolSessionClaimConflict, err)
	}
	// ga-gwav: later selection in this tick must not reuse our pre-write row
	// or its held-work observation. Mutation checks themselves always read live.
	delete(g.observed, id)
	return nil
}

func poolSessionClaimAllowsRequest(bp *agentBuildParams, info session.Info, request SessionRequest) (bool, error) {
	if bp == nil {
		return true, nil
	}
	_, conflict, err := bp.poolClaims().check(info.ID, request.WorkBeadID, true)
	if err != nil {
		return false, fmt.Errorf("%w: %w", errPoolSessionClaimConflict, err)
	}
	return !conflict, nil
}

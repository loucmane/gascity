package nudgequeue

import (
	"fmt"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
)

// Scope bounds queue operations to one session generation. The zero value keeps
// the ordinary queue contract, including global maintenance, unchanged.
type Scope struct {
	sessionID string
	epoch     string
}

// NewScope refuses unknown modes and incomplete strict identities. A caller
// must not turn that error into an ordinary/global scope.
func NewScope(mode, sessionID, epoch string) (Scope, error) {
	switch mode {
	case "", "global":
		return Scope{}, nil
	case "session-epoch":
		if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(epoch) == "" || strings.TrimSpace(sessionID) != sessionID || strings.TrimSpace(epoch) != epoch {
			return Scope{}, fmt.Errorf("session-epoch nudge scope requires an exact session ID and continuation epoch")
		}
		return Scope{sessionID: sessionID, epoch: epoch}, nil
	default:
		return Scope{}, fmt.Errorf("unknown nudge queue scope %q", mode)
	}
}

// Strict reports whether foreign session generations are protected.
func (s Scope) Strict() bool { return s.sessionID != "" }

// Matches excludes unfenced and stale-epoch entries in strict mode.
func (s Scope) Matches(item Item) bool {
	return !s.Strict() || (item.SessionID == s.sessionID && item.ContinuationEpoch == s.epoch)
}

// WithdrawWaitNudges bounds the implicit cancellation performed during a managed
// wake. WakeSession may return old-epoch wait IDs; those queue tuples and shadows
// remain foreign even though the session's wait records have been canceled.
func (s Scope) WithdrawWaitNudges(store beads.Store, cityPath string, ids []string) error {
	if !s.Strict() {
		return WithdrawWaitNudges(store, cityPath, ids)
	}
	if len(ids) == 0 || cityPath == "" {
		return nil
	}
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id != "" {
			want[id] = true
		}
	}
	front := NewScopedStore(beads.NudgesStore{Store: store}, s)
	return s.WithState(cityPath, func(state *State) error {
		for _, bucket := range []*[]Item{&state.Pending, &state.InFlight} {
			retained := (*bucket)[:0]
			for _, item := range *bucket {
				if !want[item.ID] {
					retained = append(retained, item)
					continue
				}
				if err := front.Terminalize(item, "failed", "wait-canceled", "delivery-withdrawn", time.Now()); err != nil {
					return err
				}
			}
			*bucket = retained
		}
		return nil
	})
}

// WithState supplies only the selected generation to existing queue operations,
// under the same lock as the full state. Foreign bucket membership and every
// foreign item field survive unchanged. This is not a city-wide maintenance lock.
func (s Scope) WithState(cityPath string, fn func(*State) error) error {
	return s.withState(cityPath, nil, fn)
}

// Enqueue checks scope and cross-generation ID collisions before fn can write
// the backing bead. The caller performs Save inside fn, under the queue lock.
func (s Scope) Enqueue(cityPath string, item Item, fn func(*State) error) error {
	if !s.Matches(item) || (s.Strict() && item.ID == "") {
		return fmt.Errorf("nudge enqueue is outside the bound session scope")
	}
	return s.withState(cityPath, &item, fn)
}

func (s Scope) withState(cityPath string, enqueue *Item, fn func(*State) error) error {
	if !s.Strict() {
		return WithState(cityPath, fn)
	}
	return WithState(cityPath, func(full *State) error {
		var selected, foreign State
		buckets := []struct {
			input             []Item
			selected, foreign *[]Item
		}{
			{full.Pending, &selected.Pending, &foreign.Pending},
			{full.InFlight, &selected.InFlight, &foreign.InFlight},
			{full.Dead, &selected.Dead, &foreign.Dead},
		}
		foreignIDs := make(map[string]bool)
		for _, bucket := range buckets {
			for _, item := range bucket.input {
				if s.Matches(item) {
					*bucket.selected = append(*bucket.selected, item)
				} else {
					if enqueue != nil && item.ID == enqueue.ID {
						return fmt.Errorf("nudge ID %q belongs to a foreign session scope", item.ID)
					}
					foreignIDs[item.ID] = true
					*bucket.foreign = append(*bucket.foreign, item)
				}
			}
		}
		for _, bucket := range [][]Item{selected.Pending, selected.InFlight, selected.Dead} {
			for _, item := range bucket {
				if foreignIDs[item.ID] {
					return fmt.Errorf("nudge ID %q crosses session scopes", item.ID)
				}
			}
		}
		if err := fn(&selected); err != nil {
			return err
		}
		for _, bucket := range [][]Item{selected.Pending, selected.InFlight, selected.Dead} {
			for _, item := range bucket {
				if !s.Matches(item) || foreignIDs[item.ID] {
					return fmt.Errorf("nudge operation escaped session scope")
				}
			}
		}
		full.Pending = foreign.Pending
		full.Pending = append(full.Pending, selected.Pending...)
		full.InFlight = foreign.InFlight
		full.InFlight = append(full.InFlight, selected.InFlight...)
		full.Dead = foreign.Dead
		full.Dead = append(full.Dead, selected.Dead...)
		SortState(full)
		return nil
	})
}

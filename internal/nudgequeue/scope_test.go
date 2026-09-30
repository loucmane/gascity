package nudgequeue

import (
	"reflect"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
)

func TestScopeIdentityContract(t *testing.T) {
	for _, tc := range []struct {
		mode, id, epoch string
		valid, strict   bool
	}{
		{"", "", "", true, false},
		{"global", "", "", true, false},
		{"session-epoch", "session", "2", true, true},
		{"session-epoch", "", "2", false, false},
		{"session-epoch", "session", "", false, false},
		{"session-epoch", " session", "2", false, false},
		{"session-epoch", "session", "2 ", false, false},
		{"typo", "session", "2", false, false},
	} {
		t.Run(tc.mode+"/"+tc.id+"/"+tc.epoch, func(t *testing.T) {
			s, err := NewScope(tc.mode, tc.id, tc.epoch)
			if (err == nil) != tc.valid {
				t.Fatalf("NewScope: %v", err)
			}
			if !tc.valid {
				return
			}
			if s.Strict() != tc.strict {
				t.Fatalf("strict=%v", s.Strict())
			}
			for _, item := range []Item{{}, {SessionID: "other", ContinuationEpoch: "2"}, {SessionID: "session", ContinuationEpoch: "1"}} {
				if s.Matches(item) == tc.strict {
					t.Fatalf("foreign match=%v", s.Matches(item))
				}
			}
			if !s.Matches(Item{SessionID: "session", ContinuationEpoch: "2"}) {
				t.Fatal("owned identity refused")
			}
		})
	}
}

func TestScopedStoreRefusesForeignShadowBeforeWrite(t *testing.T) {
	for _, operation := range []string{"save", "terminalize", "fallback"} {
		t.Run(operation, func(t *testing.T) {
			store := beads.NewMemStore()
			ordinary := NewStore(beads.NudgesStore{Store: store})
			foreign := Item{ID: "collision", SessionID: "foreign", ContinuationEpoch: "1"}
			id, _, err := ordinary.Save(foreign)
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.Get(id)
			if err != nil {
				t.Fatal(err)
			}
			scope, err := NewScope("session-epoch", "current", "2")
			if err != nil {
				t.Fatal(err)
			}
			scoped := NewScopedStore(beads.NudgesStore{Store: store}, scope)
			owned := Item{ID: "collision", BeadID: id, SessionID: "current", ContinuationEpoch: "2"}
			if operation == "fallback" {
				owned.BeadID = ""
			}
			if operation == "save" {
				_, _, err = scoped.Save(owned)
			} else {
				err = scoped.Terminalize(owned, "injected", "", "provider-nudge-return", time.Now())
			}
			if err == nil {
				t.Fatal("foreign shadow accepted")
			}
			after, getErr := store.Get(id)
			if getErr != nil {
				t.Fatal(getErr)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("foreign shadow changed")
			}
		})
	}
}

func TestScopeRejectsCollisionBeforeCallbackAndEscapeBeforeCommit(t *testing.T) {
	for _, operation := range []string{"enqueue-collision", "duplicate-collision", "escape"} {
		t.Run(operation, func(t *testing.T) {
			dir := t.TempDir()
			foreign := Item{ID: "same-id", SessionID: "foreign", ContinuationEpoch: "1"}
			owned := Item{ID: "same-id", SessionID: "current", ContinuationEpoch: "2"}
			if err := WithState(dir, func(s *State) error {
				s.Dead = []Item{foreign}
				if operation == "duplicate-collision" {
					s.Pending = []Item{owned}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			before, err := LoadState(dir)
			if err != nil {
				t.Fatal(err)
			}
			scope, _ := NewScope("session-epoch", "current", "2")
			called := false
			fn := func(s *State) error { called = true; s.Pending = append(s.Pending, foreign); return nil }
			if operation == "enqueue-collision" {
				err = scope.Enqueue(dir, owned, fn)
			} else {
				err = scope.WithState(dir, fn)
			}
			if err == nil {
				t.Fatal("invalid transaction accepted")
			}
			if called != (operation == "escape") {
				t.Fatalf("callback=%v", called)
			}
			after, err := LoadState(dir)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("refused transaction changed queue")
			}
		})
	}
}

func TestScopedSaveChecksExactOpenShadowAfterOwnedTerminal(t *testing.T) {
	now := time.Now().UTC()
	foreign := beads.Bead{ID: "foreign-open", Status: "open", Type: "chore", CreatedAt: now.Add(-time.Hour), Labels: []string{"gc:nudge", "nudge:collision"}, Metadata: map[string]string{"nudge_id": "collision", "session_id": "foreign", "continuation_epoch": "1", "state": "queued"}}
	terminal := beads.Bead{ID: "owned-terminal", Status: "closed", Type: "chore", CreatedAt: now, Labels: []string{"gc:nudge", "nudge:collision"}, Metadata: map[string]string{"nudge_id": "collision", "session_id": "current", "continuation_epoch": "2", "state": "injected"}}
	store := beads.NewMemStoreFrom(2, []beads.Bead{foreign, terminal}, nil)
	scope, err := NewScope("session-epoch", "current", "2")
	if err != nil {
		t.Fatal(err)
	}
	if id, created, err := NewScopedStore(beads.NudgesStore{Store: store}, scope).Save(Item{ID: "collision", SessionID: "current", ContinuationEpoch: "2"}); err == nil || id != "" || created {
		t.Fatalf("cross-scope open shadow returned: id=%q created=%v err=%v", id, created, err)
	}
	for _, before := range []beads.Bead{foreign, terminal} {
		after, err := store.Get(before.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("shadow %s changed", before.ID)
		}
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
)

func TestPlanBdReadyOutcomeOverfetchesBeforeLimit(t *testing.T) {
	plan, child, err := planBdReadyOutcome([]string{"ready", "--label", "role:worker", "--limit", "1", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.enabled || !plan.json || plan.limit != 1 {
		t.Fatalf("plan = %+v, want enabled JSON limit 1", plan)
	}
	got := strings.Join(child, " ")
	if got != "ready --label role:worker --json --limit 0" {
		t.Fatalf("child args = %q, want all external filters plus unlimited JSON", got)
	}
}

func TestPlanBdReadyOutcomeFailsClosedForStatusOnlyClaim(t *testing.T) {
	_, _, err := planBdReadyOutcome([]string{"ready", "--claim", "--json"})
	if err == nil || !strings.Contains(err.Error(), "status-only dependency predicate") {
		t.Fatalf("plan error = %v, want explicit outcome-aware claim refusal", err)
	}
}

func TestOutcomeAwareReadyExplainMovesFailedDependentToBlocked(t *testing.T) {
	store := beads.NewMemStore()
	blocker, err := store.Create(beads.Bead{Title: "failed prerequisite", Type: "task"})
	if err != nil {
		t.Fatal(err)
	}
	dependent, err := store.Create(beads.Bead{Title: "dependent", Type: "task"})
	if err != nil {
		t.Fatal(err)
	}
	eligible, err := store.Create(beads.Bead{Title: "eligible", Type: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DepAdd(dependent.ID, blocker.ID, "blocks"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CloseAll([]string{blocker.ID}, map[string]string{"gc.outcome": "fail"}); err != nil {
		t.Fatal(err)
	}

	raw := []byte(`{
  "ready": [
    {"id":"` + dependent.ID + `","title":"dependent","status":"open","issue_type":"task","reason":"all blockers resolved","resolved_blockers":["` + blocker.ID + `"]},
    {"id":"` + eligible.ID + `","title":"eligible","status":"open","issue_type":"task","reason":"no blockers","resolved_blockers":[]}
  ],
  "blocked": [],
  "summary": {"total_ready":2,"total_blocked":0,"cycle_count":0}
}`)
	var stdout bytes.Buffer
	if err := emitOutcomeAwareBdReadyExplanation(bdReadyOutcomePlan{json: true, explain: true}, raw, store, &stdout); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Ready []struct {
			ID string `json:"id"`
		} `json:"ready"`
		Blocked []struct {
			ID        string `json:"id"`
			BlockedBy []struct {
				ID string `json:"id"`
			} `json:"blocked_by"`
		} `json:"blocked"`
		Summary map[string]int `json:"summary"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode explanation: %v; output=%s", err, stdout.String())
	}
	if len(got.Ready) != 1 || got.Ready[0].ID != eligible.ID {
		t.Fatalf("ready = %+v, want only %s", got.Ready, eligible.ID)
	}
	if len(got.Blocked) != 1 || got.Blocked[0].ID != dependent.ID || len(got.Blocked[0].BlockedBy) != 1 || got.Blocked[0].BlockedBy[0].ID != blocker.ID {
		t.Fatalf("blocked = %+v, want %s blocked by %s", got.Blocked, dependent.ID, blocker.ID)
	}
	if got.Summary["total_ready"] != 1 || got.Summary["total_blocked"] != 1 {
		t.Fatalf("summary = %+v, want 1 ready / 1 blocked", got.Summary)
	}
}

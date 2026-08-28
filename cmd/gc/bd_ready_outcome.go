package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
)

const bdReadyDefaultLimit = 100

type bdReadyOutcomePlan struct {
	enabled bool
	json    bool
	explain bool
	limit   int
	offset  int
}

// planBdReadyOutcome converts a public ready query into an unlimited JSON
// query against external bd. Gas City then applies its controller-authoritative
// failed-outcome dependency rule before offset/limit and presentation. Keeping
// bd in charge of label/type/priority/sort/molecule filters avoids a second,
// inevitably drifting CLI parser.
func planBdReadyOutcome(args []string) (bdReadyOutcomePlan, []string, error) {
	plan := bdReadyOutcomePlan{limit: bdReadyDefaultLimit}
	if len(args) == 0 || args[0] != "ready" {
		return plan, args, nil
	}
	for _, arg := range args[1:] {
		if arg == "--help" || arg == "-h" {
			return plan, args, nil
		}
	}
	plan.enabled = true
	child := make([]string, 0, len(args)+2)
	child = append(child, "ready")
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--claim":
			return bdReadyOutcomePlan{}, nil, fmt.Errorf("ready --claim cannot safely use external bd's status-only dependency predicate; use the managed claim surface instead")
		case arg == "--json":
			plan.json = true
			continue
		case arg == "--json=true":
			plan.json = true
			continue
		case arg == "--json=false":
			plan.json = false
			continue
		case arg == "--explain":
			plan.explain = true
			child = append(child, arg)
		case arg == "--plain" || strings.HasPrefix(arg, "--plain=") || arg == "--pretty" || strings.HasPrefix(arg, "--pretty="):
			// Presentation is rendered only after Gas City's outcome-aware
			// filtering, so external bd must always emit machine JSON here.
			continue
		case arg == "--limit" || arg == "-n":
			if i+1 >= len(args) {
				return bdReadyOutcomePlan{}, nil, fmt.Errorf("%s requires an integer value", arg)
			}
			value, err := parseBdReadyNonNegativeInt(arg, args[i+1])
			if err != nil {
				return bdReadyOutcomePlan{}, nil, err
			}
			plan.limit = value
			i++
		case strings.HasPrefix(arg, "--limit=") || strings.HasPrefix(arg, "-n="):
			name, raw, _ := strings.Cut(arg, "=")
			value, err := parseBdReadyNonNegativeInt(name, raw)
			if err != nil {
				return bdReadyOutcomePlan{}, nil, err
			}
			plan.limit = value
		case arg == "--offset":
			if i+1 >= len(args) {
				return bdReadyOutcomePlan{}, nil, fmt.Errorf("--offset requires an integer value")
			}
			value, err := parseBdReadyNonNegativeInt("--offset", args[i+1])
			if err != nil {
				return bdReadyOutcomePlan{}, nil, err
			}
			plan.offset = value
			i++
		case strings.HasPrefix(arg, "--offset="):
			_, raw, _ := strings.Cut(arg, "=")
			value, err := parseBdReadyNonNegativeInt("--offset", raw)
			if err != nil {
				return bdReadyOutcomePlan{}, nil, err
			}
			plan.offset = value
		default:
			child = append(child, arg)
		}
	}
	// --explain always returns the complete reasoning object. Ordinary ready
	// queries must be unlimited here so a failed row ahead of the caller's
	// requested limit cannot hide a later controller-ready row.
	child = append(child, "--json")
	if !plan.explain {
		child = append(child, "--limit", "0")
	}
	return plan, child, nil
}

func parseBdReadyNonNegativeInt(flag, raw string) (int, error) {
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s requires a non-negative integer, got %q", flag, raw)
	}
	return value, nil
}

type bdReadyOutcomeCandidate struct {
	raw      json.RawMessage
	bead     beads.Bead
	blockers []beads.Bead
}

func emitOutcomeAwareBdReady(plan bdReadyOutcomePlan, raw []byte, target execStoreTarget, cityPath string, cfg *config.City, stdout, stderr io.Writer) int {
	store, err := openOutcomeAwareBdReadyStore(target.ScopeRoot, cityPath, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "gc bd ready: opening authoritative readiness store: %v\n", err) //nolint:errcheck
		return 1
	}
	if plan.explain {
		err = emitOutcomeAwareBdReadyExplanation(plan, raw, store, stdout)
	} else {
		err = emitOutcomeAwareBdReadyList(plan, raw, store, stdout)
	}
	closeErr := closeBeadStoreHandle(store)
	if err != nil {
		fmt.Fprintf(stderr, "gc bd ready: %v\n", err) //nolint:errcheck
		return 1
	}
	if closeErr != nil {
		fmt.Fprintf(stderr, "gc bd ready: closing authoritative readiness store: %v\n", closeErr) //nolint:errcheck
		return 1
	}
	return 0
}

func openOutcomeAwareBdReadyStore(scopeRoot, cityPath string, cfg *config.City) (beads.Store, error) {
	if openBdReadyStoreForTest != nil {
		return openBdReadyStoreForTest(scopeRoot, cityPath, cfg)
	}
	return openStoreAtForCityWithConfig(scopeRoot, cityPath, cfg)
}

func classifyOutcomeAwareBdReady(rawRows []json.RawMessage, store beads.Store) ([]bdReadyOutcomeCandidate, error) {
	rows := make([]bdReadyOutcomeCandidate, 0, len(rawRows))
	for _, raw := range rawRows {
		var projected beads.Bead
		if err := json.Unmarshal(raw, &projected); err != nil {
			return nil, fmt.Errorf("decoding external ready row: %w", err)
		}
		if strings.TrimSpace(projected.ID) == "" {
			return nil, fmt.Errorf("external ready row has no bead id")
		}
		dependent, err := store.Get(projected.ID)
		if err != nil {
			return nil, fmt.Errorf("reading external ready candidate %s from authoritative store: %w", projected.ID, err)
		}
		blockers, err := beads.UnsatisfiedReadyBlockingDependencies(store, dependent)
		if err != nil {
			return nil, err
		}
		rows = append(rows, bdReadyOutcomeCandidate{raw: raw, bead: dependent, blockers: blockers})
	}
	return rows, nil
}

func emitOutcomeAwareBdReadyList(plan bdReadyOutcomePlan, raw []byte, store beads.Store, stdout io.Writer) error {
	var rawRows []json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(raw), &rawRows); err != nil {
		return fmt.Errorf("decoding external ready JSON: %w", err)
	}
	classified, err := classifyOutcomeAwareBdReady(rawRows, store)
	if err != nil {
		return err
	}
	ready := make([]bdReadyOutcomeCandidate, 0, len(classified))
	for _, row := range classified {
		if len(row.blockers) == 0 {
			ready = append(ready, row)
		}
	}
	total := len(ready)
	if plan.offset >= len(ready) {
		ready = nil
	} else if plan.offset > 0 {
		ready = ready[plan.offset:]
	}
	if plan.limit > 0 && len(ready) > plan.limit {
		ready = ready[:plan.limit]
	}
	if plan.json {
		out := make([]json.RawMessage, len(ready))
		for i := range ready {
			out[i] = ready[i].raw
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding outcome-aware ready JSON: %w", err)
		}
		_, err = fmt.Fprintln(stdout, string(data))
		return err
	}
	if len(ready) == 0 {
		_, err := fmt.Fprint(stdout, "\n○ No controller-ready work found\n\n")
		return err
	}
	if _, err := fmt.Fprintf(stdout, "\n📋 Controller-ready work (%d issues with no active or failed blockers):\n\n", len(ready)); err != nil {
		return err
	}
	for i, row := range ready {
		priority := "P?"
		if row.bead.Priority != nil {
			priority = fmt.Sprintf("P%d", *row.bead.Priority)
		}
		issueType := row.bead.Type
		if issueType == "" {
			issueType = "task"
		}
		if _, err := fmt.Fprintf(stdout, "%d. [%s] [%s] %s: %s\n", i+1, priority, issueType, row.bead.ID, row.bead.Title); err != nil {
			return err
		}
		if row.bead.Assignee != "" {
			if _, err := fmt.Fprintf(stdout, "   Assignee: %s\n", row.bead.Assignee); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if plan.limit > 0 && plan.offset+len(ready) < total {
		_, err := fmt.Fprintf(stdout, "Showing %d of %d controller-ready issues. Use --limit 0 for all.\n\n", len(ready), total)
		return err
	}
	return nil
}

type bdReadyExplanationWire struct {
	Ready   []json.RawMessage `json:"ready"`
	Blocked []json.RawMessage `json:"blocked"`
	Cycles  json.RawMessage   `json:"cycles,omitempty"`
	Summary map[string]int    `json:"summary"`
}

func emitOutcomeAwareBdReadyExplanation(plan bdReadyOutcomePlan, raw []byte, store beads.Store, stdout io.Writer) error {
	var explanation bdReadyExplanationWire
	if err := json.Unmarshal(bytes.TrimSpace(raw), &explanation); err != nil {
		return fmt.Errorf("decoding external ready explanation: %w", err)
	}
	classified, err := classifyOutcomeAwareBdReady(explanation.Ready, store)
	if err != nil {
		return err
	}
	ready := make([]json.RawMessage, 0, len(classified))
	blocked := append([]json.RawMessage(nil), explanation.Blocked...)
	for _, row := range classified {
		if len(row.blockers) == 0 {
			ready = append(ready, row.raw)
			continue
		}
		var moved map[string]any
		if err := json.Unmarshal(row.raw, &moved); err != nil {
			return fmt.Errorf("decoding external ready explanation row %s: %w", row.bead.ID, err)
		}
		delete(moved, "reason")
		delete(moved, "resolved_blockers")
		delete(moved, "dependency_count")
		delete(moved, "dependent_count")
		blockerRows := make([]map[string]any, 0, len(row.blockers))
		for _, blocker := range row.blockers {
			priority := 0
			if blocker.Priority != nil {
				priority = *blocker.Priority
			}
			blockerRows = append(blockerRows, map[string]any{
				"id": blocker.ID, "title": blocker.Title, "status": blocker.Status, "priority": priority,
			})
		}
		moved["blocked_by"] = blockerRows
		moved["blocked_by_count"] = len(blockerRows)
		encoded, err := json.Marshal(moved)
		if err != nil {
			return fmt.Errorf("encoding outcome-aware blocked explanation %s: %w", row.bead.ID, err)
		}
		blocked = append(blocked, encoded)
	}
	explanation.Ready = ready
	explanation.Blocked = blocked
	if explanation.Summary == nil {
		explanation.Summary = make(map[string]int)
	}
	explanation.Summary["total_ready"] = len(ready)
	explanation.Summary["total_blocked"] = len(blocked)
	if plan.json {
		data, err := json.MarshalIndent(explanation, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding outcome-aware ready explanation: %w", err)
		}
		_, err = fmt.Fprintln(stdout, string(data))
		return err
	}
	if _, err := fmt.Fprintf(stdout, "\nReady (%d) / blocked (%d) under Gas City outcome-aware semantics\n", len(ready), len(blocked)); err != nil {
		return err
	}
	for _, raw := range ready {
		var bead beads.Bead
		if err := json.Unmarshal(raw, &bead); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(stdout, "  READY   %s: %s\n", bead.ID, bead.Title); err != nil {
			return err
		}
	}
	for _, raw := range blocked {
		var item struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			BlockedBy []struct {
				ID string `json:"id"`
			} `json:"blocked_by"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return err
		}
		ids := make([]string, 0, len(item.BlockedBy))
		for _, blocker := range item.BlockedBy {
			ids = append(ids, blocker.ID)
		}
		if _, err := fmt.Fprintf(stdout, "  BLOCKED %s: %s (by %s)\n", item.ID, item.Title, strings.Join(ids, ", ")); err != nil {
			return err
		}
	}
	return nil
}

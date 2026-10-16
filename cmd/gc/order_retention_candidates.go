package main

import (
	"sort"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/orders"
)

// closedOrderTrackingRetentionCandidates returns exactly the expired suffix of
// each bucket after its recent-history floor. Count and both sweep variants use
// this selection, so the confirm gate and deletion cannot disagree on policy.
func closedOrderTrackingRetentionCandidates(store beads.Store, cutoff time.Time, retainLast int, onlyOrders map[string]struct{}) (map[string][]orders.OrderRun, error) {
	reader := orders.NewStore(beads.OrdersStore{Store: store})
	runs, err := reader.ClosedRunsForRetention(cutoff)
	if err != nil {
		return nil, err
	}
	byOrder := bucketClosedRetentionRuns(runs, onlyOrders)
	needed := make(map[string]int)
	for name, group := range byOrder {
		fresh, expired := 0, false
		for _, run := range group {
			if orderTrackingClosedReferenceTime(run).Before(cutoff) {
				expired = true
			} else {
				fresh++
			}
		}
		if expired && fresh < retainLast {
			needed[name] = retainLast - fresh
		}
	}

	// CreatedAt <= reference time: every omitted row (CreatedAt >= cutoff)
	// outranks every expired candidate. We only need up to retainLast such
	// witnesses per bucket, not their precise order or the entire recent corpus.
	seen := make(map[string]struct{})
	addWitness := func(run orders.OrderRun) {
		if run.CreatedAt.Before(cutoff) {
			return // already present in the candidate read
		}
		name, ok := orderTrackingRetentionBucket(run, onlyOrders)
		if !ok {
			if len(onlyOrders) > 0 {
				return
			}
			name = legacyOrderTrackingRetentionBucket
		}
		if needed[name] == 0 {
			return
		}
		if _, ok := seen[run.ID]; ok {
			return
		}
		seen[run.ID] = struct{}{}
		byOrder[name] = append(byOrder[name], run)
		needed[name]--
		if needed[name] == 0 {
			delete(needed, name)
		}
	}
	// Modern labeled orders usually finish with one bounded read per bucket.
	// These are witnesses only: labels may also occur on non-tracking roots or
	// disagree with a run's first order label, and title-only runs need fallback.
	for name := range needed {
		if name == legacyOrderTrackingRetentionBucket {
			continue
		}
		recent, err := reader.RecentClosedRunsForRetention(name, time.Time{}, retainLast)
		if err != nil {
			return nil, err
		}
		for _, run := range recent {
			addWitness(run)
		}
	}

	// Legacy names cannot be selected by an order-run label. Page the tracking
	// rows only until each remaining floor is proved or we reach the candidate
	// cutoff. Never issue an unbounded history read. A backing desc limit may
	// choose either ID order within a timestamp, so defer its entire boundary
	// timestamp to the next page. A page consisting only of ties grows until
	// that timestamp is complete; no tie member is skipped or counted twice.
	const pageSize = 256
	before, limit := time.Time{}, pageSize
	for len(needed) > 0 {
		page, err := reader.RecentClosedRunsForRetention("", before, limit)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		oldest := page[len(page)-1].CreatedAt
		complete := page
		done := len(page) < limit || oldest.Before(cutoff)
		if !done {
			end := len(page)
			for end > 0 && page[end-1].CreatedAt.Equal(oldest) {
				end--
			}
			if end == 0 {
				limit *= 2
				continue
			}
			complete = page[:end]
		}
		for _, run := range complete {
			addWitness(run)
		}
		if done {
			break
		}
		before, limit = complete[len(complete)-1].CreatedAt, pageSize
	}

	for name, group := range byOrder {
		sort.Slice(group, func(i, j int) bool {
			left, right := orderTrackingClosedReferenceTime(group[i]), orderTrackingClosedReferenceTime(group[j])
			if left.Equal(right) {
				return group[i].ID > group[j].ID
			}
			return left.After(right)
		})
		eligible := group[:0]
		for i, run := range group {
			if i >= retainLast && orderTrackingClosedReferenceTime(run).Before(cutoff) {
				eligible = append(eligible, run)
			}
		}
		byOrder[name] = eligible
	}
	return byOrder, nil
}

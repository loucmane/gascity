package main

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/orders"
)

type retentionReadRecorder struct {
	beads.Store
	queries    []beads.ListQuery
	failRead   int
	nativeTies bool
}

func (s *retentionReadRecorder) List(q beads.ListQuery) ([]beads.Bead, error) {
	s.queries = append(s.queries, q)
	if len(s.queries) == s.failRead {
		return nil, errors.New("retention read failed")
	}
	if !s.nativeTies || !q.AllowBackingCreatedLimit {
		return s.Store.List(q)
	}
	// Reproduce the native backing's ID-ascending boundary ties before its
	// LIMIT, then the adapter's canonical re-sort of the returned sample.
	unlimited := q
	unlimited.Limit = 0
	rows, err := s.Store.List(unlimited)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})
	if q.Limit > 0 && len(rows) > q.Limit {
		rows = rows[:q.Limit]
	}
	beads.SortBeads(rows, q.Sort)
	return rows, err
}

func TestRetentionCandidateReadErrorsFailClosed(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	var seed []beads.Bead
	for i := range 20 {
		seed = append(seed, beads.Bead{
			ID: fmt.Sprintf("old-%02d", i), Title: "order:a", Status: "closed",
			CreatedAt: now.Add(-8 * 24 * time.Hour), Labels: []string{labelOrderTracking, "order-run:a"},
		})
	}
	// Read 1 is the candidate query; 2 is the labeled witness query; 3 is the
	// fallback needed to discover title-only witnesses. None may fail open.
	for _, failRead := range []int{1, 2, 3} {
		for _, mode := range []string{"count", "sweep", "bounded"} {
			t.Run(fmt.Sprintf("read=%d/%s", failRead, mode), func(t *testing.T) {
				store := &retentionReadRecorder{Store: beads.NewMemStoreFrom(100, seed, nil), failRead: failRead}
				policy := orderTrackingRetentionPolicyForConfig(nil)
				var n int
				var err error
				switch mode {
				case "count":
					n, err = countClosedOrderTrackingRetentionEligible([]beads.Store{store}, now, policy, nil)
				case "sweep":
					n, err = sweepClosedOrderTrackingRetention(store, now, policy, nil)
				case "bounded":
					n, err = sweepClosedOrderTrackingRetentionBounded(store, now, policy, nil, 100)
				}
				if n != 0 || err == nil {
					t.Fatalf("result = %d, %v; want zero and error", n, err)
				}
				for _, row := range seed {
					if _, err := store.Get(row.ID); err != nil {
						t.Fatalf("deleted %s after read failure", row.ID)
					}
				}
			})
		}
	}
}

func TestRetentionReadsBoundHistory(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	var seed []beads.Bead
	for i := range 1200 {
		created := now.Add(-time.Hour)
		if i < 20 {
			created = now.Add(-8 * 24 * time.Hour)
		}
		seed = append(seed, beads.Bead{
			ID: fmt.Sprintf("run-%04d", i), Title: "order:busy", Status: "closed",
			CreatedAt: created, Labels: []string{labelOrderTracking, "order-run:busy"}, Ephemeral: true,
		})
	}
	policy := orderTrackingRetentionPolicyForConfig(nil)
	for _, mode := range []string{"count", "sweep", "bounded"} {
		t.Run(mode, func(t *testing.T) {
			store := &retentionReadRecorder{Store: beads.NewMemStoreFrom(2000, seed, nil)}
			var n int
			var err error
			switch mode {
			case "count":
				n, err = countClosedOrderTrackingRetentionEligible([]beads.Store{store}, now, policy, nil)
			case "sweep":
				n, err = sweepClosedOrderTrackingRetention(store, now, policy, nil)
			case "bounded":
				n, err = sweepClosedOrderTrackingRetentionBounded(store, now, policy, nil, 100)
			}
			if err != nil || n != 20 {
				t.Fatalf("result = %d, %v; want 20, nil", n, err)
			}
			for _, q := range store.queries {
				if q.Status != "closed" {
					continue // graph-aware deletion has its own reads
				}
				if q.CreatedBefore.IsZero() && q.Limit <= 0 {
					t.Errorf("unbounded closed-history read: %+v", q)
				}
				if q.Limit == 0 && !q.CreatedBefore.Equal(now.Add(-policy.deleteAfterClose)) {
					t.Errorf("candidate cutoff = %v, want retention cutoff", q.CreatedBefore)
				}
				if q.Limit > policy.retainLast {
					t.Errorf("busy labeled bucket needed more than retainLast rows: %+v", q)
				}
				v := reflect.ValueOf(q).FieldByName("SkipDependencies")
				if !v.IsValid() || !v.Bool() {
					t.Error("retention query must skip dependency hydration")
				}
			}
		})
	}
}

// The oracle deliberately retains the old full-scan algorithm independently of
// the new candidate selector, including the reference-time/ID ordering.
func fullScanRetentionIDs(seed []beads.Bead, now time.Time, policy orderTrackingRetentionPolicy, only map[string]struct{}) []string {
	var runs []orders.OrderRun
	for _, b := range beads.ApplyListQuery(seed, beads.ListQuery{Status: "closed", Label: labelOrderTracking, TierMode: beads.TierBoth}) {
		name, _ := orders.NameFromTrackingBead(b)
		runs = append(runs, orders.OrderRun{ID: b.ID, Scoped: name, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt})
	}
	var ids []string
	for _, group := range bucketClosedRetentionRuns(runs, only) {
		sort.Slice(group, func(i, j int) bool {
			a, b := orderTrackingClosedReferenceTime(group[i]), orderTrackingClosedReferenceTime(group[j])
			if a.Equal(b) {
				return group[i].ID > group[j].ID
			}
			return a.After(b)
		})
		for i, run := range group {
			if i >= max(policy.retainLast, minClosedOrderTrackingRetained) && orderTrackingClosedReferenceTime(run).Before(now.Add(-policy.deleteAfterClose)) {
				ids = append(ids, run.ID)
			}
		}
	}
	slices.Sort(ids)
	return ids
}

func TestRetentionCandidatesMatchFullScan(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-7 * 24 * time.Hour)
	var seed []beads.Bead
	for orderIndex, name := range []string{"busy", "title-only", "", "small", "old-only", "updated"} {
		n := 35
		if name == "small" {
			n = 4
		}
		for i := range n {
			created := cutoff.Add(-time.Duration(1+i%5) * time.Hour)
			if i%3 == 0 && name != "old-only" && name != "updated" {
				created = cutoff.Add(time.Duration(i%2) * time.Hour) // includes exact cutoff and ties
			}
			b := beads.Bead{
				ID: fmt.Sprintf("run-%d-%02d", orderIndex, i), Title: "unresolved", Status: "closed",
				CreatedAt: created, Labels: []string{labelOrderTracking}, Ephemeral: i%2 == 0,
			}
			if name != "" {
				b.Title = "order:" + name
			}
			if name != "" && name != "title-only" {
				b.Labels = append(b.Labels, "order-run:"+name)
			}
			if i%4 == 0 || name == "updated" && i < 9 {
				b.UpdatedAt = now.Add(-time.Hour) // old creation, recent reference
			} else if i%2 == 0 {
				b.UpdatedAt = created.Add(time.Minute)
			}
			seed = append(seed, b)
		}
	}
	// A large same-timestamp burst from another order forces the legacy fallback
	// through a page seam; it must not skip the other buckets tied at that seam.
	for i := range 600 {
		seed = append(seed, beads.Bead{
			ID: fmt.Sprintf("noise-%04d", i), Title: "order:noise", Status: "closed",
			CreatedAt: cutoff, Labels: []string{labelOrderTracking, "order-run:noise"},
		})
	}
	seed = append(seed,
		beads.Bead{ID: "zero-time", Title: "order:old-only", Status: "closed", Labels: []string{labelOrderTracking}},
		beads.Bead{ID: "foreign-root", Title: "root", Status: "closed", CreatedAt: now, Labels: []string{"order-run:busy"}},
		beads.Bead{ID: "multiple-labels", Title: "order:busy", Status: "closed", CreatedAt: now, Labels: []string{labelOrderTracking, "order-run:elsewhere", "order-run:busy"}},
	)
	for _, only := range []map[string]struct{}{nil, {"title-only": {}, "busy": {}}, {"missing": {}}} {
		for _, floor := range []int{0, 10, 17, 50} {
			policy := orderTrackingRetentionPolicy{deleteAfterClose: 7 * 24 * time.Hour, retainLast: floor}
			want := fullScanRetentionIDs(seed, now, policy, only)
			for _, bounded := range []bool{false, true} {
				t.Run(fmt.Sprintf("filter=%v/floor=%d/bounded=%v", only, floor, bounded), func(t *testing.T) {
					store := &retentionReadRecorder{Store: beads.NewMemStoreFrom(10000, seed, nil), nativeTies: true}
					n, err := countClosedOrderTrackingRetentionEligible([]beads.Store{store}, now, policy, only)
					if err != nil || n != len(want) {
						t.Fatalf("count = %d, %v; want %d", n, err, len(want))
					}
					if bounded {
						n, err = sweepClosedOrderTrackingRetentionBounded(store, now, policy, only, len(seed))
					} else {
						n, err = sweepClosedOrderTrackingRetention(store, now, policy, only)
					}
					if err != nil || n != len(want) {
						t.Fatalf("deleted = %d, %v; want %d", n, err, len(want))
					}
					var got []string
					for _, b := range seed {
						if _, err := store.Get(b.ID); err != nil {
							got = append(got, b.ID)
						}
					}
					slices.Sort(got)
					if !slices.Equal(got, want) {
						t.Fatalf("deleted IDs = %v, want %v", got, want)
					}
					for _, q := range store.queries {
						if q.Status == "closed" && (!q.SkipDependencies || q.CreatedBefore.IsZero() && q.Limit <= 0) {
							t.Fatalf("unbounded or dependency-hydrating retention query: %+v", q)
						}
					}
				})
			}
		}
	}
}

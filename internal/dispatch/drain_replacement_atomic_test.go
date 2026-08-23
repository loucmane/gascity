package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
)

var errInjectedDrainReplacementWrite = errors.New("injected drain replacement write failure")

// stagedAtomicDrainStore gives the unit tests the same commit-or-rollback
// contract RetryFailedDrainItem requires from production NativeDoltStore. Each
// callback mutates an isolated MemStore snapshot and publishes it only after a
// nil return. failAt injects an error immediately before the numbered write.
type stagedAtomicDrainStore struct {
	*beads.MemStore
	txMu      sync.Mutex
	failAt    int
	lastOps   int
	txArrived chan struct{}
	txRelease <-chan struct{}
}

func (*stagedAtomicDrainStore) AtomicTx() bool { return true }

func (s *stagedAtomicDrainStore) Tx(_ string, fn func(beads.Tx) error) error {
	if s.txArrived != nil {
		s.txArrived <- struct{}{}
		<-s.txRelease
	}
	s.txMu.Lock()
	defer s.txMu.Unlock()

	staged, err := cloneDrainMemStore(s.MemStore)
	if err != nil {
		return err
	}
	tx := &failingDrainReplacementTx{GraphTx: staged, failAt: s.failAt}
	err = fn(tx)
	s.lastOps = tx.ops
	if err != nil {
		return err
	}
	s.MemStore = staged
	return nil
}

type failingDrainReplacementTx struct {
	beads.GraphTx
	failAt int
	ops    int
}

func (tx *failingDrainReplacementTx) beforeWrite() error {
	tx.ops++
	if tx.failAt == tx.ops {
		return fmt.Errorf("%w at write %d", errInjectedDrainReplacementWrite, tx.ops)
	}
	return nil
}

func (tx *failingDrainReplacementTx) Create(bead beads.Bead) (beads.Bead, error) {
	if err := tx.beforeWrite(); err != nil {
		return beads.Bead{}, err
	}
	return tx.GraphTx.Create(bead)
}

func (tx *failingDrainReplacementTx) Update(id string, opts beads.UpdateOpts) error {
	if err := tx.beforeWrite(); err != nil {
		return err
	}
	return tx.GraphTx.Update(id, opts)
}

func (tx *failingDrainReplacementTx) SetMetadataBatch(id string, kvs map[string]string) error {
	if err := tx.beforeWrite(); err != nil {
		return err
	}
	return tx.GraphTx.SetMetadataBatch(id, kvs)
}

func (tx *failingDrainReplacementTx) Close(id string) error {
	if err := tx.beforeWrite(); err != nil {
		return err
	}
	return tx.GraphTx.Close(id)
}

func (tx *failingDrainReplacementTx) DepAdd(issueID, dependsOnID, depType string) error {
	if err := tx.beforeWrite(); err != nil {
		return err
	}
	return tx.GraphTx.DepAdd(issueID, dependsOnID, depType)
}

type failedDrainReplacementFixture struct {
	store      *stagedAtomicDrainStore
	drain      beads.Bead
	member     beads.Bead
	oldRoot    beads.Bead
	dependent  beads.Bead
	formulaDir string
}

func newFailedDrainReplacementFixture(t *testing.T, failAt int) failedDrainReplacementFixture {
	t.Helper()
	formulaDir := t.TempDir()
	writeRetryableDrainItemFormula(t, formulaDir)
	mem, drain := seedDrainWorkflow(t)
	if _, err := ProcessControl(mem, drain, ProcessOptions{FormulaSearchPaths: []string{formulaDir}}); err != nil {
		t.Fatalf("ProcessControl(drain expand): %v", err)
	}
	drain = mustGetBead(t, mem, drain.ID)
	row := mustDrainManifest(t, drain).Rows[0]
	member := mustGetBead(t, mem, row.MemberID)
	priority := 1
	if err := mem.Update(member.ID, beads.UpdateOpts{Priority: &priority}); err != nil {
		t.Fatalf("set source priority: %v", err)
	}
	if err := mem.SetMetadataBatch(member.ID, map[string]string{
		"gc.base_commit":   "a83fa64caa0778f3b0212a0fc7728d1b2a43432e",
		"gc.worktree_path": filepath.Join(t.TempDir(), "policy-worktrees", member.ID),
	}); err != nil {
		t.Fatalf("stamp source inputs: %v", err)
	}
	member = mustGetBead(t, mem, member.ID)
	oldRoot := mustGetBead(t, mem, row.ItemRootID)
	prepare := mustFindDrainItemStepByRef(t, mem, oldRoot.ID, "prepare")
	if err := updateMetadataAndClose(mem, prepare.ID, map[string]string{
		beadmeta.OutcomeMetadataKey:       beadmeta.OutcomeFail,
		beadmeta.FailureClassMetadataKey:  beadmeta.FailureClassHard,
		beadmeta.FailureReasonMetadataKey: "validator_missing",
	}); err != nil {
		t.Fatalf("close prepare failure: %v", err)
	}
	work := mustFindDrainItemStepByRef(t, mem, oldRoot.ID, "work")
	staleOwner := "ci-dead"
	inProgress := "in_progress"
	if err := mem.Update(work.ID, beads.UpdateOpts{Assignee: &staleOwner, Status: &inProgress}); err != nil {
		t.Fatalf("assign stale owner: %v", err)
	}
	dependent, err := mem.Create(beads.Bead{Title: "downstream reservation", Type: "task"})
	if err != nil {
		t.Fatalf("create downstream dependent: %v", err)
	}
	if err := mem.DepAdd(dependent.ID, oldRoot.ID, "blocks"); err != nil {
		t.Fatalf("add downstream dependency: %v", err)
	}
	return failedDrainReplacementFixture{
		store:      &stagedAtomicDrainStore{MemStore: mem, failAt: failAt},
		drain:      drain,
		member:     member,
		oldRoot:    oldRoot,
		dependent:  dependent,
		formulaDir: formulaDir,
	}
}

func (f failedDrainReplacementFixture) retry(opts ProcessOptions) (DrainItemReplacementResult, error) {
	opts.FormulaSearchPaths = []string{f.formulaDir}
	return RetryFailedDrainItem(context.Background(), f.store, f.drain.ID, f.member.ID, opts)
}

func TestRetryFailedDrainItemRollsBackEveryWriteBoundary(t *testing.T) {
	probe := newFailedDrainReplacementFixture(t, 0)
	if _, err := probe.retry(ProcessOptions{}); err != nil {
		t.Fatalf("successful probe: %v", err)
	}
	writeCount := probe.store.lastOps
	if writeCount < 10 {
		t.Fatalf("successful transaction recorded %d writes, want a substantive multi-boundary swap", writeCount)
	}

	for failAt := 1; failAt <= writeCount; failAt++ {
		t.Run(strconv.Itoa(failAt), func(t *testing.T) {
			fixture := newFailedDrainReplacementFixture(t, failAt)
			before := snapshotDrainStore(t, fixture.store)
			_, err := fixture.retry(ProcessOptions{})
			if !errors.Is(err, errInjectedDrainReplacementWrite) {
				t.Fatalf("RetryFailedDrainItem error = %v, want injected failure", err)
			}
			after := snapshotDrainStore(t, fixture.store)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("write %d escaped rollback\nbefore=%#v\nafter=%#v", failAt, before, after)
			}
		})
	}
}

func TestRetryFailedDrainItemPreservesDependenciesPriorityAndClearsOwnership(t *testing.T) {
	fixture := newFailedDrainReplacementFixture(t, 0)
	result, err := fixture.retry(ProcessOptions{})
	if err != nil {
		t.Fatalf("RetryFailedDrainItem: %v", err)
	}
	replacement := mustGetBead(t, fixture.store, result.NewRootID)
	if !reflect.DeepEqual(replacement.Priority, fixture.member.Priority) {
		t.Fatalf("replacement priority = %v, want source priority %v", replacement.Priority, fixture.member.Priority)
	}
	assertHasBlockingDep(t, fixture.store, fixture.dependent.ID, result.NewRootID)
	for _, old := range mustListDrainWorkflow(t, fixture.store, fixture.oldRoot.ID) {
		if old.Assignee != "" {
			t.Errorf("old workflow bead %s retained stale assignee %q", old.ID, old.Assignee)
		}
	}
}

func TestRetryFailedDrainItemRefusesInvalidSuccessfulAndAmbiguousStateWithoutMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, failedDrainReplacementFixture)
	}{
		{
			name: "invalid unpinned formula source",
			mutate: func(t *testing.T, f failedDrainReplacementFixture) {
				t.Helper()
				if err := f.store.SetMetadata(f.oldRoot.ID, beadmeta.FormulaSourceMetadataKey, "relative.formula.toml"); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "already successful",
			mutate: func(t *testing.T, f failedDrainReplacementFixture) {
				t.Helper()
				if err := f.store.SetMetadata(f.oldRoot.ID, beadmeta.OutcomeMetadataKey, beadmeta.OutcomePass); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "ambiguous manifest rows",
			mutate: func(t *testing.T, f failedDrainReplacementFixture) {
				t.Helper()
				control := mustGetBead(t, f.store, f.drain.ID)
				manifest := mustDrainManifest(t, control)
				manifest.Rows = append(manifest.Rows, manifest.Rows[0])
				raw, err := json.Marshal(manifest)
				if err != nil {
					t.Fatal(err)
				}
				if err := f.store.SetMetadata(control.ID, drainManifestMetadataKey, string(raw)); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newFailedDrainReplacementFixture(t, 0)
			tt.mutate(t, fixture)
			before := snapshotDrainStore(t, fixture.store)
			var traces []string
			_, err := fixture.retry(ProcessOptions{Tracef: func(format string, args ...any) {
				traces = append(traces, fmt.Sprintf(format, args...))
			}})
			if !errors.Is(err, ErrDrainItemRetryRefused) {
				t.Fatalf("error = %v, want ErrDrainItemRetryRefused", err)
			}
			if after := snapshotDrainStore(t, fixture.store); !reflect.DeepEqual(after, before) {
				t.Fatalf("refusal mutated state\nbefore=%#v\nafter=%#v", before, after)
			}
			if len(traces) != 1 || !strings.Contains(traces[0], "drain-item-retry attention") {
				t.Fatalf("traces = %q, want one attention event", traces)
			}
		})
	}
}

func TestRetryFailedDrainItemConcurrentCallsCreateExactlyOneReplacement(t *testing.T) {
	fixture := newFailedDrainReplacementFixture(t, 0)
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	fixture.store.txArrived = arrived
	fixture.store.txRelease = release

	type response struct {
		result DrainItemReplacementResult
		err    error
	}
	responses := make(chan response, 2)
	var traceMu sync.Mutex
	var traces []string
	opts := ProcessOptions{Tracef: func(format string, args ...any) {
		traceMu.Lock()
		defer traceMu.Unlock()
		traces = append(traces, fmt.Sprintf(format, args...))
	}}
	for range 2 {
		go func() {
			result, err := fixture.retry(opts)
			responses <- response{result: result, err: err}
		}()
	}
	<-arrived
	<-arrived
	close(release)

	var successes, refusals int
	var replacementID string
	for range 2 {
		response := <-responses
		switch {
		case response.err == nil:
			successes++
			if response.result.NewRootID != "" {
				replacementID = response.result.NewRootID
			}
		case errors.Is(response.err, ErrDrainItemRetryRefused):
			refusals++
		default:
			t.Fatalf("concurrent retry error = %v", response.err)
		}
	}
	if successes != 1 || refusals != 1 {
		t.Fatalf("concurrent outcomes success=%d refusal=%d, want 1/1", successes, refusals)
	}
	row := mustDrainManifest(t, mustGetBead(t, fixture.store, fixture.drain.ID)).Rows[0]
	if replacementID == "" || row.ItemRootID != replacementID {
		t.Fatalf("manifest replacement = %q, successful result = %q", row.ItemRootID, replacementID)
	}
	roots, err := fixture.store.ListByMetadata(map[string]string{beadmeta.ItemRootKeyMetadataKey: row.ItemRootKey}, 0, beads.IncludeClosed)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 2 {
		t.Fatalf("stable-key roots = %d, want old evidence + exactly one replacement", len(roots))
	}
	traceMu.Lock()
	defer traceMu.Unlock()
	if len(traces) != 1 || !strings.Contains(traces[0], "changed concurrently") {
		t.Fatalf("concurrent traces = %q, want one operator-visible refusal", traces)
	}
}

type drainStoreSnapshot struct {
	Beads []beads.Bead
	Deps  []beads.Dep
}

func snapshotDrainStore(t *testing.T, store beads.Store) drainStoreSnapshot {
	t.Helper()
	items, err := store.List(beads.ListQuery{AllowScan: true, IncludeClosed: true})
	if err != nil {
		t.Fatalf("snapshot beads: %v", err)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	var deps []beads.Dep
	for _, item := range items {
		outgoing, err := store.DepList(item.ID, "down")
		if err != nil {
			t.Fatalf("snapshot dependencies for %s: %v", item.ID, err)
		}
		deps = append(deps, outgoing...)
	}
	sort.Slice(deps, func(i, j int) bool {
		if deps[i].IssueID != deps[j].IssueID {
			return deps[i].IssueID < deps[j].IssueID
		}
		if deps[i].DependsOnID != deps[j].DependsOnID {
			return deps[i].DependsOnID < deps[j].DependsOnID
		}
		return deps[i].Type < deps[j].Type
	})
	return drainStoreSnapshot{Beads: items, Deps: deps}
}

func cloneDrainMemStore(store *beads.MemStore) (*beads.MemStore, error) {
	snapshot := drainStoreSnapshot{}
	items, err := store.List(beads.ListQuery{AllowScan: true, IncludeClosed: true})
	if err != nil {
		return nil, err
	}
	snapshot.Beads = items
	maxSeq := 0
	for _, item := range items {
		if n, err := strconv.Atoi(strings.TrimPrefix(item.ID, "gc-")); err == nil && n > maxSeq {
			maxSeq = n
		}
		deps, err := store.DepList(item.ID, "down")
		if err != nil {
			return nil, err
		}
		snapshot.Deps = append(snapshot.Deps, deps...)
	}
	return beads.NewMemStoreFrom(maxSeq, snapshot.Beads, snapshot.Deps), nil
}

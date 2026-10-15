package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
)

func verifiedWorkFixture(t *testing.T, v *VerifiedSession) (*beads.MemStore, beads.Bead) {
	t.Helper()
	b := beads.Bead{
		ID: "work", Type: "task", Status: "in_progress", Assignee: v.assignee(), Revision: 1,
		Metadata: map[string]string{
			beadmeta.SessionIDMetadataKey: v.ID, beadmeta.SessionNameMetadataKey: v.SessionName,
			beadmeta.ClaimedAtMetadataKey: "2026-10-13T00:00:00Z",
		},
	}
	mem := beads.NewMemStoreFrom(1, []beads.Bead{b}, nil)
	installVerifiedWorkStore(t, v, mem)
	return mem, b
}

func TestVerifiedHookPendingUnwindRejectsSameIncarnationABA(t *testing.T) {
	v, _ := newVerifiedHookFixture(t)
	mem, bead := verifiedWorkFixture(t, v)
	opts := hookClaimOptions{Verified: v, Assignee: v.assignee()}
	ops := hookClaimOps{}
	first, err := fenceVerifiedHookWork(bead, opts, ops, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := fenceVerifiedHookWork(first, opts, ops, "")
	if err != nil {
		t.Fatal(err)
	}
	if first.Metadata[verifiedHookAttemptKey] == second.Metadata[verifiedHookAttemptKey] {
		t.Fatal("adoption reused the old claim attempt")
	}
	if released, err := releaseVerifiedHookPending(first, opts, ops, ""); err != nil || released {
		t.Fatalf("old unwind released a later adoption: %v %v", released, err)
	}
	live, _ := mem.Get(bead.ID)
	if live.Status != "in_progress" || live.Revision != 3 {
		t.Fatal("old unwind mutated adopted work")
	}
	if released, err := releaseVerifiedHookPending(second, opts, ops, ""); err != nil || !released {
		t.Fatalf("current attempt did not release: %v %v", released, err)
	}
	live, _ = mem.Get(bead.ID)
	if live.Status != "open" || live.Assignee != "" || live.Metadata[verifiedHookIncarnationKey] != "" || live.Metadata[verifiedHookAttemptKey] != "" {
		t.Fatal("release retained claim authority")
	}
}

func TestVerifiedHookPendingUnwindLosesCASAgainstAdoption(t *testing.T) {
	v, _ := newVerifiedHookFixture(t)
	mem, bead := verifiedWorkFixture(t, v)
	opts := hookClaimOptions{Verified: v, Assignee: v.assignee()}
	first, err := fenceVerifiedHookWork(bead, opts, hookClaimOps{}, "")
	if err != nil {
		t.Fatal(err)
	}
	race := &releaseWorkCASRace{MemStore: mem, beforeWrite: func() {
		// Same assignee and incarnation: only the attempt distinguishes it.
		if err := mem.SetMetadata(bead.ID, verifiedHookAttemptKey, "later-claim"); err != nil {
			t.Fatal(err)
		}
	}}
	installVerifiedWorkStore(t, v, race)
	if released, err := releaseVerifiedHookPending(first, opts, hookClaimOps{}, ""); err != nil || released {
		t.Fatalf("racing unwind: released=%v err=%v", released, err)
	}
	live, _ := mem.Get(bead.ID)
	if live.Status != "in_progress" || live.Metadata[verifiedHookAttemptKey] != "later-claim" {
		t.Fatal("racing unwind overwrote adoption")
	}
}

func TestVerifiedHookFenceCannotOverwriteSuccessorMetadata(t *testing.T) {
	v, own := newVerifiedHookFixture(t)
	mem, bead := verifiedWorkFixture(t, v)
	race := &releaseWorkCASRace{MemStore: mem, beforeWrite: func() {
		if err := own.mem.Update(v.ID, beads.UpdateOpts{Metadata: map[string]string{"generation": "2", "instance_token": "successor"}}); err != nil {
			t.Fatal(err)
		}
		next, err := v.front.VerifySession(v.ID, "successor", 2)
		if err != nil {
			t.Fatal(err)
		}
		successor := &VerifiedSession{VerifiedSession: next, front: v.front}
		installVerifiedWorkStore(t, successor, mem)
		if _, err := fenceVerifiedHookWork(bead, hookClaimOptions{Verified: successor}, hookClaimOps{}, ""); err != nil {
			t.Fatal(err)
		}
	}}
	installVerifiedWorkStore(t, v, race)
	if _, err := fenceVerifiedHookWork(bead, hookClaimOptions{Verified: v}, hookClaimOps{}, ""); !beads.IsPreconditionFailed(err) {
		t.Fatalf("old fence overwrote successor: %v", err)
	}
	live, _ := mem.Get(bead.ID)
	if live.Metadata[verifiedHookIncarnationKey] == v.workIncarnation() || live.Revision != 2 {
		t.Fatal("old metadata reached successor work")
	}
}

func TestVerifiedHookLegacyOrForeignMarkersCannotRelease(t *testing.T) {
	for _, marker := range []string{"", "2:other-incarnation"} {
		t.Run(marker, func(t *testing.T) {
			v, _ := newVerifiedHookFixture(t)
			mem, bead := verifiedWorkFixture(t, v)
			if err := mem.Update(bead.ID, beads.UpdateOpts{Metadata: map[string]string{verifiedHookIncarnationKey: marker, verifiedHookAttemptKey: "attempt"}}); err != nil {
				t.Fatal(err)
			}
			bead, _ = mem.Get(bead.ID)
			if err := releaseVerifiedHookClaims(v, mem); err != nil {
				t.Fatal(err)
			}
			if released, err := releaseVerifiedHookPending(bead, hookClaimOptions{Verified: v}, hookClaimOps{}, ""); err != nil || released {
				t.Fatalf("unsafe unwind %v %v", released, err)
			}
			live, _ := mem.Get(bead.ID)
			if live.Revision != bead.Revision || live.Status != "in_progress" {
				t.Fatal("unproven work released")
			}
		})
	}
}

func TestVerifiedHookWorkFenceRequiresAtomicBackendAndCanonicalOwner(t *testing.T) {
	for _, kind := range []string{"unsupported", "foreign", "released"} {
		t.Run(kind, func(t *testing.T) {
			v, own := newVerifiedHookFixture(t)
			mem, bead := verifiedWorkFixture(t, v)
			switch kind {
			case "unsupported":
				installVerifiedWorkStore(t, v, releaseNoCASStore{mem})
			case "foreign":
				assignee := "other"
				if err := mem.Update(bead.ID, beads.UpdateOpts{Assignee: &assignee}); err != nil {
					t.Fatal(err)
				}
			case "released":
				status := "open"
				if err := mem.Update(bead.ID, beads.UpdateOpts{Status: &status}); err != nil {
					t.Fatal(err)
				}
			}
			_, err := fenceVerifiedHookWork(bead, hookClaimOptions{Verified: v}, hookClaimOps{}, "")
			if err == nil || own.writes != 0 {
				t.Fatalf("unfenced work admitted: %v", err)
			}
			if kind == "unsupported" && !errors.Is(err, beads.ErrConditionalWriteUnsupported) {
				t.Fatal(err)
			}
			if code := writeHookClaimWorkResultForBead(hookClaimJSONResult{Action: "work"}, bead, hookClaimOptions{Verified: v}, hookClaimOps{}, "", true, io.Discard, io.Discard); code != 1 {
				t.Fatal("fence failure delivered work")
			}
		})
	}
}

func TestVerifiedHookWorkFenceUsesConfiguredFileStoreAndClassResidence(t *testing.T) {
	clearGCEnv(t)
	t.Setenv("GC_BEADS", "file")
	v, _ := newVerifiedHookFixture(t)
	v.cityPath = t.TempDir()
	if err := ensureScopedFileStoreLayout(v.cityPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v.cityPath, "city.toml"), []byte("[workspace]\nname = \"test\"\n[beads]\nprovider = \"file\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(v.cityPath, "own-rig")
	if err := ensurePersistedScopeLocalFileStore(dir); err != nil {
		t.Fatal(err)
	}
	v.AllowedWorkStores = []hookStore{{dir: dir}}
	store, err := openVerifiedHookWorkStore(context.Background(), nil, v, nil, dir, "work")
	if err != nil {
		t.Fatal(err)
	}
	bead, err := store.Create(beads.Bead{Title: "work", Type: "task", Assignee: v.assignee()})
	if err != nil {
		t.Fatal(err)
	}
	status := "in_progress"
	if err := store.Update(bead.ID, beads.UpdateOpts{Status: &status}); err != nil {
		t.Fatal(err)
	}
	first, err := fenceVerifiedHookWork(bead, hookClaimOptions{Verified: v}, hookClaimOps{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	// Read the persisted result through a fresh observer. FileStore's own
	// conformance owns refresh of already-held readers across atomic rewrites.
	readWork := func() beads.Bead {
		t.Helper()
		observer, err := openVerifiedHookWorkStore(context.Background(), nil, v, nil, dir, bead.ID)
		if err != nil {
			t.Fatal(err)
		}
		row, err := observer.Get(bead.ID)
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	before := readWork()
	second, err := fenceVerifiedHookWork(bead, hookClaimOptions{Verified: v}, hookClaimOps{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	after := readWork()
	if after.Revision <= before.Revision || first.Metadata[verifiedHookAttemptKey] == second.Metadata[verifiedHookAttemptKey] {
		t.Fatalf("file-store adoption did not advance fence: revision %d -> %d, attempt %s -> %s", before.Revision, after.Revision, first.Metadata[verifiedHookAttemptKey], second.Metadata[verifiedHookAttemptKey])
	}
	if _, err := openVerifiedHookWorkStore(context.Background(), nil, v, nil, filepath.Join(v.cityPath, "foreign"), bead.ID); err == nil {
		t.Fatal("foreign work store admitted")
	}
	// A proven relocated class copy stays on that ledger even if a work copy
	// exists. No fresh routing decision is made by the compensation path.
	class := beads.NewMemStoreFrom(1, []beads.Bead{after}, nil)
	route := &hookClaimClassRoute{class: class, resident: map[string]bool{bead.ID: true}}
	ops := hookClaimOps{ClassRoute: route}
	adopted, err := fenceVerifiedHookWork(bead, hookClaimOptions{Verified: v}, ops, dir)
	if err != nil {
		t.Fatal(err)
	}
	if released, err := releaseVerifiedHookPending(adopted, hookClaimOptions{Verified: v}, ops, dir); err != nil || !released {
		t.Fatalf("class release %v %v", released, err)
	}
	work, _ := store.Get(bead.ID)
	if work.Revision != after.Revision || work.Status != "in_progress" {
		t.Fatal("class compensation changed the work copy")
	}
}

func TestVerifiedHookLegacyRestampCannotOverwriteAdoptedWork(t *testing.T) {
	v, _ := newVerifiedHookFixture(t)
	mem, bead := verifiedWorkFixture(t, v)
	from := v.SessionName
	if err := mem.Update(bead.ID, beads.UpdateOpts{Assignee: &from}); err != nil {
		t.Fatal(err)
	}
	race := &releaseWorkCASRace{MemStore: mem, beforeWrite: func() {
		if err := mem.Update(bead.ID, beads.UpdateOpts{Metadata: map[string]string{verifiedHookIncarnationKey: "2:successor", verifiedHookAttemptKey: "adopted"}}); err != nil {
			t.Fatal(err)
		}
	}}
	installVerifiedWorkStore(t, v, race)
	if moved, err := restampVerifiedHookWork(v, nil, "", bead.ID, from, v.assignee()); err != nil || moved {
		t.Fatalf("stale legacy restamp %v %v", moved, err)
	}
	live, _ := mem.Get(bead.ID)
	if live.Assignee != from || live.Metadata[verifiedHookAttemptKey] != "adopted" {
		t.Fatal("restamp mutated adopted work")
	}
}

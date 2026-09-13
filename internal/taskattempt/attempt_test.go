package taskattempt

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
)

func armedTask(t *testing.T) (*beads.MemStore, string) {
	t.Helper()
	s := beads.NewMemStore()
	b, err := s.Create(beads.Bead{Title: "bounded synthetic task"})
	if err != nil {
		t.Fatal(err)
	}
	value, err := Request("rig:fixture", b.ID, "fixture/worker")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMetadata(b.ID, beadmeta.NativeAttemptMetadataKey, value); err != nil {
		t.Fatal(err)
	}
	return s, b.ID
}

func TestAttemptReservationAndStartAreDurable(t *testing.T) {
	s, id := armedTask(t)
	token, err := Reserve(s, id, "rig:fixture", "fixture/worker")
	if err != nil || token == "" {
		t.Fatalf("reservation: %q %v", token, err)
	}
	for range 4 {
		if _, err := Reserve(s, id, "rig:fixture", "fixture/worker"); !errors.Is(err, ErrRefused) {
			t.Fatalf("replacement: %v", err)
		}
	}
	if err := Start(s, id, "rig:fixture", "fixture/worker", token, "session-1"); err != nil {
		t.Fatal(err)
	}
	for _, sid := range []string{"session-1", "replacement-session"} {
		if err := Start(s, id, "rig:fixture", "fixture/worker", token, sid); !errors.Is(err, ErrRefused) {
			t.Fatalf("second native start %s: %v", sid, err)
		}
	}
	b, err := s.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(id); err != nil {
		t.Fatal(err)
	}
	status := "open"
	if err := s.Update(id, beads.UpdateOpts{Status: &status}); err != nil {
		t.Fatal(err)
	}
	if _, err := Reserve(s, id, "rig:fixture", "fixture/worker"); !errors.Is(err, ErrRefused) {
		t.Fatalf("reopened task: %v", err)
	}
	after, _ := s.Get(id)
	if after.Metadata[beadmeta.NativeAttemptMetadataKey] != b.Metadata[beadmeta.NativeAttemptMetadataKey] {
		t.Fatal("attempt evidence changed on refusal")
	}
}

func TestAttemptConcurrentReservationAndStart(t *testing.T) {
	s, id := armedTask(t)
	var wg sync.WaitGroup
	var winners atomic.Int32
	tokens := make(chan string, 12)
	for range 12 {
		wg.Go(func() {
			got, err := Reserve(s, id, "rig:fixture", "fixture/worker")
			if err == nil {
				tokens <- got
				winners.Add(1)
			} else if !errors.Is(err, ErrRefused) {
				t.Errorf("reserve: %v", err)
			}
		})
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("reservations: %d", winners.Load())
	}
	token := <-tokens
	winners.Store(0)
	for range 12 {
		wg.Go(func() {
			if err := Start(s, id, "rig:fixture", "fixture/worker", token, "session-1"); err == nil {
				winners.Add(1)
			} else if !errors.Is(err, ErrRefused) {
				t.Errorf("start: %v", err)
			}
		})
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("native starts: %d", winners.Load())
	}
}

func TestAttemptBindingAndMissingCapabilityRefuse(t *testing.T) {
	s, id := armedTask(t)
	if _, err := Reserve(s, id, "rig:other", "fixture/worker"); !errors.Is(err, ErrRefused) {
		t.Fatal(err)
	}
	if _, err := Reserve(s, id, "rig:fixture", "fixture/other"); !errors.Is(err, ErrRefused) {
		t.Fatal(err)
	}
	if _, err := Reserve(struct{ beads.Store }{s}, id, "rig:fixture", "fixture/worker"); !errors.Is(err, ErrRefused) {
		t.Fatal(err)
	}
	token, err := Reserve(s, id, "rig:fixture", "fixture/worker")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "wrong"} {
		if err := Start(s, id, "rig:fixture", "fixture/worker", bad, "session-1"); !errors.Is(err, ErrRefused) {
			t.Fatal(err)
		}
	}
	if err := Start(s, id, "rig:fixture", "fixture/worker", token, ""); !errors.Is(err, ErrRefused) {
		t.Fatal(err)
	}
}

type ambiguousCAS struct {
	beads.Store
	writer beads.MetadataCASWriter
}

func (s ambiguousCAS) CompareAndSetMetadataKey(id, key, expected, next string) (bool, error) {
	_, err := s.writer.CompareAndSetMetadataKey(id, key, expected, next)
	if err != nil {
		return false, err
	}
	return false, errors.New("response lost after committed CAS")
}

func TestAttemptAmbiguousCASNeverRefunds(t *testing.T) {
	s, id := armedTask(t)
	if _, err := Reserve(ambiguousCAS{s, s}, id, "rig:fixture", "fixture/worker"); err == nil {
		t.Fatal("ambiguous mutation accepted")
	}
	if _, err := Reserve(s, id, "rig:fixture", "fixture/worker"); !errors.Is(err, ErrRefused) {
		t.Fatal(err)
	}
}

func TestAttemptAbsentPolicyPreservesDefaultAndMalformedRefuses(t *testing.T) {
	s := beads.NewMemStore()
	b, err := s.Create(beads.Bead{Title: "ordinary task"})
	if err != nil {
		t.Fatal(err)
	}
	if tok, err := Reserve(s, b.ID, "rig:fixture", "fixture/worker"); err != nil || tok != "" {
		t.Fatalf("legacy: %q %v", tok, err)
	}
	if err := Start(s, b.ID, "rig:fixture", "fixture/worker", "", "session"); err != nil {
		t.Fatal(err)
	}
	if err := Start(s, b.ID, "rig:fixture", "fixture/worker", "orphan-token", "session"); !errors.Is(err, ErrRefused) {
		t.Fatal(err)
	}
	for _, raw := range []string{"", "null", "{}", "{\"limit\":2}", "not-json"} {
		if err := s.SetMetadata(b.ID, beadmeta.NativeAttemptMetadataKey, raw); err != nil {
			t.Fatal(err)
		}
		if _, err := Reserve(s, b.ID, "rig:fixture", "fixture/worker"); !errors.Is(err, ErrRefused) {
			t.Fatalf("%q: %v", raw, err)
		}
	}
}

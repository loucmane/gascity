package beads_test

import (
	"reflect"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
)

func TestBdAssignIfUnassignedUsesAtomicStatusAndAssigneeGuards(t *testing.T) {
	for _, exit := range []int{0, 13} {
		r := &releaseVerbRunner{reply: func([]string) ([]byte, error) {
			if exit != 0 {
				return nil, &bdExit{code: exit}
			}
			return nil, nil
		}}
		s := beads.NewBdStore(t.TempDir(), r.run)
		err := s.AssignIfUnassigned("work-1", "session-1")
		if exit == 0 && err != nil {
			t.Fatal(err)
		}
		if exit == 13 && !beads.IsPreconditionFailed(err) {
			t.Fatalf("lost CAS: %v", err)
		}
		want := [][]string{{"bd", "update", "--json", "work-1", "--assignee", "session-1", "--if-assignee", "", "--if-status", "open"}}
		if got := r.releaseVerbArgv(); !reflect.DeepEqual(got, want) {
			t.Fatalf("argv=%q want=%q", got, want)
		}
	}
}

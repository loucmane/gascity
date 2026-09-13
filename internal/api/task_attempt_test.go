package api

import (
	"context"
	"errors"
	"testing"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/taskattempt"
	"github.com/gastownhall/gascity/internal/worker"
)

func TestWorkerFactoryAttemptUsesCityWorkAuthorityNotSessionDecoy(t *testing.T) {
	for _, tc := range []struct {
		name, configuredName string
		withToken            bool
	}{
		{"missing-token", "test-city", false},
		{"one-native-start", "test-city", true},
		{"resolved-name-missing-token", "", false},
		{"resolved-name-one-native-start", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withToken := tc.withToken
			fs := newSessionFakeState(t)
			fs.cfg.Workspace.Name = tc.configuredName
			tasks, sessions := fs.cityBeadStore, beads.NewMemStore()
			fs.sessionsBeadStore = sessions
			task, err := tasks.Create(beads.Bead{Title: "bounded city work"})
			if err != nil {
				t.Fatal(err)
			}
			policy, err := taskattempt.Request("city:test-city", task.ID, "myrig/worker")
			if err != nil {
				t.Fatal(err)
			}
			if err := tasks.SetMetadata(task.ID, beadmeta.NativeAttemptMetadataKey, policy); err != nil {
				t.Fatal(err)
			}
			decoy, err := sessions.Create(beads.Bead{ID: task.ID, Title: "unbounded same-ID decoy"})
			if err != nil || decoy.ID != task.ID {
				t.Fatalf("decoy: %s %v", decoy.ID, err)
			}
			meta := map[string]string{
				"template": "myrig/worker", "session_name": "bounded-api", "state": "creating",
				"instance_token": "fixture", "generation": "1", "command": "true",
				beadmeta.TriggerBeadIDMetadataKey: task.ID, beadmeta.TriggerBeadStoreRefMetadataKey: "city:test-city",
			}
			if withToken {
				token, err := taskattempt.Reserve(tasks, task.ID, "city:test-city", "myrig/worker")
				if err != nil {
					t.Fatal(err)
				}
				meta[beadmeta.NativeAttemptTokenMetadataKey] = token
			}
			b, err := sessions.Create(beads.Bead{Type: "session", Title: "bounded", Metadata: meta})
			if err != nil {
				t.Fatal(err)
			}
			factory, err := New(fs).workerFactory(sessions)
			if err != nil {
				t.Fatal(err)
			}
			h, err := factory.Session(worker.SessionSpec{ID: b.ID, Template: "myrig/worker", Command: "true"})
			if err != nil {
				t.Fatal(err)
			}
			fs.sp.StartErrors["bounded-api"] = errors.New("fixture failed startup")
			err = h.StartResolved(context.Background(), "true", runtime.Config{Command: "true"})
			if withToken {
				if err == nil {
					t.Fatal("missing fixture failure")
				}
				if got := fs.sp.CountCalls("Start", "bounded-api"); got != 1 {
					t.Fatalf("first calls: %d; %v", got, err)
				}
				err = h.StartResolved(context.Background(), "true", runtime.Config{Command: "true"})
			}
			if !errors.Is(err, taskattempt.ErrRefused) {
				t.Fatalf("expected authoritative refusal: %v", err)
			}
			want := 0
			if withToken {
				want = 1
			}
			if got := fs.sp.CountCalls("Start", "bounded-api"); got != want {
				t.Fatalf("calls: %d, want %d", got, want)
			}
		})
	}
}

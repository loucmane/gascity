package main

import (
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/runtime"
)

func TestRealizePoolDesiredSessions_DanglingProviderMarksDemandNeedsOperator(t *testing.T) {
	cityPath := t.TempDir()
	brokenCommand := filepath.Join(cityPath, "bin", "codex")
	store := beads.NewMemStore()
	work, err := store.Create(beads.Bead{
		Title:  "routed implementation work",
		Type:   "task",
		Status: "open",
		Metadata: map[string]string{
			"gc.routed_to": "worker",
		},
	})
	if err != nil {
		t.Fatalf("create routed work: %v", err)
	}

	cfg := &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Providers: map[string]config.ProviderSpec{
			"codex": {Command: brokenCommand},
		},
		Agents: []config.Agent{{
			Name:              "worker",
			Provider:          "codex",
			MaxActiveSessions: intPtr(1),
		}},
	}
	bp := newAgentBuildParams(
		"test-city",
		cityPath,
		cfg,
		runtime.NewFake(),
		time.Now().UTC(),
		store,
		io.Discard,
	)
	bp.lookPath = func(command string) (string, error) {
		if command == brokenCommand {
			return "", &exec.Error{Name: command, Err: errors.New("executable file not found")}
		}
		return exec.LookPath(command)
	}
	bp.sessionBeads = newSessionBeadSnapshot(nil)
	bp.providerHealthSnapshot = &providerHealthSnapshot{present: false}

	var stderr strings.Builder
	realizePoolDesiredSessions(
		bp,
		&cfg.Agents[0],
		PoolDesiredState{
			Template: "worker",
			Requests: []SessionRequest{{
				Template:     "worker",
				Tier:         "new",
				WorkBeadID:   work.ID,
				WorkStoreRef: "city",
			}},
		},
		map[string]TemplateParams{},
		&stderr,
	)

	got, err := store.Get(work.ID)
	if err != nil {
		t.Fatalf("reload routed work: %v", err)
	}
	if !slices.Contains(got.Labels, "needs/operator") {
		t.Fatalf("labels = %v, want needs/operator; stderr=%s", got.Labels, stderr.String())
	}
	if got.Metadata[beadmeta.FailureOwnerMetadataKey] != "gc.desired-state" {
		t.Fatalf("failure owner = %q, want gc.desired-state", got.Metadata[beadmeta.FailureOwnerMetadataKey])
	}
	if got.Metadata[beadmeta.FailureReasonMetadataKey] != "provider_command_unavailable" {
		t.Fatalf("failure reason = %q, want provider_command_unavailable", got.Metadata[beadmeta.FailureReasonMetadataKey])
	}
	if got.Metadata[beadmeta.ControllerErrorMetadataKey] == "" {
		t.Fatal("controller error is empty")
	}
	if got.Metadata["gc.provider_resolution_signature"] == "" {
		t.Fatal("provider resolution signature is empty")
	}
}

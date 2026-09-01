package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/session"
	"github.com/spf13/cobra"
)

// newSessionWakeCmd creates the "gc session wake <id-or-alias>" command.
func newSessionWakeCmd(stdout, stderr io.Writer) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "wake <session-id-or-alias>",
		Short: "Wake a session (request start and clear holds)",
		Long: `Request wake for a session and release user hold or crash-loop quarantine metadata.

After waking, the reconciler will start the session on its next tick
if it has wake reasons (e.g., a matching config agent). If the session
has no wake reasons, it remains asleep.

Accepts a session ID (e.g., gc-42) or session alias (e.g., mayor).`,
		Example: `  gc session wake gc-42
  gc session wake mayor`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdSessionWake(args, stdout, stderr, jsonOutput) != 0 {
				return errExit
			}
			return nil
		},
		ValidArgsFunction: completeSessionIDs,
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit JSONL")
	return cmd
}

type sessionWakeDeps struct {
	store                     beads.Store
	cfg                       *config.City
	cityPath                  string
	cityResolved              bool
	now                       func() time.Time
	withdrawQueuedWaitNudges  func(string, []string) error
	cityUsesManagedReconciler func(string) bool
	pokeController            func(string) error
}

type sessionWakeClient interface {
	WakeSession(string) (string, error)
}

// sessionWakeAPIClient resolves the authoritative in-process city mutation
// surface. It is a seam so command tests can prove that a live managed city is
// never mutated through a stale standalone backing-store view.
var sessionWakeAPIClient = func(cityPath string) (sessionWakeClient, bool) {
	// The race exists only for supervisor-managed cities, where the CLI's
	// backing-store write is outside the supervisor's CachingStore. Standalone
	// controllers retain the existing local-write + poke behavior.
	client := supervisorCityAPIClient(cityPath)
	_, registered, err := registeredCityEntry(cityPath)
	managed := err == nil && registered && supervisorAlive() != 0
	if client == nil {
		return nil, managed
	}
	return client, managed
}

// cmdSessionWake is the CLI entry point for "gc session wake".
func cmdSessionWake(args []string, stdout, stderr io.Writer, jsonOutput ...bool) int {
	asJSON := sessionJSONRequested(jsonOutput)
	cityPath, cityErr := resolveCity()
	var cfg *config.City
	if cityErr == nil {
		cfg, _ = loadCityConfig(cityPath, stderr)
		client, managed := sessionWakeAPIClient(cityPath)
		if client != nil {
			id, err := client.WakeSession(args[0])
			if err != nil {
				// Mutation responses are not safe to retry locally: the runtime may
				// have committed the wake before the transport failed. Fail closed
				// and let the operator inspect the authoritative session state.
				fmt.Fprintf(stderr, "gc session wake: authoritative runtime: %v\n", err) //nolint:errcheck
				return 1
			}
			return renderSessionWakeResult(id, stdout, stderr, asJSON, 0)
		}
		if managed {
			fmt.Fprintln(stderr, "gc session wake: authoritative runtime unavailable; session state was not changed") //nolint:errcheck
			return 1
		}
	}

	store, code := openCityStore(stderr, "gc session wake")
	if store == nil {
		return code
	}
	return doSessionWake(args[0], stdout, stderr, asJSON, sessionWakeDeps{
		store:                     store,
		cfg:                       cfg,
		cityPath:                  cityPath,
		cityResolved:              cityErr == nil,
		now:                       time.Now,
		withdrawQueuedWaitNudges:  withdrawQueuedWaitNudges,
		cityUsesManagedReconciler: cityUsesManagedReconciler,
		pokeController:            pokeController,
	})
}

func renderSessionWakeResult(id string, stdout, stderr io.Writer, asJSON bool, waitNudgesWithdrawn int) int {
	if asJSON {
		if err := writeSessionActionJSON(stdout, sessionActionResult{
			Action:              "wake",
			SessionID:           id,
			State:               "wake_requested",
			WaitNudgesWithdrawn: waitNudgesWithdrawn,
		}); err != nil {
			fmt.Fprintf(stderr, "gc session wake: %v\n", err) //nolint:errcheck
			return 1
		}
		return 0
	}
	fmt.Fprintf(stdout, "Session %s: wake requested.\n", id) //nolint:errcheck
	return 0
}

func doSessionWake(target string, stdout, stderr io.Writer, asJSON bool, deps sessionWakeDeps) int {
	sessStore := cliSessionStore(deps.store, deps.cfg, deps.cityPath)
	id, err := resolveSessionIDMaterializingNamed(deps.cityPath, deps.cfg, sessStore, target)
	if err != nil {
		fmt.Fprintf(stderr, "gc session wake: %v\n", err) //nolint:errcheck
		return 1
	}

	sessFront := sessionFrontDoor(sessStore)
	res, err := sessFront.WakeSession(id, deps.now().UTC(), session.WakeOpts{})
	if err != nil {
		if state, conflict := session.WakeConflictState(err); conflict {
			fmt.Fprintf(stderr, "gc session wake: session %s is %s\n", id, state) //nolint:errcheck
			return 1
		}
		switch {
		case errors.Is(err, session.ErrNotSessionBead):
			fmt.Fprintf(stderr, "gc session wake: %s is not a session\n", id) //nolint:errcheck
		case errors.Is(err, beads.ErrNotFound):
			fmt.Fprintf(stderr, "gc session wake: %v\n", err) //nolint:errcheck
		default:
			fmt.Fprintf(stderr, "gc session wake: updating metadata: %v\n", err) //nolint:errcheck
		}
		return 1
	}
	nudgeIDs := res.NudgeIDs
	hasRunnableTemplate := sessionWakeHasRunnableTemplateInfo(res.Info, deps.cfg)
	if !hasRunnableTemplate && sessionWakeRequestedCreateInfo(res.Info) {
		if err := sessFront.ApplyPatch(id, map[string]string{
			"state":                     string(session.StateAsleep),
			"state_reason":              "",
			"pending_create_claim":      "",
			"pending_create_started_at": "",
			"wake_request":              "",
			"wake_requested_at":         "",
		}); err != nil {
			fmt.Fprintf(stderr, "gc session wake: updating metadata: %v\n", err) //nolint:errcheck
			return 1
		}
	}
	if deps.cityResolved {
		if err := deps.withdrawQueuedWaitNudges(deps.cityPath, nudgeIDs); err != nil {
			fmt.Fprintf(stderr, "gc session wake: warning: withdrawing queued wait nudges: %v\n", err) //nolint:errcheck
		}
		if deps.cityUsesManagedReconciler(deps.cityPath) {
			if err := deps.pokeController(deps.cityPath); err != nil {
				fmt.Fprintf(stderr, "gc session wake: warning: poke failed: %v\n", err) //nolint:errcheck
			}
		}
	}

	return renderSessionWakeResult(id, stdout, stderr, asJSON, len(nudgeIDs))
}

func sessionWakeHasRunnableTemplateInfo(info session.Info, cfg *config.City) bool {
	if cfg == nil {
		return true
	}
	template := normalizedSessionTemplateInfo(info, cfg)
	if template == "" {
		template = info.Template
	}
	return findAgentByTemplate(cfg, template) != nil
}

func sessionWakeRequestedCreateInfo(info session.Info) bool {
	state := session.State(strings.TrimSpace(info.MetadataState))
	return state == session.StateSuspended || state == session.StateDrained
}

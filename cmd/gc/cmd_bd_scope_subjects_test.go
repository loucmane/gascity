package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/config"
)

// ga-z9hz: flag values can name real beads without being mutation subjects.
func TestResolveBdScopeTargetMutationSubjects(t *testing.T) {
	cityDir := t.TempDir()
	setCwd(t, cityDir)
	cfg := &config.City{
		Workspace: config.Workspace{Name: "city", Prefix: "ci"},
		Rigs:      []config.Rig{{Name: "bounty", Path: filepath.Join(cityDir, "bounty"), Prefix: "bty"}},
	}
	city := bdCityScopeTarget(cityDir, cfg)
	bounty := bdRigScopeTarget(cityDir, cfg.Rigs[0])
	const sessionID = "ci-wisp-eahk7sl"
	const reviewID = "bty-8r3"
	const otherID = "bty-o1c"

	originalProbe := bdBeadExists
	t.Cleanup(func() { bdBeadExists = originalProbe })
	var probed []string
	bdBeadExists = func(_ string, _ *config.City, target execStoreTarget, id string) (bool, error) {
		probed = append(probed, id)
		return (target == city && id == sessionID) ||
			(target == bounty && (id == reviewID || id == otherID)), nil
	}

	tests := []struct {
		name         string
		args         []string
		rigName      string
		cityExplicit bool
		want         execStoreTarget
		wantProbed   []string
	}{
		{
			name: "show review",
			args: []string{"show", reviewID}, want: bounty, wantProbed: []string{reviewID},
		},
		{
			name: "spaced guarded update",
			args: []string{"update", reviewID, "--if-revision=-3471981609212282325", "--if-status", "in_progress", "--if-assignee", sessionID, "--set-metadata", "gc.work_outcome=no-op", "--append-notes", "Review complete."},
			want: bounty, wantProbed: []string{reviewID},
		},
		{
			name: "joined guarded update",
			args: []string{"update", reviewID, "--if-revision=-3471981609212282325", "--if-status=in_progress", "--if-assignee=" + sessionID, "--set-metadata=gc.work_outcome=no-op", "--append-notes=Review complete."},
			want: bounty, wantProbed: []string{reviewID},
		},
		{
			name: "assignee and notes before subject",
			args: []string{"update", "--assignee", sessionID, "--append-notes", sessionID, reviewID},
			want: bounty, wantProbed: []string{reviewID},
		},
		{
			name: "note text beginning with city id",
			args: []string{"update", reviewID, "--append-notes", sessionID + " completed the review"},
			want: bounty, wantProbed: []string{reviewID},
		},
		{
			name: "other value flags",
			args: []string{"update", "--title", sessionID, "-d", sessionID, "--parent", sessionID, "--session", sessionID, "--notes", sessionID, "--body-file", sessionID, reviewID},
			want: bounty, wantProbed: []string{reviewID},
		},
		{
			name: "rig value before subject",
			args: []string{"update", "--assignee", otherID, reviewID},
			want: bounty, wantProbed: []string{reviewID},
		},
		{
			name: "leading spaced global flags",
			args: []string{"--json", "--actor", sessionID, "--database", otherID, "update", reviewID, "--if-assignee", sessionID},
			want: bounty, wantProbed: []string{reviewID},
		},
		{
			name: "leading joined global flag",
			args: []string{"--actor=" + sessionID, "update", reviewID, "--if-assignee", sessionID},
			want: bounty, wantProbed: []string{reviewID},
		},
		{
			name: "close reason is not a subject",
			args: []string{"close", reviewID, "--reason", sessionID},
			want: bounty, wantProbed: []string{reviewID},
		},
		{
			name: "reopen reason is not a subject",
			args: []string{"reopen", "-r", sessionID, reviewID},
			want: bounty, wantProbed: []string{reviewID},
		},
		{
			name: "delete file is not a subject",
			args: []string{"delete", "--from-file", sessionID, reviewID},
			want: bounty, wantProbed: []string{reviewID},
		},
		{
			name: "heartbeat subject",
			args: []string{"heartbeat", reviewID}, want: bounty, wantProbed: []string{reviewID},
		},
		{
			name: "no mutation subjects",
			args: []string{"update", "--assignee", sessionID}, want: city,
		},
		{
			name: "terminator leaves positional subjects",
			args: []string{"update", "--assignee", sessionID, "--", reviewID},
			want: bounty, wantProbed: []string{reviewID},
		},
		{
			name: "positional city read",
			args: []string{"show", sessionID}, want: city, wantProbed: []string{sessionID},
		},
		{
			name: "positional city mutation",
			args: []string{"update", sessionID, "--assignee", reviewID}, want: city, wantProbed: []string{sessionID},
		},
		{
			name: "city subject after terminator",
			args: []string{"close", "--", sessionID}, want: city, wantProbed: []string{sessionID},
		},
		{
			name: "city subject retains priority in a batch",
			args: []string{"update", reviewID, sessionID}, want: city, wantProbed: []string{sessionID},
		},
		{
			name: "explicit city wins over rig subject",
			args: []string{"update", reviewID, "--if-assignee", sessionID}, cityExplicit: true, want: city,
		},
		{
			name: "explicit rig wins over city subject",
			args: []string{"update", sessionID}, rigName: "bounty", want: bounty,
		},
		{
			name: "explicit rig wins over explicit city",
			args: []string{"update", sessionID}, rigName: "bounty", cityExplicit: true, want: bounty,
		},
		{
			name: "ambiguous mutation uses raw city first probing",
			args: []string{"update", reviewID, "--unknown-flag", sessionID}, want: city, wantProbed: []string{sessionID},
		},
		{
			name: "ambiguous mutation with leading globals",
			args: []string{"--actor", sessionID, "update", "--unknown-flag", sessionID, reviewID}, want: city, wantProbed: []string{sessionID},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probed = nil
			got, err := resolveBdScopeTarget(cfg, cityDir, tt.rigName, tt.args, tt.cityExplicit, io.Discard)
			if err != nil {
				t.Errorf("resolveBdScopeTarget: %v", err)
			} else if got != tt.want {
				t.Errorf("target = %#v, want %#v", got, tt.want)
			}
			if !reflect.DeepEqual(probed, tt.wantProbed) {
				t.Errorf("probed = %q, want only subjects %q", probed, tt.wantProbed)
			}
		})
	}
	for _, verb := range []string{"update", "close", "reopen", "delete", "heartbeat"} {
		for _, tt := range bdMutationGlobalFlagCases(verb, sessionID) {
			t.Run(verb+"/"+tt.name, func(t *testing.T) {
				probed = nil
				got, err := resolveBdScopeTarget(cfg, cityDir, "", tt.args, false, io.Discard)
				if err != nil {
					t.Fatalf("resolveBdScopeTarget(%q): %v", tt.args, err)
				}
				if got != bounty {
					t.Errorf("target = %#v, want %#v", got, bounty)
				}
				if !reflect.DeepEqual(probed, []string{reviewID}) {
					t.Errorf("probed = %q, want only subject %q", probed, reviewID)
				}
			})
		}
	}
	for _, flags := range [][]string{
		{"-s", "open"},
		{"-sopen"},
		{"-s=open"},
		{"--status=open"},
		{"-d", sessionID},
		{"-d" + sessionID},
		{"-description"},
		{"-qvd", sessionID},
		{"-qvd" + sessionID},
		{"-qv=false"},
		{"--acceptance-criteria", sessionID},
		{"--body", sessionID},
		{"--description-file", sessionID},
		{"-m", sessionID},
	} {
		t.Run(strings.Join(flags, " "), func(t *testing.T) {
			probed = nil
			args := append([]string{"--actor", sessionID, "update"}, flags...)
			args = append(args, reviewID)
			got, err := resolveBdScopeTarget(cfg, cityDir, "", args, false, io.Discard)
			if err != nil || got != bounty || !reflect.DeepEqual(probed, []string{reviewID}) {
				t.Fatalf("resolve(%q) = %#v, %v; probes=%q; want bounty subject only", args, got, err, probed)
			}
		})
	}
	for _, args := range [][]string{
		{"--actor", sessionID, "update", reviewID, "-qz"},
		{"--actor", sessionID, "update", reviewID, "-qz=value"},
		{"--actor", sessionID, "update", reviewID, "--unknown=value"},
		{"--actor", sessionID, "update", reviewID, "--title"},
		{"--actor", sessionID, "update", reviewID, "-d"},
		{"--actor", sessionID, "update", reviewID, "--json=invalid"},
		{"--actor", sessionID, "update", reviewID, "-q=invalid"},
		{"--unknown", "--actor", sessionID, "update", reviewID},
		{"-z", "--actor", sessionID, "update", reviewID},
		{"--actor", sessionID, "--", "update", reviewID},
	} {
		t.Run("fallback "+strings.Join(args, " "), func(t *testing.T) {
			probed = nil
			got, err := resolveBdScopeTarget(cfg, cityDir, "", args, false, io.Discard)
			if err != nil || got != city || !reflect.DeepEqual(probed, []string{sessionID}) {
				t.Fatalf("resolve(%q) = %#v, %v; probes=%q; want base raw city probe", args, got, err, probed)
			}
		})
	}
}

func TestGcBdGuardedUpdateKeepsRigScopeAndArgs(t *testing.T) {
	rigDir := bdMutationScopeFixture(t)
	args := []string{
		"update", "bty-8r3", "--if-revision=-3471981609212282325",
		"--if-status", "in_progress", "--if-assignee", "ci-wisp-eahk7sl",
		"--set-metadata", "gc.work_outcome=no-op",
		"--append-notes", "ci-wisp-eahk7sl finished the review.\nKeep the guards and this note unchanged.",
	}
	want := append([]string{rigDir}, args...)
	var stdout, stderr bytes.Buffer
	if code := doBd(args, &stdout, &stderr); code != 0 {
		t.Fatalf("doBd = %d, want 0; stderr=%q", code, stderr.String())
	}
	got := strings.Split(strings.TrimSuffix(stdout.String(), "\x00"), "\x00")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bd directory/argv = %q, want %q", got, want)
	}
}

// The resolver must understand globals on either side of every mutation verb.
// Use a real city bead as the actor in the resolver proof, and an ordinary
// actor in the subprocess proof so it can also run against the base revision.
func bdMutationGlobalFlagCases(verb, actor string) []struct {
	name string
	args []string
} {
	return []struct {
		name string
		args []string
	}{
		{"globals before", []string{"--actor", actor, "--json", verb, "bty-8r3"}},
		{"globals after", []string{verb, "bty-8r3", "--actor", actor, "--json"}},
		{"globals both sides", []string{"--json", verb, "--actor", actor, "bty-8r3"}},
		{"actor before json after", []string{"--actor", actor, verb, "bty-8r3", "--json"}},
		{"joined globals", []string{"--actor=" + actor, verb, "--format=json", "bty-8r3", "--json=false"}},
	}
}

func TestGcBdMutationGlobalFlagsKeepRigScopeAndArgs(t *testing.T) {
	rigDir := bdMutationScopeFixture(t)
	for _, verb := range []string{"update", "close", "reopen", "delete", "heartbeat"} {
		for _, tt := range bdMutationGlobalFlagCases(verb, "alice") {
			t.Run(verb+"/"+tt.name, func(t *testing.T) {
				want := append([]string{rigDir}, tt.args...)
				var stdout, stderr bytes.Buffer
				code := doBd(tt.args, &stdout, &stderr)
				// Preserve the base's pre-routing validation: a raw heartbeat
				// accepts exactly one ID, whereas leading globals pass through.
				if verb == "heartbeat" && tt.args[0] == "heartbeat" {
					if code != 1 || !strings.Contains(stderr.String(), "usage: gc bd heartbeat <issue-id>") || stdout.Len() != 0 {
						t.Fatalf("doBd = %d, stdout=%q, stderr=%q; want existing heartbeat usage refusal", code, stdout.String(), stderr.String())
					}
					return
				}
				if code != 0 {
					t.Fatalf("doBd(%q) = %d, want 0; stderr=%q", tt.args, code, stderr.String())
				}
				got := strings.Split(strings.TrimSuffix(stdout.String(), "\x00"), "\x00")
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("bd directory/argv = %q, want %q", got, want)
				}
			})
		}
	}
}

// The base replay overlays this constant along with the exact base production
// files. Every row still checks doBd, scope environment, exit status and argv.
const bdScopeReplayBase = false

// ga-z9hz: a scope-only alias must not teach the exact-ID guard to swallow a
// subject. The fake bd resolves bty-8r to bty-8r3, as a partial-ID resolver can.
func TestGcBdScopeManifestAliasesPreserveExactIDGuard(t *testing.T) {
	rigDir := bdMutationScopeFixture(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"close", "-resolution", "bty-8r"}, "cannot safely verify bead IDs"},
		{[]string{"update", "-message", "bty-8r"}, "cannot safely verify bead IDs"},
		{[]string{"update", "-description-file", "bty-8r"}, "cannot safely verify bead IDs"},
		{[]string{"update", "-acceptance-criteria", "bty-8r"}, "cannot safely verify bead IDs"},
		{[]string{"close", "-message", "bty-8r"}, "cannot safely verify bead IDs"},
		{[]string{"close", "bty-8r"}, "substring collision"},
		{[]string{"update", "bty-8r", "--status", "open"}, "substring collision"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			args := append([]string{"--rig", "bounty"}, tc.args...)
			var stdout, stderr bytes.Buffer
			code := doBd(args, &stdout, &stderr)
			if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("doBd(%q) = %d, stdout=%q, stderr=%q; want base refusal %q and no mutation", args, code, stdout.String(), stderr.String(), tc.want)
			}
			t.Logf("PARITY %q: exit=1; %s; no mutation forwarded", args, tc.want)
		})
	}
	t.Run("exact subject forwards", func(t *testing.T) {
		args := []string{"close", "bty-8r3"}
		var stdout, stderr bytes.Buffer
		code := doBd(append([]string{"--rig", "bounty"}, args...), &stdout, &stderr)
		got := strings.Split(strings.TrimSuffix(stdout.String(), "\x00"), "\x00")
		if code != 0 || !reflect.DeepEqual(got, append([]string{rigDir}, args...)) {
			t.Fatalf("exact-ID control: exit=%d, stdout=%q, stderr=%q", code, stdout.String(), stderr.String())
		}
	})
}

// ga-z9hz: recognizing heartbeat for routing must not expand gc lint's
// validation scope. Exercise the real lint command with only a fake bd in PATH.
func TestGcBdScopeManifestPreservesHeartbeatLint(t *testing.T) {
	bdMutationScopeFixture(t)
	packDir := t.TempDir()
	writeLintPack(t, packDir, "heartbeat-flags", "worker", "prompts/worker.template.md")
	for _, flags := range []string{"-qv", "-vq", "-q=false", "-Cpath", "-C=path", "-qvCpath", "-qvC path", "--actor alice --json"} {
		t.Run(flags, func(t *testing.T) {
			writeLintFile(t, filepath.Join(packDir, "prompts", "worker.template.md"),
				"Agent {{.AgentName}}\n`bd heartbeat bty-8r3 "+flags+"`\n")
			var stdout, stderr bytes.Buffer
			code := run([]string{"lint", packDir}, &stdout, &stderr)
			if code != 0 || stderr.Len() != 0 {
				t.Fatalf("gc lint heartbeat %s: exit=%d, stdout=%q, stderr=%q; want base zero findings", flags, code, stdout.String(), stderr.String())
			}
			t.Logf("PARITY bd heartbeat bty-8r3 %s: gc lint exit=0; zero findings", flags)
		})
	}
}

func TestGcBdScopeCompatibilityMatrix(t *testing.T) {
	rigDir := bdMutationScopeFixture(t)
	cityDir := filepath.Dir(rigDir)
	for _, tc := range []struct {
		name, verb   string
		flags        []string
		guardRefusal bool
	}{
		{"status spaced", "update", []string{"-s", "open"}, false},
		{"status attached", "update", []string{"-sopen"}, true},
		{"status short equals", "update", []string{"-s=open"}, false},
		{"status long equals", "update", []string{"--status=open"}, false},
		{"description spaced", "update", []string{"-d", "text"}, false},
		{"single dash long collision", "update", []string{"-description"}, false},
		{"boolean cluster", "update", []string{"-qv"}, true},
		{"cluster spaced value", "update", []string{"-qvs", "open"}, true},
		{"cluster attached value", "update", []string{"-qvsopen"}, true},
		{"cluster equals value", "update", []string{"-qvs=open"}, false},
		{"acceptance legacy", "update", []string{"--acceptance-criteria", "done"}, true},
		{"body alias", "update", []string{"--body", "text"}, true},
		{"description file alias", "update", []string{"--description-file", "text"}, true},
		{"message alias", "update", []string{"-m", "text"}, true},
		{"close attached reason", "close", []string{"-rdone"}, true},
		{"close cluster", "close", []string{"-qfrdone"}, true},
		{"close message alias", "close", []string{"-m", "done"}, true},
		{"close resolution alias", "close", []string{"--resolution", "done"}, true},
		{"close comment alias", "close", []string{"--comment", "done"}, true},
		{"reopen attached reason", "reopen", []string{"-rdone"}, true},
		{"delete cluster", "delete", []string{"-qvf"}, true},
		{"unknown short", "update", []string{"-qz"}, true},
		{"unknown long", "update", []string{"--future-flag", "text"}, true},
	} {
		for _, placement := range []string{"before", "after", "both"} {
			t.Run(tc.name+"/"+placement, func(t *testing.T) {
				// Put flags before the subject to expose swallowed subjects.
				args := append([]string{tc.verb}, tc.flags...)
				args = append(args, "bty-8r3")
				if placement != "after" {
					args = append([]string{"--actor", "alice"}, args...)
				}
				if placement != "before" {
					args = append(args, "--json")
				}
				refused := tc.guardRefusal
				checkBdScopeReplay(t, args, rigDir, placement == "after" && refused)
			})
		}
	}
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"guard city value", []string{"update", "bty-8r3", "--if-assignee", "ci-wisp-eahk7sl"}},
		{"description city value", []string{"update", "-d", "ci-wisp-eahk7sl", "bty-8r3"}},
		{"leading actor city value", []string{"--actor", "ci-wisp-eahk7sl", "update", "bty-8r3"}},
		{"actor and guard city values", []string{"--actor", "ci-wisp-eahk7sl", "update", "bty-8r3", "--if-assignee", "ci-wisp-eahk7sl"}},
		{"close reason city value", []string{"close", "bty-8r3", "--reason", "ci-wisp-eahk7sl"}},
		{"reopen reason city value", []string{"reopen", "bty-8r3", "-r", "ci-wisp-eahk7sl"}},
		{"delete file city value", []string{"delete", "--from-file", "ci-wisp-eahk7sl", "bty-8r3"}},
		{"heartbeat actor city value", []string{"--actor", "ci-wisp-eahk7sl", "heartbeat", "bty-8r3", "--json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantDir := rigDir
			if bdScopeReplayBase {
				wantDir = cityDir
			}
			checkBdScopeReplay(t, tc.args, wantDir, false)
		})
	}
	for _, flags := range [][]string{
		{"--future-flag", "text"},
		{"--future-flag=text"},
		{"-qz"},
		{"-qz=text"},
		{"--title"},
		{"-d"},
		{"--json=invalid"},
		{"-q=invalid"},
	} {
		t.Run("raw fallback "+strings.Join(flags, " "), func(t *testing.T) {
			args := append([]string{"--actor", "ci-wisp-eahk7sl", "update", "bty-8r3"}, flags...)
			checkBdScopeReplay(t, args, cityDir, false)
		})
	}
	for _, args := range [][]string{
		{"--actor", "alice", "update", "bty-8r3", "-sopen"},
		{"--actor", "alice", "close", "bty-8r3", "-rdone"},
		{"update", "-description", "bty-8r3"},
		{"--actor", "alice", "update", "bty-8r3", "--acceptance-criteria", "done"},
		{"-qv", "--actor", "alice", "update", "bty-8r3", "-sopen"},
		{"-qv", "close", "bty-8r3", "-frdone", "--json"},
	} {
		t.Run("reported "+strings.Join(args, " "), func(t *testing.T) {
			checkBdScopeReplay(t, args, rigDir, false)
		})
	}
}

func checkBdScopeReplay(t *testing.T, args []string, wantDir string, refused bool) {
	t.Helper()
	probe := bdBeadExists
	defer func() { bdBeadExists = probe }()
	var selectedRoot string
	bdBeadExists = func(cityPath string, cfg *config.City, target execStoreTarget, id string) (bool, error) {
		exists, err := probe(cityPath, cfg, target, id)
		if exists {
			selectedRoot = target.ScopeRoot
		}
		return exists, err
	}
	want := append([]string{wantDir}, args...)
	var stdout, stderr bytes.Buffer
	code := doBd(args, &stdout, &stderr)
	if selectedRoot != wantDir {
		t.Fatalf("doBd(%q) selected store %q, want %q", args, selectedRoot, wantDir)
	}
	if refused {
		if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "cannot safely verify bead IDs") {
			t.Fatalf("doBd(%q) = %d, stdout=%q, stderr=%q; want base exact-ID guard refusal", args, code, stdout.String(), stderr.String())
		}
		t.Logf("PARITY %q: bounty; exact-ID guard refusal", args)
		return
	}
	got := strings.Split(strings.TrimSuffix(stdout.String(), "\x00"), "\x00")
	if code != 0 || !reflect.DeepEqual(got, want) {
		t.Fatalf("doBd(%q) = %d, stdout=%q, stderr=%q; want %q", args, code, stdout.String(), stderr.String(), want)
	}
	scope := "bounty"
	if filepath.Base(wantDir) != "bounty" {
		scope = "city"
	}
	t.Logf("PARITY %q: %s; unchanged argv", args, scope)
}

func bdMutationScopeFixture(t *testing.T) string {
	t.Helper()
	_, cityDir := bdSQLRefusalCityFixture(t, "", false)
	setCwd(t, cityDir)
	rigDir := filepath.Join(cityDir, "bounty")
	if err := os.MkdirAll(filepath.Join(rigDir, ".beads"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cityDir, "city.toml"), []byte(`[workspace]
name = "city"
prefix = "ci"

[[rigs]]
name = "bounty"
path = "bounty"
prefix = "bty"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	// A local backend marker lets the fake bd run without a managed server.
	if err := os.WriteFile(filepath.Join(rigDir, ".beads", "metadata.json"), []byte(`{"backend":"doltlite"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	bdBeadExists = func(_ string, _ *config.City, target execStoreTarget, id string) (bool, error) {
		return (target.ScopeKind == "city" && id == "ci-wisp-eahk7sl") ||
			(target.RigName == "bounty" && id == "bty-8r3"), nil
	}
	// Record argv with NUL delimiters, so spaces and newlines in the note
	// cannot hide argument splitting. Check only the synthetic scope env.
	binDir := t.TempDir()
	script := `#!/bin/sh
if [ "$1" = show ]; then
  printf '[{"id":"bty-8r3","status":"in_progress"}]\n'
  exit 0
fi
if [ "$PWD" != "$GC_STORE_ROOT" ] ||
   [ "$BEADS_DIR" != "$PWD/.beads" ]; then
  echo "wrong scope environment" >&2
  exit 71
fi
case "$GC_STORE_SCOPE" in
rig)
  if [ "$GC_RIG" != bounty ] || [ "$GC_RIG_ROOT" != "$PWD" ] || [ "$GC_BEADS_PREFIX" != bty ]; then
    echo "wrong rig environment" >&2
    exit 72
  fi ;;
city)
  if [ -n "$GC_RIG" ] || [ -n "$GC_RIG_ROOT" ] || [ "$GC_BEADS_PREFIX" != ci ]; then
    echo "wrong city environment" >&2
    exit 73
  fi ;;
*) exit 74 ;;
esac
printf '%s\000' "$PWD" "$@"
`
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return rigDir
}

// ga-z9hz: help omits registered hidden aliases. This checked list comes from
// cmd/bd at e243402ba64058a1e2861b0e9d5b84d3efb589d3, including
// registerCommonIssueFlags, registerPriorityFlag and registerCloseReasonFlag.
// heartbeat.go:104-107 registers no local flags; it inherits the global rows.
// Keep source locations when updating the pin; runtime discovery is not used.
func TestBdScopeManifestsCoverPinnedBDRegistrations(t *testing.T) {
	const registrations = `* main.go:872 value --directory -C
* main.go:873 value --db
* main.go:874 value --database
* main.go:875 value --actor
* main.go:876 bool --json
* main.go:877 value --format
* main.go:879 bool --sandbox
* main.go:880 bool --readonly
* main.go:881 bool --global
* main.go:882 value --dolt-auto-commit
* main.go:883 bool --cpu-profile
* main.go:884 value --mem-profile
* main.go:885 bool --verbose -v
* main.go:886 bool --quiet -q
* main.go:887 bool --ignore-schema-skew
* main.go:888 bool --no-color
update flags.go:14 value --assignee -a
update flags.go:15 value --description -d
update flags.go:16 value --body
update flags.go:18 value --message -m
update flags.go:20 value --body-file
update flags.go:21 value --description-file
update flags.go:23 bool --stdin
update flags.go:29 value --design
update flags.go:30 value --design-file
update flags.go:32 value --acceptance
update flags.go:33 value --notes
update flags.go:34 value --append-notes
update flags.go:35 value --external-ref
update flags.go:334 value --priority -p
update update.go:1022 value --status -s
update update.go:1024 value --title
update update.go:1025 value --type -t
update update.go:1028 bool --allow-empty-description
update update.go:1029 value --spec-id
update update.go:1030 value --acceptance-criteria
update update.go:1032 value --estimate -e
update update.go:1033 value --add-label
update update.go:1034 value --remove-label
update update.go:1035 value --set-labels
update update.go:1036 value --parent
update update.go:1037 bool --claim
update update.go:1039 bool --force
update update.go:1041 value --if-assignee
update update.go:1042 value --if-status
update update.go:1044 value --if-revision
update update.go:1051 value --session
update update.go:1061 value --due
update update.go:1062 value --defer
update update.go:1064 value --await-id
update update.go:1066 bool --ephemeral
update update.go:1067 bool --persistent
update update.go:1068 bool --no-history
update update.go:1069 bool --history
update update.go:1071 value --metadata
update update.go:1073 value --set-metadata
update update.go:1074 value --unset-metadata
close close.go:507 value --resolution
close close.go:509 value --message -m
close close.go:511 value --comment
close close.go:513 value --reason-file
close close.go:514 bool --force -f
close close.go:515 bool --continue
close close.go:516 bool --no-auto
close close.go:517 bool --suggest-next
close close.go:518 bool --claim-next
close close.go:519 value --session
close close.go:521 value --if-revision
close close.go:531 value --reason -r
reopen reopen.go:212 value --reason -r
reopen reopen.go:216 value --if-revision
delete delete.go:565 bool --force -f
delete delete.go:566 value --from-file
delete delete.go:567 bool --dry-run
delete delete.go:568 bool --cascade
delete delete.go:570 value --if-revision`
	if got := len(strings.Split(registrations, "\n")); got != 76 {
		t.Fatalf("registration checklist has %d rows, want 76", got)
	}
	for _, sub := range []string{"", "update", "close", "reopen", "delete", "heartbeat"} {
		t.Run(sub, func(t *testing.T) {
			values, bools := bdScopeFlagSets(sub)
			if values == nil || bools == nil {
				t.Fatalf("missing manifest for %s", sub)
			}
			for _, flag := range []string{"--help", "-h", "--version", "-V", "--profile"} {
				if !bools[flag] || values[flag] {
					t.Errorf("missing boolean compatibility flag %s", flag)
				}
			}
			for _, row := range strings.Split(registrations, "\n") {
				fields := strings.Fields(row)
				if fields[0] != "*" && fields[0] != sub {
					continue
				}
				for _, flag := range fields[3:] {
					wantValue := fields[2] == "value"
					if values[flag] != wantValue || bools[flag] == wantValue {
						t.Errorf("%s registered at cmd/bd/%s as %s: value=%v bool=%v", flag, fields[1], fields[2], values[flag], bools[flag])
					}
				}
			}
		})
	}
}

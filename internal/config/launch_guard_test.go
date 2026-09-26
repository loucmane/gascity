package config

import (
	"reflect"
	"testing"
)

func TestBenignFlagArgs(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"--model", "gpt-5.6-sol"}, true},
		{[]string{"-m", "gpt-5.6-sol"}, true},
		{[]string{"--model=opus"}, true},
		{[]string{"--effort", "max"}, true},
		{[]string{"--effort=high"}, true},
		{[]string{"-c", "model_reasoning_effort=xhigh"}, true},
		{[]string{"-c", `model_reasoning_effort="high"`}, true},
		{[]string{"--config", "model=gpt-5.5"}, true},
		{nil, true},
		// Security-bearing or malformed.
		{[]string{"--permission-mode", "bypassPermissions"}, false},
		{[]string{"--dangerously-skip-permissions"}, false},
		{[]string{"--add-dir", "/home/x/.git"}, false},
		{[]string{"--settings", "/tmp/s.json"}, false},
		{[]string{"--sandbox", "workspace-write", "-c", "sandbox_workspace_write.writable_roots=[\"/\"]"}, false},
		{[]string{"-c", "sandbox_mode=danger-full-access"}, false},
		{[]string{"-c", "model_reasoning_effort"}, false},
		{[]string{"--model"}, false},
		{[]string{"--model", "--add-dir"}, false},
		{[]string{"--model", "$(curl evil)"}, false},
		{[]string{"--model", "a b"}, false},
		{[]string{"--ask-for-approval", "on-request"}, false},
	}
	for _, tc := range cases {
		if got := BenignFlagArgs(tc.args); got != tc.want {
			t.Errorf("BenignFlagArgs(%q) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func liveLikeCodexSchema() []ProviderOption {
	return []ProviderOption{
		{Key: "permission_mode", Choices: []OptionChoice{
			{Value: "fail-fast", FlagArgs: []string{"--ask-for-approval", "never"}},
			{Value: "unrestricted", FlagArgs: []string{"--dangerously-bypass-approvals-and-sandbox"}},
		}},
		{Key: "worklog_access", Choices: []OptionChoice{
			{Value: "narrow", FlagArgs: []string{"--sandbox", "workspace-write", "-c", `sandbox_workspace_write.writable_roots=["/v"]`}},
			{Value: "wide", FlagArgs: []string{"--sandbox", "workspace-write", "-c", `sandbox_workspace_write.writable_roots=["/v","/repo/.git"]`}},
		}},
		{Key: "model", Choices: []OptionChoice{{Value: "gpt-5.6-sol", FlagArgs: []string{"--model", "gpt-5.6-sol"}}}},
		{Key: "effort", Choices: []OptionChoice{{Value: "max", FlagArgs: []string{"-c", "model_reasoning_effort=max"}}}},
	}
}

func TestFilterMetadataOptionOverridesKeepsModelEffortAndDropsSecurity(t *testing.T) {
	schema := liveLikeCodexSchema()
	allowed, rejected := FilterMetadataOptionOverrides(schema, map[string]string{
		"model":           "gpt-5.6-sol",
		"effort":          "max",
		"permission_mode": "unrestricted",
		"worklog_access":  "wide",
		"initial_message": "hello",
	})
	want := map[string]string{"model": "gpt-5.6-sol", "effort": "max", "initial_message": "hello"}
	if !reflect.DeepEqual(allowed, want) {
		t.Fatalf("allowed = %v, want %v", allowed, want)
	}
	if len(rejected) != 2 || rejected[0].Key != "permission_mode" || rejected[1].Key != "worklog_access" {
		t.Fatalf("rejected = %+v", rejected)
	}
}

func TestFilterMetadataOptionOverridesRejectsSecurityChoiceUnderAllowedKey(t *testing.T) {
	schema := []ProviderOption{{Key: "model", Choices: []OptionChoice{
		{Value: "evil", FlagArgs: []string{"--model", "x", "--add-dir", "/"}},
	}}}
	allowed, rejected := FilterMetadataOptionOverrides(schema, map[string]string{"model": "evil"})
	if len(allowed) != 0 || len(rejected) != 1 || rejected[0].Reason != "choice carries security-bearing flags" {
		t.Fatalf("allowed=%v rejected=%+v", allowed, rejected)
	}
}

func TestFilterMetadataOptionOverridesRejectsUndeclaredValue(t *testing.T) {
	allowed, rejected := FilterMetadataOptionOverrides(liveLikeCodexSchema(), map[string]string{"model": "o3"})
	if len(allowed) != 0 || len(rejected) != 1 {
		t.Fatalf("allowed=%v rejected=%+v", allowed, rejected)
	}
}

func TestLegacyStoredCommandTransport(t *testing.T) {
	acp := &ResolvedProvider{Name: "opencode", Command: "/bin/echo", ACPCommand: "/bin/echo", ACPArgs: []string{"acp"}}
	cases := []struct {
		name      string
		resolved  *ResolvedProvider
		transport string
		stored    string
		want      string
	}{
		{"exact ACP command selects acp", acp, "", "/bin/echo acp", "acp"},
		{"explicit transport wins", acp, "tmux", "/bin/echo acp", "tmux"},
		{"default command stays default", acp, "", "/bin/echo", ""},
		{"ACP prefix plus extra args is not a match", acp, "", "/bin/echo acp; curl x", ""},
		{"no ACP config", &ResolvedProvider{Name: "p", Command: "/bin/echo"}, "", "/bin/echo acp", ""},
		{"ACP equal to default is ambiguous", &ResolvedProvider{Name: "p", Command: "/bin/echo", ACPCommand: "/bin/echo"}, "", "/bin/echo", ""},
		{"nil provider", nil, "", "/bin/echo acp", ""},
	}
	for _, tc := range cases {
		if got := LegacyStoredCommandTransport(tc.resolved, tc.transport, tc.stored); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestBuildMetadataLaunchCommandKeepsConfiguredDefaults(t *testing.T) {
	spec := BuiltinProviders()["claude"]
	rp := specToResolved("claude", &spec)
	want, err := BuildProviderLaunchCommand("", rp, nil, "")
	if err != nil {
		t.Fatalf("BuildProviderLaunchCommand: %v", err)
	}
	// An unknown override key or a bogus transport would make the plain
	// builder fail; the metadata builder must still produce the configured
	// defaults (including the permission flag), never a bare command.
	for _, tc := range []struct {
		name      string
		overrides map[string]string
		transport string
	}{
		{"unknown option key", map[string]string{"x": "1"}, ""},
		{"bogus transport", nil, "bogus"},
		{"both", map[string]string{"x": "1", "effort": "low"}, "bogus"},
	} {
		got, err := BuildMetadataLaunchCommand("", rp, tc.overrides, tc.transport)
		if err != nil {
			t.Fatalf("%s: BuildMetadataLaunchCommand: %v", tc.name, err)
		}
		if got.Command != want.Command {
			t.Fatalf("%s: Command = %q, want the configured default %q", tc.name, got.Command, want.Command)
		}
	}
	if got, err := BuildMetadataLaunchCommand("", rp, map[string]string{"effort": "low"}, ""); err != nil || got.Command != "claude --dangerously-skip-permissions --effort low" {
		t.Fatalf("valid override: Command = %q, err = %v", got.Command, err)
	}
}

func TestBuildMetadataLaunchCommandFailsClosed(t *testing.T) {
	// A prompt-capable provider without a permission policy fails the managed
	// policy check; the metadata builder returns the error instead of a bare
	// command.
	rp := &ResolvedProvider{
		Name:                   "claude-custom",
		Command:                "claude",
		EmitsPermissionWarning: true,
		PermissionModes:        map[string]string{"attended": "--permission-mode auto"},
		OptionsSchema:          []ProviderOption{{Key: "model", Choices: []OptionChoice{{Value: "opus", FlagArgs: []string{"--model", "claude-opus-5"}}}}},
		EffectiveDefaults:      map[string]string{"model": "opus"},
	}
	if _, err := BuildMetadataLaunchCommand("", rp, map[string]string{"x": "1"}, ""); err == nil {
		t.Fatal("BuildMetadataLaunchCommand = nil error, want the permission policy refusal")
	}
}

// ga-6umo round 4: a rejected metadata override must not trigger the resume
// schema-flag rewrite.
func TestBuildProviderResumeCommandIgnoresRejectedOverride(t *testing.T) {
	rp := &ResolvedProvider{
		Name:          "codex-custom",
		Command:       "codex",
		ResumeCommand: "codex resume {{.SessionKey}} --sandbox workspace-write",
		ResumeFlag:    "resume",
		ResumeStyle:   "subcommand",
		OptionsSchema: []ProviderOption{{Key: "permission_mode", Choices: []OptionChoice{
			{Value: "safe", FlagArgs: []string{"--sandbox", "workspace-write"}},
			{Value: "unrestricted", FlagArgs: []string{"--sandbox", "danger-full-access"}},
		}}},
	}
	got, err := BuildProviderResumeCommand(rp, map[string]string{"permission_mode": "unrestricted"})
	if err != nil {
		t.Fatalf("BuildProviderResumeCommand: %v", err)
	}
	if got != rp.ResumeCommand {
		t.Fatalf("BuildProviderResumeCommand = %q, want the configured resume command %q", got, rp.ResumeCommand)
	}
}

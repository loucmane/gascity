package session

import (
	"strings"
	"testing"
)

// ga-6umo: session keys, resume forms and the resume-pair strip.

func TestValidSessionKey(t *testing.T) {
	for _, key := range []string{"abc-123", "0343b7f8-7a64-4a2b-8edc-be0ea350cb61", "ses_01.x:y", strings.Repeat("a", 128)} {
		if !ValidSessionKey(key) {
			t.Errorf("ValidSessionKey(%q) = false, want true", key)
		}
	}
	for _, key := range []string{"", "-p", "--dangerously-skip-permissions", "a b", "a;b", "$(x)", "a`b", "a\nb", "a'b", ".hidden", strings.Repeat("a", 129)} {
		if ValidSessionKey(key) {
			t.Errorf("ValidSessionKey(%q) = true, want false", key)
		}
	}
}

func TestKnownResumeForms(t *testing.T) {
	for _, flag := range []string{"--resume", "--session", "--conversation", "resume", "threads continue"} {
		if !KnownResumeFlag(flag) {
			t.Errorf("KnownResumeFlag(%q) = false, want true", flag)
		}
	}
	for _, flag := range []string{"--settings", "--dangerously-skip-permissions", "-c", ""} {
		if KnownResumeFlag(flag) {
			t.Errorf("KnownResumeFlag(%q) = true, want false", flag)
		}
	}
	for _, style := range []string{"", "flag", "subcommand"} {
		if !KnownResumeStyle(style) {
			t.Errorf("KnownResumeStyle(%q) = false, want true", style)
		}
	}
	if KnownResumeStyle("shell") {
		t.Error("KnownResumeStyle(shell) = true, want false")
	}
}

func TestBuildResumeCommandDropsInvalidSessionKey(t *testing.T) {
	for _, info := range []Info{
		{Command: "claude", ResumeFlag: "--resume", SessionKey: "x; curl evil"},
		{Command: "claude", ResumeCommand: "claude --resume {{.SessionKey}}", ResumeFlag: "--resume", SessionKey: "--dangerously-skip-permissions"},
	} {
		got := BuildResumeCommand(info)
		if strings.Contains(got, "curl") || strings.Contains(got, "dangerously") {
			t.Errorf("BuildResumeCommand(%+v) = %q, spliced an invalid session key", info, got)
		}
	}
	if got, want := BuildResumeCommand(Info{Command: "claude", ResumeFlag: "--resume", SessionKey: "abc-123"}), "claude --resume abc-123"; got != want {
		t.Errorf("BuildResumeCommand(valid key) = %q, want %q", got, want)
	}
}

func TestStripResumeFlagPair(t *testing.T) {
	cases := []struct{ cmd, flag, want string }{
		{"claude --resume key-A --dangerously-skip-permissions", "--resume", "claude --dangerously-skip-permissions"},
		{"claude --settings '/a b.json' --resume key-A --effort max", "--resume", "claude --settings '/a b.json' --effort max"},
		{"claude --resume key-A", "--resume", "claude"},
		{"claude --dangerously-skip-permissions", "--resume", "claude --dangerously-skip-permissions"},
		{"claude --resume", "--resume", "claude --resume"},
		{"claude --resume key-A", "", "claude --resume key-A"},
	}
	for _, tc := range cases {
		if got := stripResumeFlagPair(tc.cmd, tc.flag); got != tc.want {
			t.Errorf("stripResumeFlagPair(%q, %q) = %q, want %q", tc.cmd, tc.flag, got, tc.want)
		}
	}
}

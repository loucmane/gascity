package session

import "regexp"

// ga-6umo: session keys and fork parent ids are read from worker-writable bead
// metadata and spliced into launch commands. A value outside this grammar is
// never spliced: it cannot start with a dash (so it cannot be parsed as a
// flag) and cannot contain whitespace or shell metacharacters.
var sessionKeyGrammar = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// ValidSessionKey reports whether key may be spliced into a launch command.
func ValidSessionKey(key string) bool {
	return sessionKeyGrammar.MatchString(key)
}

// knownResumeFlags are the resume flags Core's builtin provider profiles
// declare. A stale-key retry reads the resume flag from bead metadata only to
// strip a resume form out of the command; any other value could strip a
// security flag instead, so it is not honored.
var knownResumeFlags = map[string]bool{
	"--resume":         true,
	"--session":        true,
	"--conversation":   true,
	"resume":           true,
	"threads continue": true,
}

// KnownResumeFlag reports whether flag is a builtin provider resume flag.
func KnownResumeFlag(flag string) bool {
	return knownResumeFlags[flag]
}

// KnownResumeStyle reports whether style is a declared resume style.
func KnownResumeStyle(style string) bool {
	return style == "" || style == "flag" || style == "subcommand"
}

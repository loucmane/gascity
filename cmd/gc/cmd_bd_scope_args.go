package main

import (
	"strconv"
	"strings"

	"github.com/gastownhall/gascity/internal/bdflags"
)

// bdScopeProbeArgs narrows routing probes only when the entire mutation argv
// is understood. ga-z9hz: flag values can name beads in other stores. Unknown
// syntax must retain the base's raw probes, not add a new refusal before the
// existing exact-ID guard and bd's own validation.
func bdScopeProbeArgs(args []string) []string {
	values, bools := bdScopeFlagSets("")
	var subjects []string
	haveVerb := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if !haveVerb {
				return args
			}
			return append(subjects, args[i+1:]...)
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			if haveVerb {
				subjects = append(subjects, arg)
				continue
			}
			switch arg {
			case "update", "close", "reopen", "delete", "heartbeat":
				values, bools = bdScopeFlagSets(arg)
				haveVerb = true
				continue
			default:
				return args
			}
		}
		width, understood := bdScopeFlagWidth(args[i:], values, bools)
		if !understood {
			return args
		}
		i += width - 1
	}
	if !haveVerb {
		return args
	}
	return subjects
}

// bdScopeFlagWidth follows pflag's token consumption, using only short names
// for single-dash tokens. Values are left to bd to validate; boolean assignments
// must parse so malformed syntax falls back to the original routing.
func bdScopeFlagWidth(args []string, values, bools map[string]bool) (int, bool) {
	arg := args[0]
	if strings.HasPrefix(arg, "--") {
		name, value, attached := strings.Cut(arg, "=")
		if values[name] {
			if attached {
				return 1, true
			}
			return 2, len(args) > 1
		}
		if bools[name] {
			if attached {
				_, err := strconv.ParseBool(value)
				return 1, err == nil
			}
			return 1, true
		}
		return 0, false
	}
	for shorts := arg[1:]; len(shorts) > 0; {
		name := "-" + shorts[:1]
		rest := shorts[1:]
		if !values[name] && !bools[name] {
			return 0, false
		}
		// pflag only treats '=' as a separator when a value follows it.
		if len(rest) > 1 && rest[0] == '=' {
			if bools[name] {
				_, err := strconv.ParseBool(rest[1:])
				return 1, err == nil
			}
			return 1, true
		}
		if values[name] {
			if rest != "" {
				return 1, true
			}
			return 2, len(args) > 1
		}
		shorts = rest
	}
	return 1, true
}

// ga-z9hz: these pinned tables belong only to scope probing. The guard, by-ID
// funnel, work-record gate and lint have different parsing/refusal contracts
// and must keep their existing internal/bdflags manifests.
//
// Audited against cmd/bd registrations (including hidden aliases and helpers)
// at e243402ba64058a1e2861b0e9d5b84d3efb589d3. The 76-registration checklist
// in TestBdScopeManifestsCoverPinnedBDRegistrations pins coverage. Retain the
// prior compatibility superset, including Cobra help/version and --profile.
// No help discovery runs here; unrecognized syntax keeps raw-argv routing.
var bdScopeGlobalValueFlags = map[string]bool{
	"--actor": true, "--database": true, "--db": true, "-C": true,
	"--directory": true, "--dolt-auto-commit": true, "--format": true,
	"--mem-profile": true,
}

var bdScopeGlobalBoolFlags = map[string]bool{
	"--profile":     true,
	"--cpu-profile": true, "--global": true, "--ignore-schema-skew": true,
	"--json": true, "--no-color": true, "-q": true, "--quiet": true,
	"--readonly": true, "--sandbox": true, "-v": true, "--verbose": true,
	"-h": true, "--help": true, "-V": true, "--version": true,
}

var bdScopeValueFlagsBySub = map[string]map[string]bool{
	"update": {
		"--acceptance-criteria": true, "--body": true, "--description-file": true,
		"--message": true, "-m": true,
		"--acceptance": true, "--add-label": true, "--append-notes": true,
		"-a": true, "--assignee": true, "--await-id": true, "--body-file": true,
		"--defer": true, "-d": true, "--description": true, "--design": true,
		"--design-file": true, "--due": true, "-e": true, "--estimate": true,
		"--external-ref": true, "--metadata": true, "--notes": true,
		"--parent": true, "-p": true, "--priority": true, "--remove-label": true,
		"--session": true, "--set-labels": true, "--set-metadata": true,
		"-s": true, "--status": true, "-t": true, "--type": true,
		"--title": true, "--spec-id": true, "--unset-metadata": true,
		"--if-assignee": true, "--if-status": true, "--if-revision": true,
	},
	"close": {
		"--resolution": true, "--message": true, "-m": true, "--comment": true,
		"-r": true, "--reason": true, "--reason-file": true, "--session": true, "--if-revision": true,
	},
	"reopen": {
		"-r": true, "--reason": true, "--if-revision": true,
	},
	"delete": {
		"--from-file": true, "--if-revision": true,
	},
	"heartbeat": {},
}

var bdScopeBoolFlagsBySub = map[string]map[string]bool{
	"update": {
		"--allow-empty-description": true, "--ephemeral": true,
		"--force": true, "--history": true, "--no-history": true,
		"--persistent": true, "--stdin": true,
	},
	"close": {
		"--claim-next": true, "--continue": true, "-f": true, "--force": true,
		"--no-auto": true, "--suggest-next": true,
	},
	"reopen": {},
	"delete": {
		"--cascade": true, "--dry-run": true, "-f": true, "--force": true,
	},
	"heartbeat": {},
}

// bdScopeFlagSets returns fresh scope-only sets; an empty sub selects globals.
func bdScopeFlagSets(sub string) (map[string]bool, map[string]bool) {
	if sub != "" && bdScopeValueFlagsBySub[sub] == nil {
		return nil, nil
	}
	values, bools := make(map[string]bool), make(map[string]bool)
	for _, flags := range []map[string]bool{bdScopeGlobalValueFlags, bdScopeValueFlagsBySub[sub]} {
		for flag := range flags {
			values[flag] = true
		}
	}
	for _, flags := range []map[string]bool{bdScopeGlobalBoolFlags, bdScopeBoolFlagsBySub[sub]} {
		for flag := range flags {
			bools[flag] = true
		}
	}
	if sub == "update" {
		// ga-z9hz: bd's `update --claim` is boolean. TestClaimCASStaysOnTheWorkerPullPath
		// permits that literal in cmd/gc only inside ==/!= (inspection, never a
		// claim argv), so take it from the shared manifest by comparison.
		for flag := range bdflags.BoolFlags("update") {
			if flag == "--claim" {
				bools[flag] = true
			}
		}
	}
	return values, bools
}

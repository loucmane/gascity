package config

import (
	"sort"
	"strings"
)

// Launch-parameter guard (ga-6umo).
//
// Session and work beads are writable by the workers they launch, so any launch
// parameter read from bead metadata is worker-controlled. The helpers here let
// metadata select among a provider's declared choices only when the choice
// cannot widen the session: its flag args are limited to a small benign set
// (model and reasoning effort). Security-bearing choices (permission mode,
// sandbox, write roots, settings, extra directories) can only come from config.

// MetadataOverridableOptionKeys are the option keys bead or session metadata may
// override. Any other key is ignored when it comes from metadata.
var MetadataOverridableOptionKeys = []string{"model", "effort"}

// RejectedOptionOverride records one metadata option override the guard
// ignored, for events and logs.
type RejectedOptionOverride struct {
	Key    string
	Value  string
	Reason string
}

// FilterMetadataOptionOverrides returns the subset of metadata-derived option
// overrides that may be applied: keys in MetadataOverridableOptionKeys whose
// selected choice exists in the schema and carries only benign flag args.
// Keys that are not options in the schema (for example initial_message) pass
// through unchanged. The rejected list is sorted by key.
func FilterMetadataOptionOverrides(schema []ProviderOption, overrides map[string]string) (map[string]string, []RejectedOptionOverride) {
	if len(overrides) == 0 {
		return overrides, nil
	}
	allowed := make(map[string]string, len(overrides))
	var rejected []RejectedOptionOverride
	for key, value := range overrides {
		opt := findOption(schema, key)
		if opt == nil {
			// Not an option (initial_message and similar): not a launch flag.
			allowed[key] = value
			continue
		}
		if !metadataOverridableKey(key) {
			rejected = append(rejected, RejectedOptionOverride{Key: key, Value: value, Reason: "option key is not overridable from metadata"})
			continue
		}
		choice := findChoice(opt.Choices, value)
		if choice == nil {
			rejected = append(rejected, RejectedOptionOverride{Key: key, Value: value, Reason: "value is not a declared choice"})
			continue
		}
		if !BenignFlagArgs(choice.FlagArgs) {
			rejected = append(rejected, RejectedOptionOverride{Key: key, Value: value, Reason: "choice carries security-bearing flags"})
			continue
		}
		allowed[key] = value
	}
	sort.Slice(rejected, func(i, j int) bool { return rejected[i].Key < rejected[j].Key })
	return allowed, rejected
}

func metadataOverridableKey(key string) bool {
	for _, k := range MetadataOverridableOptionKeys {
		if k == key {
			return true
		}
	}
	return false
}

// BenignFlagArgs reports whether args consist only of model or reasoning-effort
// selections: "--model V" / "-m V" / "--model=V", "--effort V" / "--effort=V",
// and "-c"/"--config" whose value is exactly model=V or model_reasoning_effort=V.
// Values must be single plain tokens (no leading dash, no whitespace, no shell
// metacharacters). Anything else is security-bearing.
func BenignFlagArgs(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--model" || arg == "-m" || arg == "--effort":
			if i+1 >= len(args) || !benignValue(args[i+1]) {
				return false
			}
			i++
		case strings.HasPrefix(arg, "--model=") || strings.HasPrefix(arg, "--effort="):
			if !benignValue(arg[strings.IndexByte(arg, '=')+1:]) {
				return false
			}
		case arg == "-c" || arg == "--config":
			if i+1 >= len(args) || !benignConfigValue(args[i+1]) {
				return false
			}
			i++
		case strings.HasPrefix(arg, "--config="):
			if !benignConfigValue(strings.TrimPrefix(arg, "--config=")) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func benignConfigValue(v string) bool {
	key, value, ok := strings.Cut(v, "=")
	if !ok {
		return false
	}
	if key != "model" && key != "model_reasoning_effort" {
		return false
	}
	return benignValue(strings.Trim(value, `"`))
}

func benignValue(v string) bool {
	if v == "" || strings.HasPrefix(v, "-") {
		return false
	}
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-' || r == ':' || r == '/':
		default:
			return false
		}
	}
	return true
}

// BuildMetadataLaunchCommand builds a launch command from config for a session
// whose option overrides and transport come from worker-writable metadata
// (ga-6umo). An unknown transport means the provider default. When the
// overrides make the build fail (for example an unknown option key), it
// retries with no overrides. It never falls back to a bare command that skipped
// the option defaults or ValidateManagedLaunchPermissionPolicy: a build that
// still fails returns the error so the caller refuses the launch.
func BuildMetadataLaunchCommand(cityPath string, resolved *ResolvedProvider, overrides map[string]string, transport string) (ProviderLaunchCommand, error) {
	if !IsValidSessionTransport(transport) {
		transport = ""
	}
	cmd, err := BuildProviderLaunchCommand(cityPath, resolved, overrides, transport)
	if err == nil || len(overrides) == 0 {
		return cmd, err
	}
	if retry, retryErr := BuildProviderLaunchCommand(cityPath, resolved, nil, transport); retryErr == nil {
		return retry, nil
	}
	return ProviderLaunchCommand{}, err
}

// LegacyStoredCommandTransport returns "acp" when transport is unset and
// storedCommand is exactly the configured ACP command line of the provider and
// not also its default command line. Legacy sessions recorded no transport, so
// the stored command is the only sign they ran over ACP. It only selects
// between config-derived commands; the stored bytes are never launched
// (ga-6umo).
func LegacyStoredCommandTransport(resolved *ResolvedProvider, transport, storedCommand string) string {
	if resolved == nil || strings.TrimSpace(transport) != "" {
		return transport
	}
	if strings.TrimSpace(resolved.ACPCommand) == "" && resolved.ACPArgs == nil {
		return transport
	}
	stored := strings.TrimSpace(storedCommand)
	acp := strings.TrimSpace(resolved.ACPCommandString())
	if stored == "" || stored != acp || stored == strings.TrimSpace(resolved.CommandString()) {
		return transport
	}
	return "acp"
}

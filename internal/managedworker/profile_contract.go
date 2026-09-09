package managedworker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ProfileKind distinguishes candidate-only work from signing-capable delivery.
// The empty value retains the legacy signing contract and its exact digest.
type ProfileKind string

const (
	// ProfileKindSigning requires the existing managed signer readiness proof.
	ProfileKindSigning ProfileKind = "signing"
	// ProfileKindCandidate explicitly excludes managed signing.
	ProfileKindCandidate ProfileKind = "candidate"
	// NoSignerIdentity is the explicit candidate-only signer posture.
	NoSignerIdentity = "none"
	// ControlPolicySchemaV1 is the provider-neutral, kind-bound policy marker.
	ControlPolicySchemaV1 = "gc.worker-control-policy.v1"
	// CandidateControlPolicySchemaV1 is the reviewed candidate policy format.
	CandidateControlPolicySchemaV1 = "gc.candidate-control-policy.v1"
)

// UnmarshalJSON rejects malformed declarations rather than treating them as omission.
func (kind *ProfileKind) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("profile_kind: %w", err)
	}
	if value != string(ProfileKindSigning) && value != string(ProfileKindCandidate) {
		return fmt.Errorf("unsupported profile_kind %q", value)
	}
	*kind = ProfileKind(value)
	return nil
}

// UnmarshalJSON retains strict nested-field decoding and distinguishes a missing
// optional policy from an explicitly null declaration. Duplicate profile fields
// are ambiguous authority and are refused before the typed value is populated.
func (profile *WorkerProfile) UnmarshalJSON(data []byte) error {
	fields, err := uniqueJSONObject(data)
	if err != nil {
		return fmt.Errorf("worker profile: %w", err)
	}
	for name := range fields {
		for _, control := range []string{"control_policy", "profile_kind"} {
			if strings.EqualFold(name, control) && name != control {
				return fmt.Errorf("worker profile field %q must be spelled %q", name, control)
			}
		}
	}
	if raw, present := fields["control_policy"]; present {
		policyFields, err := uniqueJSONObject(raw)
		if err != nil {
			return fmt.Errorf("control_policy: %w", err)
		}
		for name := range policyFields {
			if name != "path" && name != "sha256" {
				return fmt.Errorf("control_policy: unknown field %q", name)
			}
		}
	}
	type wireProfile WorkerProfile
	var decoded wireProfile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*profile = WorkerProfile(decoded)
	return nil
}

func uniqueJSONObject(data []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("expected JSON object")
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := token.(string)
		if !ok {
			return nil, errors.New("expected JSON field name")
		}
		if _, exists := fields[name]; exists {
			return nil, fmt.Errorf("duplicate field %q", name)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, err
	}
	return fields, nil
}

// EffectiveProfileKind resolves only the documented omitted-field legacy default.
// A receipt must still pass validation before this value is used for authority.
func (profile WorkerProfile) EffectiveProfileKind() ProfileKind {
	if profile.ProfileKind == "" {
		return ProfileKindSigning
	}
	return profile.ProfileKind
}

func validateProfileContract(profile WorkerProfile, requireRuntime bool) error {
	if profile.ProfileKind != "" && !requireRuntime {
		return errors.New("profile_kind requires provisioning receipt v2 runtime controls")
	}
	switch profile.EffectiveProfileKind() {
	case ProfileKindCandidate:
		if profile.SignerIdentity != NoSignerIdentity {
			return errors.New("candidate signer_identity must be none")
		}
		if profile.ControlPolicy == nil {
			return errors.New("candidate control_policy is required")
		}
	case ProfileKindSigning:
		if profile.SignerIdentity == NoSignerIdentity {
			return errors.New("signing signer_identity must not be none")
		}
	default:
		return fmt.Errorf("unsupported profile_kind %q", profile.ProfileKind)
	}
	if profile.ControlPolicy == nil {
		return nil
	}
	if profile.ProfileKind == "" {
		return errors.New("control_policy requires explicit profile_kind")
	}
	if err := validateFilePin("control_policy", *profile.ControlPolicy); err != nil {
		return err
	}
	matches := 0
	for _, argument := range profile.Argv {
		if argument == profile.ControlPolicy.Path {
			matches++
		}
	}
	if matches != 1 {
		return errors.New("control_policy path must occur exactly once in bound argv")
	}
	for _, root := range profile.WritableRoots {
		relative, err := filepath.Rel(root, profile.ControlPolicy.Path)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("control_policy must be outside worker writable roots")
		}
	}
	return nil
}

// VerifyControlPolicy checks the exact receipt-bound bytes and kind marker.
// The reader must reject unsafe filesystem paths; production callers use
// ReadControlPolicy. A marker does not prove effective provider permissions.
func VerifyControlPolicy(readFile func(string) ([]byte, error), profile WorkerProfile) error {
	if err := validateProfileContract(profile, len(profile.Environment) > 0 || len(profile.Toolchains) > 0); err != nil {
		return err
	}
	pin := profile.ControlPolicy
	if pin == nil {
		return nil
	}
	if readFile == nil {
		return errors.New("control_policy safe reader is required")
	}
	data, err := readFile(pin.Path)
	if err != nil {
		return fmt.Errorf("control_policy %q: %w", pin.Path, err)
	}
	if !equalDigest(sha256Hex(data), pin.SHA256) {
		return errors.New("control_policy sha256 mismatch")
	}
	fields, err := uniqueJSONObject(data)
	if err != nil {
		return fmt.Errorf("control_policy: %w", err)
	}
	markerBytes, present := fields["_gc"]
	if !present {
		return errors.New("control_policy _gc marker is required")
	}
	if _, err := uniqueJSONObject(markerBytes); err != nil {
		return fmt.Errorf("control_policy _gc: %w", err)
	}
	var marker struct {
		Schema string      `json:"schema"`
		Kind   ProfileKind `json:"profile_kind"`
	}
	decoder := json.NewDecoder(bytes.NewReader(markerBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&marker); err != nil {
		return fmt.Errorf("control_policy _gc: %w", err)
	}
	if marker.Kind != profile.EffectiveProfileKind() {
		return errors.New("control_policy profile_kind mismatch")
	}
	if marker.Schema != ControlPolicySchemaV1 &&
		(marker.Schema != CandidateControlPolicySchemaV1 || marker.Kind != ProfileKindCandidate) {
		return fmt.Errorf("control_policy schema %q is unsupported for %q", marker.Schema, marker.Kind)
	}
	return nil
}

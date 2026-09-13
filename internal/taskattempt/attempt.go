// Package taskattempt enforces an opt-in, durable one-native-start contract on
// the driving work bead. It is independent of pool capacity and provider names.
package taskattempt

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
)

// ErrRefused marks fail-closed admission; it never authorizes a retry or refund.
var ErrRefused = errors.New("native task attempt refused")

type record struct {
	Schema    string `json:"schema"`
	StoreRef  string `json:"store_ref"`
	BeadID    string `json:"bead_id"`
	Template  string `json:"template"`
	Limit     int    `json:"limit"`
	State     string `json:"state"`
	Token     string `json:"token,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

func (r record) encode() string {
	b, _ := json.Marshal(r) // Only strings/integers: encoding cannot fail.
	return string(b)
}

func validScope(ref, id, template string) bool {
	kind, name, ok := strings.Cut(ref, ":")
	return ok && (kind == "city" || kind == "rig") && name != "" &&
		strings.TrimSpace(ref) == ref && id != "" && strings.TrimSpace(id) == id &&
		template != "" && strings.TrimSpace(template) == template
}

// Request returns the canonical operator-stamped policy value. Stamping it is
// separate from routing authority. No release/reset API exists: failed attempts
// retain this record, and an authorized successor uses a fresh task bead.
func Request(ref, id, template string) (string, error) {
	if !validScope(ref, id, template) {
		return "", fmt.Errorf("%w: incomplete task identity", ErrRefused)
	}
	return (record{Schema: "gc.native-task-attempt.v1", StoreRef: ref, BeadID: id, Template: template, Limit: 1, State: "requested"}).encode(), nil
}

func read(s beads.Store, id, ref, template string) (record, string, bool, error) {
	if s == nil {
		return record{}, "", false, fmt.Errorf("%w: work store unavailable", ErrRefused)
	}
	live := beads.HandlesFor(s).Live
	if live == nil {
		return record{}, "", false, fmt.Errorf("%w: authoritative work reader unavailable", ErrRefused)
	}
	b, err := live.Get(id)
	if err != nil {
		return record{}, "", false, fmt.Errorf("%w: read driving task: %w", ErrRefused, err)
	}
	raw, present := b.Metadata[beadmeta.NativeAttemptMetadataKey]
	if !present {
		return record{}, "", false, nil
	}
	var r record
	if len(raw) > 4096 || json.Unmarshal([]byte(raw), &r) != nil || r.encode() != raw ||
		r.Schema != "gc.native-task-attempt.v1" || r.Limit != 1 || !validScope(ref, id, template) ||
		r.StoreRef != ref || r.BeadID != id || b.ID != id || r.Template != template {
		return record{}, "", true, fmt.Errorf("%w: malformed or mismatched task policy", ErrRefused)
	}
	switch r.State {
	case "requested":
		if r.Token == "" && r.SessionID == "" {
			return r, raw, true, nil
		}
	case "reserved", "started":
		token, err := hex.DecodeString(r.Token)
		if err == nil && len(token) == 16 && ((r.State == "reserved" && r.SessionID == "") || (r.State == "started" && r.SessionID != "")) {
			return r, raw, true, nil
		}
	}
	return record{}, "", true, fmt.Errorf("%w: invalid attempt state", ErrRefused)
}

func swap(s beads.Store, id, before string, after record) error {
	w, ok := beads.MetadataCASWriterFor(s)
	if !ok {
		return fmt.Errorf("%w: atomic metadata capability unavailable", ErrRefused)
	}
	changed, err := w.CompareAndSetMetadataKey(id, beadmeta.NativeAttemptMetadataKey, before, after.encode())
	if err != nil {
		return fmt.Errorf("%w: attempt CAS (not refunded): %w", ErrRefused, err)
	}
	if !changed {
		return fmt.Errorf("%w: attempt already reserved or consumed", ErrRefused)
	}
	return nil
}

// Reserve spends the one session-creation reservation before creating a native
// session bead. Neither failed creation nor a scheduler restart refunds it.
func Reserve(s beads.Store, id, ref, template string) (string, error) {
	r, raw, present, err := read(s, id, ref, template)
	if err != nil || !present {
		return "", err
	}
	if r.State != "requested" {
		return "", fmt.Errorf("%w: task already reserved", ErrRefused)
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("%w: reservation token: %w", ErrRefused, err)
	}
	r.State, r.Token = "reserved", hex.EncodeToString(nonce[:])
	if err := swap(s, id, raw, r); err != nil {
		return "", err
	}
	return r.Token, nil
}

// Start authorizes exactly one provider.Start invocation, not one concurrently
// active session. An already-started record never returns idempotent success,
// even for the same session. An ambiguous CAS fails closed and is not refunded.
func Start(s beads.Store, id, ref, template, token, sessionID string) error {
	r, raw, present, err := read(s, id, ref, template)
	if err != nil {
		return err
	}
	if !present && token == "" {
		return nil
	}
	if !present || r.State != "reserved" || token == "" || r.Token != token || sessionID == "" {
		return fmt.Errorf("%w: missing reservation or attempt already started", ErrRefused)
	}
	r.State, r.SessionID = "started", sessionID
	return swap(s, id, raw, r)
}

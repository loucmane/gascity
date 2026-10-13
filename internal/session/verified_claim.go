package session

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
)

var (
	// ErrStaleSession means an operation no longer belongs to this incarnation.
	ErrStaleSession = errors.New("session incarnation no longer matches")
	// ErrClaimPrecondition means another invocation changed the current claim.
	ErrClaimPrecondition = errors.New("current claim prior value no longer matches")
)

// VerifiedSession binds a worker operation to one persisted incarnation. It
// contains a digest, never the runtime token. Only VerifySession constructs it.
// Routing is frozen too: an identity change cannot retarget an in-flight call.
type VerifiedSession struct {
	ID                      string
	Generation              int
	InstanceTokenSHA256     string
	SessionName             string
	Template                string
	AgentName               string
	Alias                   string
	ConfiguredNamedIdentity string
	Origin                  string
	CurrentClaim            string
}

func tokenDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func verifiedSessionFromBead(b beads.Bead) VerifiedSession {
	generation, _ := strconv.Atoi(b.Metadata["generation"])
	return VerifiedSession{
		ID: b.ID, Generation: generation,
		InstanceTokenSHA256: tokenDigest(b.Metadata["instance_token"]),
		SessionName:         b.Metadata["session_name"], Template: b.Metadata["template"],
		AgentName: b.Metadata["agent_name"], Alias: b.Metadata["alias"],
		ConfiguredNamedIdentity: b.Metadata["configured_named_identity"],
		Origin:                  b.Metadata["session_origin"],
		CurrentClaim:            b.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey],
	}
}

// VerifySession reads exactly id. It performs no resolver/list fallback and
// refuses closed, damaged, and superseded rows. Draining is permitted so an
// authenticated worker can acknowledge its own drain; it cannot stamp new work.
func (s *Store) VerifySession(id, token string, generation int) (VerifiedSession, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(token) == "" || generation <= 0 {
		return VerifiedSession{}, ErrStaleSession
	}
	b, err := s.validatedBead(id)
	if err != nil {
		return VerifiedSession{}, err
	}
	v := verifiedSessionFromBead(b)
	if v.ID != id || v.Generation != generation || v.InstanceTokenSHA256 != tokenDigest(token) {
		return VerifiedSession{}, ErrStaleSession
	}
	if err := validateVerifiedSession(b, v, false); err != nil {
		return VerifiedSession{}, err
	}
	return v, nil
}

func validateVerifiedSession(b beads.Bead, v VerifiedSession, claiming bool) error {
	actual := verifiedSessionFromBead(b)
	actual.CurrentClaim = v.CurrentClaim // the claim is guarded separately
	if v.ID == "" || v.Generation <= 0 || b.Status != "open" || b.Type != BeadType || actual != v {
		return ErrStaleSession
	}
	p := SessionFenceProjection{State: b.Metadata["state"]}
	if !p.ClaimEligible() && (claiming || p.State != string(StateDraining)) {
		return ErrStaleSession
	}
	return nil
}

func (s *Store) verifiedBead(v VerifiedSession, claiming bool) (beads.Bead, error) {
	b, err := s.validatedBead(v.ID)
	if err != nil {
		if errors.Is(err, beads.ErrNotFound) || errors.Is(err, ErrSessionNotFound) {
			err = errors.Join(ErrStaleSession, err)
		}
		return beads.Bead{}, fmt.Errorf("verified session read: %w", err)
	}
	if err := validateVerifiedSession(b, v, claiming); err != nil {
		return beads.Bead{}, err
	}
	return b, nil
}

// ReadVerified does one exact Get and returns a redacted persisted snapshot.
func (s *Store) ReadVerified(v VerifiedSession) (Info, error) {
	b, err := s.verifiedBead(v, false)
	if err != nil {
		return Info{}, err
	}
	info := infoFromPersistedBead(b)
	info.InstanceToken = ""
	return info, nil
}

// CurrentClaimVerified reads the claim only while v still names this incarnation.
func (s *Store) CurrentClaimVerified(v VerifiedSession) (string, error) {
	b, err := s.verifiedBead(v, false)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(b.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey]), nil
}

// CompareAndSetCurrentClaim is the managed worker's only session write. All
// identity, lifecycle and prior-value predicates are evaluated on the SAME row
// revision passed to UpdateIfMatch. A conflict retries the complete predicate,
// never just a metadata merge. Unsupported backends fail closed.
func (s *Store) CompareAndSetCurrentClaim(v VerifiedSession, expected, next string) error {
	writer, ok := beads.RequiredConditionalWriterFor(s.store.Store)
	if !ok {
		return beads.ErrConditionalWriteUnsupported
	}
	for attempt := 0; attempt < 4; attempt++ {
		b, err := s.verifiedBead(v, next != "")
		if err != nil {
			return err
		}
		if b.Metadata[beadmeta.CurrentClaimBeadIDMetadataKey] != expected {
			return ErrClaimPrecondition
		}
		if expected == next {
			return nil
		}
		err = writer.UpdateIfMatch(v.ID, b.Revision, beads.UpdateOpts{Metadata: map[string]string{
			beadmeta.CurrentClaimBeadIDMetadataKey: next,
		}})
		if !beads.IsPreconditionFailed(err) {
			return err
		}
	}
	return fmt.Errorf("current claim contention: %w", ErrClaimPrecondition)
}

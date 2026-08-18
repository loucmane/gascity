package session

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/citylayout"
	"github.com/gastownhall/gascity/internal/fsys"
)

const (
	sessionFenceProjectionSchemaVersion  = 1
	sessionFenceProjectionMaxBytes       = 64 << 10
	sessionFenceProjectionStateTombstone = "tombstoned"
)

// SessionFenceProjection is the controller-authored, worker-readable identity
// record used to fence hook claims without giving a sandboxed worker access to
// the city task store. InstanceTokenSHA256 is a digest, never the raw token.
type SessionFenceProjection struct {
	SchemaVersion       int    `json:"schema_version"`
	SessionID           string `json:"session_id"`
	InstanceTokenSHA256 string `json:"instance_token_sha256"`
	Generation          int    `json:"generation"`
	State               string `json:"state"`
	ProjectedAt         string `json:"projected_at"`
}

// MatchesInstanceToken hashes token and compares its digest with the projected
// digest in constant time.
func (p SessionFenceProjection) MatchesInstanceToken(token string) bool {
	want, err := hex.DecodeString(p.InstanceTokenSHA256)
	if err != nil || len(want) != sha256.Size {
		return false
	}
	got := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(want, got[:]) == 1
}

// ClaimEligible reports whether the projection state admits a claim from a
// live runtime. Empty is retained for legacy active sessions.
func (p SessionFenceProjection) ClaimEligible() bool {
	switch State(p.State) {
	case StateNone, StateActive, StateAwake, StateCreating, StateStartPending:
		return true
	default:
		return false
	}
}

// LoadSessionFenceProjection reads and validates one controller projection.
// The final path must be a regular file and is opened without following a
// symlink. Files above the fixed size bound and JSON with unknown fields are
// rejected.
func LoadSessionFenceProjection(cityPath, sessionID string) (SessionFenceProjection, error) {
	path, err := sessionFenceProjectionPath(cityPath, sessionID)
	if err != nil {
		return SessionFenceProjection{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SessionFenceProjection{}, fmt.Errorf("session-fence projection not found: %w", err)
		}
		return SessionFenceProjection{}, fmt.Errorf("stat session-fence projection: %w", err)
	}
	if !info.Mode().IsRegular() {
		return SessionFenceProjection{}, fmt.Errorf("session-fence projection is not a regular file")
	}
	if info.Size() > sessionFenceProjectionMaxBytes {
		return SessionFenceProjection{}, fmt.Errorf("session-fence projection exceeds %d bytes", sessionFenceProjectionMaxBytes)
	}
	data, err := (fsys.OSFS{}).ReadRegularFile(path)
	if err != nil {
		return SessionFenceProjection{}, fmt.Errorf("read session-fence projection: %w", err)
	}
	if len(data) > sessionFenceProjectionMaxBytes {
		return SessionFenceProjection{}, fmt.Errorf("session-fence projection exceeds %d bytes", sessionFenceProjectionMaxBytes)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var projection SessionFenceProjection
	if err := decoder.Decode(&projection); err != nil {
		return SessionFenceProjection{}, fmt.Errorf("decode session-fence projection: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = fmt.Errorf("unexpected trailing JSON value")
		}
		return SessionFenceProjection{}, fmt.Errorf("decode session-fence projection: %w", err)
	}
	if err := validateSessionFenceProjection(projection); err != nil {
		return SessionFenceProjection{}, err
	}
	return projection, nil
}

func validateSessionFenceProjection(projection SessionFenceProjection) error {
	if projection.SchemaVersion != sessionFenceProjectionSchemaVersion {
		return fmt.Errorf("unsupported session-fence projection schema version %d", projection.SchemaVersion)
	}
	if strings.TrimSpace(projection.SessionID) == "" || projection.SessionID != strings.TrimSpace(projection.SessionID) {
		return fmt.Errorf("invalid session-fence projection session id")
	}
	digest, err := hex.DecodeString(projection.InstanceTokenSHA256)
	if err != nil || len(digest) != sha256.Size {
		return fmt.Errorf("invalid session-fence projection token digest")
	}
	if projection.Generation <= 0 {
		return fmt.Errorf("invalid session-fence projection generation %d", projection.Generation)
	}
	switch State(projection.State) {
	case StateNone, StateActive, StateAwake, StateCreating, StateStartPending,
		StateAsleep, StateSuspended, StateFailedCreate, StateDraining, StateDrained,
		StateArchived, StateQuarantined, StateClosed, State(sessionFenceProjectionStateTombstone):
	default:
		return fmt.Errorf("invalid session-fence projection state %q", projection.State)
	}
	if _, err := time.Parse(time.RFC3339Nano, projection.ProjectedAt); err != nil {
		return fmt.Errorf("invalid session-fence projection timestamp: %w", err)
	}
	return nil
}

func sessionFenceProjectionPath(cityPath, sessionID string) (string, error) {
	cityPath = strings.TrimSpace(cityPath)
	sessionID = strings.TrimSpace(sessionID)
	if cityPath == "" {
		return "", fmt.Errorf("city path is empty")
	}
	if sessionID == "" || filepath.Base(sessionID) != sessionID || sessionID == "." || sessionID == ".." {
		return "", fmt.Errorf("invalid session id %q for session-fence projection", sessionID)
	}
	return citylayout.RuntimePath(cityPath, "runtime", "session-fence", sessionID+".json"), nil
}

func writeSessionFenceProjection(cityPath, sessionID, token string, generation int, state State, now time.Time) error {
	if strings.TrimSpace(cityPath) == "" {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	path, err := sessionFenceProjectionPath(cityPath, sessionID)
	if err != nil {
		return err
	}
	if generation <= 0 {
		generation = DefaultGeneration
	}
	digest := sha256.Sum256([]byte(token))
	projection := SessionFenceProjection{
		SchemaVersion:       sessionFenceProjectionSchemaVersion,
		SessionID:           sessionID,
		InstanceTokenSHA256: hex.EncodeToString(digest[:]),
		Generation:          generation,
		State:               string(state),
		ProjectedAt:         now.UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(projection)
	if err != nil {
		return fmt.Errorf("marshal session-fence projection: %w", err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create session-fence projection directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create session-fence projection temp file: %w", err)
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := temp.Chmod(0o644); err != nil {
		_ = temp.Close()
		return fmt.Errorf("chmod session-fence projection temp file: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write session-fence projection temp file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync session-fence projection temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close session-fence projection temp file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename session-fence projection: %w", err)
	}
	directory, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open session-fence projection directory for sync: %w", err)
	}
	defer func() { _ = directory.Close() }()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync session-fence projection directory: %w", err)
	}
	return nil
}

func (m *Manager) publishSessionFenceProjection(id, token string, generation int, state State) error {
	return writeSessionFenceProjection(m.cityPath, id, token, generation, state, m.now())
}

func (m *Manager) tombstoneSessionFenceProjection(id, token string, generation int) error {
	return m.publishSessionFenceProjection(id, token, generation, State(sessionFenceProjectionStateTombstone))
}

// TombstoneSessionFenceProjection invalidates a controller-owned projection
// before a controller path outside Manager replaces a token or tears down a
// runtime. An empty city path is a compatibility no-op for isolated SDK use.
func TombstoneSessionFenceProjection(cityPath, sessionID, token string, generation int) error {
	return writeSessionFenceProjection(cityPath, sessionID, token, generation, State(sessionFenceProjectionStateTombstone), time.Now())
}

// PublishLiveSessionFenceProjection publishes the current controller-owned
// identity for a runtime the reconciler has observed alive. Callers must hold
// the session mutation lock so a lifecycle tombstone cannot race this refresh.
// Empty city paths remain a compatibility no-op for isolated SDK use.
func PublishLiveSessionFenceProjection(cityPath string, info Info) error {
	if strings.TrimSpace(cityPath) == "" {
		return nil
	}
	if strings.TrimSpace(info.ID) == "" {
		return fmt.Errorf("live session has no id for claim-fence projection")
	}
	if strings.TrimSpace(info.InstanceToken) == "" {
		return fmt.Errorf("live session %q has no instance token for claim-fence projection", info.ID)
	}
	state := info.State
	if projection := (SessionFenceProjection{State: string(state)}); !projection.ClaimEligible() {
		// A surviving runtime can be paired with stale asleep metadata until the
		// reconciler's later heal commits active. The live observation is enough to
		// bridge that one recovery state; explicit blockers remain fail-closed.
		if state != StateAsleep {
			return fmt.Errorf("live session %q state %q is not claim-eligible", info.ID, state)
		}
		state = StateActive
	}
	generation := sessionFenceGeneration(info.Generation)
	if current, err := LoadSessionFenceProjection(cityPath, info.ID); err == nil &&
		current.SessionID == info.ID &&
		current.Generation == generation &&
		current.MatchesInstanceToken(info.InstanceToken) {
		if current.State == sessionFenceProjectionStateTombstone {
			return fmt.Errorf("live session %q claim-fence projection is tombstoned", info.ID)
		}
		if current.State == string(state) {
			return nil
		}
	}
	return writeSessionFenceProjection(
		cityPath,
		info.ID,
		info.InstanceToken,
		generation,
		state,
		time.Now(),
	)
}

func sessionFenceGeneration(raw string) int {
	generation, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || generation <= 0 {
		return DefaultGeneration
	}
	return generation
}

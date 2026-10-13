package beads

import (
	"fmt"
	"strings"
)

// AssignIfUnassigned pins an open continuation without overwriting a claim
// that won since discovery. bd 1.3.1 supports these two atomic predicates even
// though it does not expose its library's general row-version CAS on the CLI.
func (s *BdStore) AssignIfUnassigned(id, assignee string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(assignee) == "" {
		return fmt.Errorf("continuation id and assignee are required")
	}
	if err := s.guardRelocatedClassIDs("assign-if-unassigned "+id, id); err != nil {
		return err
	}
	if err := s.releaseIDCollision(id); err != nil {
		return err
	}
	out, err := s.runBDTransientWriteOutput("update", "--json", id, "--assignee", assignee, "--if-assignee", "", "--if-status", "open")
	if err == nil {
		return nil
	}
	if bdExitCode(err) == bdCASPreconditionExitCode {
		return &PreconditionFailedError{ID: id}
	}
	detail := string(out) + " " + err.Error()
	if isBdUnknownFlagError(detail, "--if-assignee") || isBdUnknownFlagError(detail, "--if-status") {
		return ErrConditionalWriteUnsupported
	}
	return fmt.Errorf("assign-if-unassigned: %w", err)
}

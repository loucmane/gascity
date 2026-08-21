package platforminstall

import "errors"

// PlanStep is one deterministic, ordered preflight or mutation in an install.
type PlanStep struct {
	Order   int
	Action  string
	Path    string
	SHA256  string
	Mutates bool
}

// ErrPlanDisabled keeps dry-run fail-closed in the RED contract.
var ErrPlanDisabled = errors.New("platform install planning is disabled")

// Plan validates a manifest and returns the exact ordered install plan without mutation.
func Plan(_ Manifest) ([]PlanStep, error) {
	return nil, ErrPlanDisabled
}

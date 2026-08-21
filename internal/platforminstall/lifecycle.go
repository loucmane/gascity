package platforminstall

import (
	"context"
	"errors"
)

var errActivationDisabled = errors.New("platform activation is disabled")

// Lifecycle is the supervisor transition boundary used by Apply. Production
// supplies the Gas City supervisor implementation; tests use a deterministic
// fake and never touch a live process.
type Lifecycle interface {
	Restart(context.Context, Manifest) error
	Verify(context.Context, Manifest) (RuntimeProof, error)
}

// Apply installs the manifest and completes its explicitly authorized runtime
// activation. This RED scaffold is replaced by the transactional implementation.
func Apply(context.Context, Manifest, Lifecycle) (Receipt, error) {
	return Receipt{}, errActivationDisabled
}

// Rollback restores the exact pre-install filesystem state from manifest-bound
// backups. This RED scaffold is replaced by the transactional implementation.
func Rollback(Manifest) error {
	return errActivationDisabled
}

package platforminstall

import (
	"fmt"
	"math"
	"time"
)

const (
	metadataGrantBudget   = 25 * time.Second
	metadataLeaseBudget   = 28 * time.Second
	metadataExpiryGap     = metadataLeaseBudget - metadataGrantBudget
	metadataReturnReserve = 2 * time.Second
)

func metadataLeaseWindow(epoch, now int64, remaining time.Duration, observation bool) (int64, int64, error) {
	if now <= 0 || now > math.MaxInt64-int64(metadataLeaseBudget) {
		return 0, 0, fmt.Errorf("invalid metadata lease clock")
	}
	if observation {
		return now + int64(metadataGrantBudget), now + int64(metadataLeaseBudget), nil
	}
	if epoch <= 0 || epoch > now || epoch > math.MaxInt64-int64(metadataLeaseBudget) {
		return 0, 0, fmt.Errorf("invalid metadata transaction epoch")
	}
	if remaining <= metadataExpiryGap+metadataReturnReserve {
		return 0, 0, fmt.Errorf("insufficient metadata transaction lifetime")
	}
	// Host validation spends the same budget as publication and terminal proof.
	// An earlier caller deadline can shorten, but never renew, either grant.
	leaseEnd := epoch + int64(metadataLeaseBudget)
	contextLeaseRemaining := remaining - metadataReturnReserve
	if contextLeaseRemaining < time.Duration(leaseEnd-now) {
		leaseEnd = now + int64(contextLeaseRemaining)
	}
	deadline := leaseEnd - int64(metadataExpiryGap)
	if deadline <= now {
		return 0, 0, fmt.Errorf("metadata transaction budget exhausted before lease")
	}
	return deadline, leaseEnd, nil
}

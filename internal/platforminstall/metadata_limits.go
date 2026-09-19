package platforminstall

import "time"

// One finite policy governs both supervisor admission and transaction clients.
// Full host/input validation consumes the original grant; it never starts a
// fresh mutation window. Keep expiry and return reserves outside that grant.
// Increasing these bounds requires source review and a new image/adoption, not
// a caller-supplied timeout or an extension of an already-issued lease.
const (
	metadataGrantBudget       = 60 * time.Second
	metadataExpiryGap         = 3 * time.Second
	metadataLeaseBudget       = metadataGrantBudget + metadataExpiryGap
	metadataReturnReserve     = 2 * time.Second
	metadataTransactionBudget = metadataLeaseBudget + metadataReturnReserve
	metadataPipeBudget        = metadataGrantBudget
)

package runtime

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
)

// DrainAckIncarnationPrefix identifies an acknowledgement bound to one runtime
// incarnation. The value carries a generation and a token digest, never a token.
const DrainAckIncarnationPrefix = "incarnation:"

// DrainAckMatchesIncarnation rejects absent, malformed and stale addressed
// acknowledgements. Callers supply either the session row or runtime identity.
func DrainAckMatchesIncarnation(value, generation, token string) bool {
	n, err := strconv.Atoi(generation)
	return err == nil && n > 0 && token != "" &&
		value == fmt.Sprintf("%s%d:%x", DrainAckIncarnationPrefix, n, sha256.Sum256([]byte(token)))
}

// DrainAckMatchesRuntime retains legacy bare acknowledgements and checks an
// addressed value against the runtime's own current metadata. Metadata reads
// are not a transaction; consumers that also have a session row must check it.
func DrainAckMatchesRuntime(meta MetaStore, name, value string) (bool, error) {
	if !strings.HasPrefix(value, DrainAckIncarnationPrefix) {
		return value == "1", nil
	}
	token, err := meta.GetMeta(name, "GC_INSTANCE_TOKEN")
	if err != nil {
		return false, err
	}
	generation, err := meta.GetMeta(name, "GC_RUNTIME_EPOCH")
	if err != nil {
		return false, err
	}
	return DrainAckMatchesIncarnation(value, generation, token), nil
}

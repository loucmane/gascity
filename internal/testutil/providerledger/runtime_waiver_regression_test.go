package providerledger

import (
	"strings"
	"testing"
	"time"
)

// The retired constructor helper remains a negative policy fixture, not an
// active catalog disposition. Its owner and forcing-function date stay exact.
func TestRetiredRuntimeWaiverKeepsExactExpiryAndNoGrace(t *testing.T) {
	deadline := time.Date(2026, time.September, 2, 0, 0, 0, 0, time.UTC)
	claim := waivedRuntime(repoSymbol("internal/runtime", "NewFake"), "preserved policy regression")
	if claim.Waiver == nil || claim.Waiver.Owner != "ga-80po0c.3" || !claim.Waiver.Expires.Equal(deadline) {
		t.Fatalf("retired waiver binding changed: %+v", claim.Waiver)
	}
	entry := validRuntimeEntry("runtime.retired-waiver", "exact:retired-waiver", claim)
	if err := Validate([]Entry{entry}, deadline.Add(-time.Nanosecond)); err != nil {
		t.Fatalf("just-before-expiry positive control: %v", err)
	}
	for _, now := range []time.Time{deadline, deadline.Add(time.Nanosecond), deadline.Add(7 * 24 * time.Hour)} {
		if err := Validate([]Entry{entry}, now); err == nil || !strings.Contains(err.Error(), "waiver owned by ga-80po0c.3 expired") {
			t.Fatalf("expired waiver at %s must refuse without grace: %v", now, err)
		}
	}
	for _, entry := range Catalog() {
		for _, current := range entry.Claims {
			if current.Disposition == DispositionWaived || current.Waiver != nil {
				t.Fatalf("active catalog waiver reintroduced for %s", entry.ID)
			}
		}
	}
}

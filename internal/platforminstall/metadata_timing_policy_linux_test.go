package platforminstall

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMetadataTimingPolicySlowValidationDoesNotRenew(t *testing.T) {
	const epoch int64 = 100_000_000_000
	for _, elapsed := range []time.Duration{0, 14 * time.Second, 29 * time.Second, 59 * time.Second} {
		deadline, leaseEnd, err := metadataLeaseWindow(epoch, epoch+int64(elapsed), 65*time.Second-elapsed, false)
		if err != nil || deadline != epoch+int64(60*time.Second) || leaseEnd != epoch+int64(63*time.Second) {
			t.Fatalf("elapsed %s: deadline=%d lease=%d err=%v", elapsed, deadline, leaseEnd, err)
		}
	}
	// A generous caller cannot renew the expired original transaction.
	if _, _, err := metadataLeaseWindow(epoch, epoch+int64(60*time.Second), time.Hour, false); err == nil {
		t.Fatal("expired original grant renewed")
	}
}

func TestMetadataTimingPolicySupervisorAdmissionIsFinite(t *testing.T) {
	const now int64 = 100_000_000_000
	request := MetadataLeaseRequest{Transaction: strings.Repeat("b", 64), RequestSHA256: strings.Repeat("c", 64), Nonce: strings.Repeat("d", 64)}
	for _, duration := range []time.Duration{63 * time.Second, 65 * time.Second, 65*time.Second + 1} {
		request.Deadline = now + int64(duration)
		lease := newMetadataLease(strings.Repeat("a", 64), func() (int64, error) { return now, nil })
		_, err := lease.Begin(request)
		if (err == nil) != (duration <= 65*time.Second) {
			t.Fatalf("duration %s: err=%v", duration, err)
		}
		if err == nil {
			if _, err := lease.Begin(request); err == nil {
				t.Fatal("extended admission permitted renewal")
			}
		}
	}
}

func TestMetadataTimingPolicyNaturalExpiryUsesOriginalReserve(t *testing.T) {
	const epoch int64 = 100_000_000_000
	now := epoch + int64(29*time.Second)
	_, leaseEnd, err := metadataLeaseWindow(epoch, now, 36*time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	err = metadataWaitLeaseExpiry(context.Background(), leaseEnd, func() (int64, error) {
		return now, nil
	}, func(_ context.Context, delay time.Duration) error {
		now += int64(delay)
		return nil
	})
	if err != nil || now != epoch+int64(63*time.Second) || epoch+int64(65*time.Second)-now != int64(2*time.Second) {
		t.Fatalf("expiry/reserve mismatch: now=%d err=%v", now, err)
	}
}

func TestMetadataTimingPolicyRejectsUnboundedExpiryWait(t *testing.T) {
	const now int64 = 100_000_000_000
	err := metadataWaitLeaseExpiry(context.Background(), now+int64(65*time.Second)+1, func() (int64, error) {
		return now, nil
	}, func(context.Context, time.Duration) error {
		t.Fatal("unbounded expiry reached wait")
		return nil
	})
	if err == nil {
		t.Fatal("unbounded expiry accepted")
	}
}

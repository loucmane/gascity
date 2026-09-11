package platforminstall

import (
	"errors"
	"strings"
	"testing"
)

func TestMetadataLeaseIsExactExclusiveAndNonrenewing(t *testing.T) {
	now := int64(100)
	lease := newMetadataLease(strings.Repeat("a", 64), func() (int64, error) { return now, nil })
	request := MetadataLeaseRequest{Transaction: strings.Repeat("b", 64), RequestSHA256: strings.Repeat("c", 64), Nonce: strings.Repeat("d", 64), Deadline: 1000}
	proof, err := lease.Begin(request)
	if err != nil {
		t.Fatal(err)
	}
	if proof.Request != request || proof.Instance != strings.Repeat("a", 64) || proof.Observed != now {
		t.Fatalf("wrong lease binding: %+v", proof)
	}
	if _, err := lease.Begin(request); err == nil {
		t.Fatal("renewed occupied lease")
	}
	if _, err := lease.Check(MetadataLeaseCheck{Request: request, Instance: proof.Instance}); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"transaction", "request", "nonce", "deadline", "instance"} {
		check := MetadataLeaseCheck{Request: request, Instance: proof.Instance}
		switch field {
		case "transaction":
			check.Request.Transaction = strings.Repeat("e", 64)
		case "request":
			check.Request.RequestSHA256 = strings.Repeat("e", 64)
		case "nonce":
			check.Request.Nonce = strings.Repeat("e", 64)
		case "deadline":
			check.Request.Deadline++
		case "instance":
			check.Instance = strings.Repeat("e", 64)
		}
		if _, err := lease.Check(check); err == nil {
			t.Fatalf("accepted changed %s", field)
		}
	}
	now = request.Deadline
	if _, err := lease.Check(MetadataLeaseCheck{Request: request, Instance: proof.Instance}); err == nil {
		t.Fatal("accepted expired lease")
	}
	if _, err := lease.Begin(request); err == nil {
		t.Fatal("replayed expired request")
	}
	request.Deadline++
	if _, err := lease.Begin(request); err != nil {
		t.Fatal(err)
	}
}

func TestMetadataLeaseRefusesUnavailableClockAndInvalidBounds(t *testing.T) {
	request := MetadataLeaseRequest{Transaction: strings.Repeat("b", 64), RequestSHA256: strings.Repeat("c", 64), Nonce: strings.Repeat("d", 64), Deadline: 1000}
	clockErr := errors.New("clock unavailable")
	lease := newMetadataLease(strings.Repeat("a", 64), func() (int64, error) { return 0, clockErr })
	if _, err := lease.Begin(request); !errors.Is(err, clockErr) {
		t.Fatalf("clock error = %v", err)
	}
	for _, deadline := range []int64{-1, 0, 100, 30_000_000_101} {
		lease := newMetadataLease(strings.Repeat("a", 64), func() (int64, error) { return 100, nil })
		request.Deadline = deadline
		if _, err := lease.Begin(request); err == nil {
			t.Fatalf("accepted deadline %d", deadline)
		}
	}
}

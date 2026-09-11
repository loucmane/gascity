package platforminstall

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMetadataObservationCannotAuthorizeWriterOrReceipt(t *testing.T) {
	for _, plan := range []bool{true, false} {
		request := metadataRequest{PlanOnly: plan, Binding: MetadataBinding{Observation: plan}}
		if err := metadataRequestPurpose(request); err != nil {
			t.Fatal(err)
		}
		request.Binding.Observation = !plan
		if err := metadataRequestPurpose(request); err == nil {
			t.Fatal("channel purpose drift accepted")
		}
	}
	manifest := metadataContractFixture(t)
	manifest.ManifestSHA256 = strings.Repeat("e", 64)
	receipt := Receipt{Schema: metadataReceiptSchema, ManifestSHA256: manifest.ManifestSHA256, ReleaseID: manifest.ReleaseID, ArtifactSHA256: manifest.Core.SHA256, PreviousSHA256: manifest.PreviousSHA256, Activation: &RuntimeProof{manifest.Core.SHA256, manifest.Activation.ExpectedCommit, manifest.Activation.ExpectedVersion}, Metadata: &MetadataCommit{Host: manifest.Metadata.Host, Binding: MetadataBinding{Request: manifest.ManifestSHA256, Transaction: manifest.Metadata.Transaction, Attempt: manifest.Metadata.Attempt}}}
	if !receiptMatchesManifest(receipt, manifest) {
		t.Fatal("valid mutation receipt refused")
	}
	receipt.Metadata.Binding.Observation = true
	if receiptMatchesManifest(receipt, manifest) {
		t.Fatal("observation accepted as mutation receipt")
	}
}

func TestMetadataMutationWaitsForExpiryWithoutRenewal(t *testing.T) {
	now := int64(100)
	waits := 0
	err := metadataWaitLeaseExpiry(context.Background(), 1000, func() (int64, error) { return now, nil }, func(_ context.Context, duration time.Duration) error {
		waits++
		if duration != 900 {
			t.Fatalf("unexpected delay %s", duration)
		}
		now += int64(duration)
		return nil
	})
	if err != nil || waits != 1 || now != 1000 {
		t.Fatal("expiry wait failed", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := metadataWaitLeaseExpiry(ctx, 1000, func() (int64, error) { t.Fatal("read clock after cancellation"); return 0, nil }, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	clockErr := errors.New("clock unavailable")
	if err := metadataWaitLeaseExpiry(context.Background(), 1000, func() (int64, error) { return 0, clockErr }, nil); !errors.Is(err, clockErr) {
		t.Fatal(err)
	}
	waitErr := errors.New("wait interrupted")
	if err := metadataWaitLeaseExpiry(context.Background(), 1000, func() (int64, error) { return 100, nil }, func(context.Context, time.Duration) error { return waitErr }); !errors.Is(err, waitErr) {
		t.Fatal(err)
	}
}

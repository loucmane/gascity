package platforminstall

import (
	"context"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMetadataMutationBudgetWiring(t *testing.T) {
	data, err := os.ReadFile("metadata_transaction_linux.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	start := strings.Index(source, "func RunMetadataTransaction(")
	if start < 0 {
		t.Fatal("transaction entry point absent")
	}
	body := source[start:]
	prior := -1
	for _, step := range []string{"epoch, err := metadataMonotonicNow()", "context.WithTimeout(ctx, metadataTransactionBudget)", "metadataSandboxArgv(manifest)", "openMetadataHost(ctx, manifest)", "session.begin(ctx, planOnly, epoch)", "command.Start()"} {
		position := strings.Index(body, step)
		if position <= prior {
			t.Fatalf("transaction budget wiring absent or out of order: %s", step)
		}
		prior = position
	}
	if strings.Count(source, "time.Now().Add(metadataPipeBudget)") != 2 || strings.Count(source, "> int64(metadataTransactionBudget)") != 2 {
		t.Fatal("writer/parent pipes and writer/expiry admission must use the shared policy")
	}
	data, err = os.ReadFile("metadata_host_linux.go")
	if err != nil {
		t.Fatal(err)
	}
	source = string(data)
	begin := strings.Index(source, "func (session *metadataHostSession) begin(")
	end := strings.Index(source[begin:], "// VerifyMetadataRuntime")
	body = source[begin : begin+end]
	prior = -1
	for _, step := range []string{"lease cannot be renewed", "metadataLeaseWindow(epoch, start, time.Until(contextDeadline), observation)", "session.binding = MetadataBinding", "session.exchange(ctx, \"begin\""} {
		position := strings.Index(body, step)
		if position <= prior {
			t.Fatalf("lease admission must precede binding and exchange: %s", step)
		}
		prior = position
	}
}

func TestMetadataLeaseBudgetRefusesBeforeBindingOrExchange(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	epoch, err := metadataMonotonicNow()
	if err != nil {
		t.Fatal(err)
	}
	session := &metadataHostSession{}
	if err := session.begin(ctx, false, epoch); err == nil {
		t.Fatal("short enclosing lifetime reached lease mutation")
	}
	if session.binding != (MetadataBinding{}) || session.connection != nil {
		t.Fatal("refused budget established lease state")
	}
}

func TestMetadataLeaseWindowUsesTransactionEpoch(t *testing.T) {
	const epoch int64 = 100_000_000_000
	now := epoch + int64(4*time.Second)
	deadline, leaseEnd, err := metadataLeaseWindow(epoch, now, 61*time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	if deadline != epoch+int64(60*time.Second) || leaseEnd != epoch+int64(63*time.Second) {
		t.Fatalf("slow validation renewed budget: deadline=%d lease=%d", deadline, leaseEnd)
	}
	if leaseEnd+int64(2*time.Second) > now+int64(61*time.Second) {
		t.Fatal("post-FINAL expiry cannot fit the outer context")
	}
}

func TestMetadataLeaseWindowEarlierParentShortensBothDeadlines(t *testing.T) {
	const epoch int64 = 100_000_000_000
	now := epoch + int64(4*time.Second)
	deadline, leaseEnd, err := metadataLeaseWindow(epoch, now, 12*time.Second, false)
	if err != nil || deadline != now+int64(7*time.Second) || leaseEnd != now+int64(10*time.Second) {
		t.Fatalf("parent lifetime ignored: deadline=%d lease=%d err=%v", deadline, leaseEnd, err)
	}
}

func TestMetadataLeaseWindowRefusesInvalidOrExhaustedBeforeBegin(t *testing.T) {
	const second = int64(time.Second)
	for _, row := range []struct {
		name       string
		epoch, now int64
		remaining  time.Duration
	}{
		{"missing-epoch", 0, 100 * second, 30 * time.Second},
		{"backwards-clock", 100 * second, 99 * second, 30 * time.Second},
		{"epoch-overflow", math.MaxInt64 - 63*second + 1, math.MaxInt64 - 63*second + 1, 65 * time.Second},
		{"grant-exhausted", 100 * second, 160 * second, 65 * time.Second},
		{"parent-exhausted", 100 * second, 104 * second, 5 * time.Second},
		{"canceled-parent", 100 * second, 104 * second, -time.Second},
	} {
		t.Run(row.name, func(t *testing.T) {
			if _, _, err := metadataLeaseWindow(row.epoch, row.now, row.remaining, false); err == nil {
				t.Fatal("impossible mutation budget accepted")
			}
		})
	}
}

func TestMetadataLeaseWindowObservationRetainsPurposeAndExistingWindow(t *testing.T) {
	const now int64 = 100_000_000_000
	deadline, leaseEnd, err := metadataLeaseWindow(0, now, 15*time.Second, true)
	if err != nil || deadline != now+int64(60*time.Second) || leaseEnd != now+int64(63*time.Second) {
		t.Fatal(deadline, leaseEnd, err)
	}
}

func TestMetadataLeaseWindowFitsExistingNaturalExpiryWait(t *testing.T) {
	const epoch int64 = 100_000_000_000
	now := epoch + int64(4*time.Second)
	_, leaseEnd, err := metadataLeaseWindow(epoch, now, 61*time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	contextEnd := epoch + int64(65*time.Second)
	now = epoch + int64(10*time.Second)
	err = metadataWaitLeaseExpiry(context.Background(), leaseEnd, func() (int64, error) {
		return now, nil
	}, func(_ context.Context, delay time.Duration) error {
		if now+int64(delay) >= contextEnd {
			return context.DeadlineExceeded
		}
		now += int64(delay)
		return nil
	})
	if errors.Is(err, context.DeadlineExceeded) || err != nil || now != epoch+int64(63*time.Second) {
		t.Fatalf("postcommit lease expiry did not complete inside original budget: now=%d err=%v", now, err)
	}
}

//go:build linux

package main

import (
	"errors"
	"testing"
)

func TestFinishMetadataPublicationOrder(t *testing.T) {
	xattrFailure := errors.New("xattr failure")
	readOnlyFailure := errors.New("read-only failure")
	for _, scenario := range []struct {
		name        string
		xattrErr    error
		readOnlyErr error
		wantErr     error
		wantCalls   string
	}{
		{name: "success", wantCalls: "xr"},
		{name: "xattr refuses before read-only", xattrErr: xattrFailure, wantErr: xattrFailure, wantCalls: "x"},
		{name: "read-only refusal retained", readOnlyErr: readOnlyFailure, wantErr: readOnlyFailure, wantCalls: "xr"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			calls := ""
			err := finishMetadata(func() error {
				calls += "x"
				return scenario.xattrErr
			}, func() error {
				calls += "r"
				return scenario.readOnlyErr
			})
			if !errors.Is(err, scenario.wantErr) || calls != scenario.wantCalls {
				t.Fatalf("got error=%v calls=%q; want error=%v calls=%q", err, calls, scenario.wantErr, scenario.wantCalls)
			}
		})
	}
}

func TestLeaseExclusiveAndBounded(t *testing.T) {
	slot := leaseSlot{Instance: "epoch-one"}
	request := leaseRequest{Transaction: "tx", Nonce: "nonce", Request: "digest", Deadline: 100}
	if err := slot.begin(request, 10); err != nil {
		t.Fatal(err)
	}
	if err := slot.begin(request, 11); err == nil {
		t.Fatal("occupied lease renewed")
	}
	if err := slot.check(request, "epoch-one", 99); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"deadline", "nonce", "instance", "missing"} {
		copySlot, copyRequest, now, epoch := slot, request, int64(20), "epoch-one"
		switch scenario {
		case "deadline":
			now = 100
		case "nonce":
			copyRequest.Nonce = "other"
		case "instance":
			epoch = "other"
		case "missing":
			copySlot.Active = false
		}
		if err := copySlot.check(copyRequest, epoch, now); err == nil {
			t.Fatal(scenario)
		}
	}
}

func TestPairReaderAndIrreversibleDisposition(t *testing.T) {
	manifest := []byte(`{"schema":"synthetic-manifest.v1","value":"new"}`)
	receipt := receiptBytes("tx", "epoch", digest(manifest))
	if err := committedPair(receipt, manifest, receipt); err != nil {
		t.Fatal(err)
	}
	if err := committedPair(receipt, []byte("old"), receipt); err == nil {
		t.Fatal("mixed pair")
	}
	if err := committedPair(receipt, manifest, []byte("other")); err == nil {
		t.Fatal("racing reader")
	}
	if got := disposition(true, errors.New("lease lost")); got != "committed_evidence_incomplete" {
		t.Fatal(got)
	}
	if got := disposition(false, errors.New("lease lost")); got != "aborted_preserved" {
		t.Fatal(got)
	}
	if got := disposition(true, nil); got != "PASS" {
		t.Fatal(got)
	}
}

func TestClosedFrames(t *testing.T) {
	value := binding{Transaction: "tx", Nonce: "nonce", Request: "request", Channel: "channel", Epoch: "epoch", Deadline: 100}
	message := frame{Binding: value, Kind: "PREPARED", Sequence: 3}
	if err := checkFrame(message, value, "PREPARED", 3, 20); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"sequence", "kind", "nonce", "epoch", "deadline", "channel", "request"} {
		changed := message
		switch scenario {
		case "sequence":
			changed.Sequence++
		case "kind":
			changed.Kind = "READ"
		case "nonce":
			changed.Binding.Nonce = "other"
		case "epoch":
			changed.Binding.Epoch = "other"
		case "deadline":
			changed.Binding.Deadline = 20
		case "channel":
			changed.Binding.Channel = "other"
		case "request":
			changed.Binding.Request = "other"
		}
		if err := checkFrame(changed, value, "PREPARED", 3, 20); err == nil {
			t.Fatal(scenario)
		}
	}
	for _, raw := range []string{`{"kind":"x","kind":"y"}`, `{"unknown":true}`, `{} {}`, `{"kind":`} {
		var decoded frame
		if err := strictJSON([]byte(raw), &decoded); err == nil {
			t.Fatal(raw)
		}
	}
}

func TestPublicationFailureNeverReplaysCommitted(t *testing.T) {
	for _, failure := range []string{"stage", "manifest", "grant", "receipt", "sync", "final"} {
		var calls []string
		ops := transactionOps{Step: func(name string) error {
			calls = append(calls, name)
			if name == failure {
				return errors.New(name)
			}
			return nil
		}}
		committed, err := publish(ops)
		if err == nil {
			t.Fatal(failure)
		}
		if committed != (failure == "sync" || failure == "final") {
			t.Fatal(failure, committed)
		}
		for _, name := range calls {
			if name == "rollback" {
				t.Fatal("automatic rollback not supported by prototype")
			}
		}
	}
}

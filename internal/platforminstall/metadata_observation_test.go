package platforminstall

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestMetadataConsecutiveObservationsAndPlanApply(t *testing.T) {
	now := int64(100)
	lease := newMetadataLease(strings.Repeat("a", 64), func() (int64, error) { return now, nil })
	request := MetadataLeaseRequest{Transaction: strings.Repeat("b", 64), RequestSHA256: strings.Repeat("c", 64), Nonce: strings.Repeat("d", 64), Deadline: 1000, Observation: true}
	for index := 0; index < 3; index++ {
		request.Nonce = fmt.Sprintf("%064x", index+1)
		proof, err := lease.Begin(request)
		if err != nil {
			t.Fatal("completed predecessor collision", err)
		}
		if _, err := lease.Check(MetadataLeaseCheck{Request: request, Instance: proof.Instance}); err != nil {
			t.Fatal(err)
		}
	}
	request.Observation = false
	proof, err := lease.Begin(request)
	if err != nil {
		t.Fatal("dry-run blocked apply", err)
	}
	if _, err := lease.Begin(request); err == nil {
		t.Fatal("occupied mutation renewed")
	}
	observation := request
	observation.Observation = true
	if _, err := lease.Begin(observation); err == nil {
		t.Fatal("observation ignored active mutation")
	}
	if _, err := lease.Check(MetadataLeaseCheck{Request: observation, Instance: proof.Instance}); err == nil {
		t.Fatal("mutation proof downgraded to observation")
	}
	if _, err := lease.Check(MetadataLeaseCheck{Request: request, Instance: proof.Instance}); err != nil {
		t.Fatal(err)
	}
	now = request.Deadline
	request.Deadline++
	if _, err := lease.Begin(request); err != nil {
		t.Fatal("expired predecessor blocks apply", err)
	}
}

func TestMetadataGenuinelyConcurrentObservationsAndMutation(t *testing.T) {
	for _, observation := range []bool{true, false} {
		lease := newMetadataLease(strings.Repeat("a", 64), func() (int64, error) { return 100, nil })
		start := make(chan struct{})
		results := make(chan error, 16)
		var workers sync.WaitGroup
		for index := 0; index < 16; index++ {
			workers.Add(1)
			go func(index int) {
				defer workers.Done()
				<-start
				request := MetadataLeaseRequest{Transaction: strings.Repeat("b", 64), RequestSHA256: strings.Repeat("c", 64), Nonce: fmt.Sprintf("%064x", index+1), Deadline: 1000, Observation: observation}
				_, err := lease.Begin(request)
				results <- err
			}(index)
		}
		close(start)
		workers.Wait()
		close(results)
		passed := 0
		for err := range results {
			if err == nil {
				passed++
			}
		}
		want := 1
		if observation {
			want = 16
		}
		if passed != want {
			t.Fatalf("observation=%v successful=%d want=%d", observation, passed, want)
		}
	}
}

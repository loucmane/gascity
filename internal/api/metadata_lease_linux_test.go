package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/platforminstall"
	"golang.org/x/sys/unix"
)

func TestSupervisorMetadataLeaseWireBindingAndGuards(t *testing.T) {
	supervisor := NewSupervisorMux(&fakeCityResolver{cities: map[string]*fakeState{"city": {}}}, nil, false, "", "", time.Time{})
	var now unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &now); err != nil {
		t.Fatal(err)
	}
	request := platforminstall.MetadataLeaseRequest{Transaction: strings.Repeat("a", 64), RequestSHA256: strings.Repeat("b", 64), Nonce: strings.Repeat("c", 64), Deadline: now.Nano() + 30_000_000_000}
	call := func(path string, body any, csrf bool) *httptest.ResponseRecorder {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		input := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
		input.Header.Set("Content-Type", "application/json")
		if csrf {
			input.Header.Set("X-GC-Request", "test")
		}
		output := httptest.NewRecorder()
		supervisor.ServeHTTP(output, input)
		return output
	}
	begin := "/v0/city/city/platform/metadata-lease/begin"
	check := "/v0/city/city/platform/metadata-lease/check"
	if response := call(begin, request, false); response.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF: %d %s", response.Code, response.Body.String())
	}
	response := call(begin, request, true)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("begin: %d %s", response.Code, response.Body.String())
	}
	var proof platforminstall.MetadataLeaseProof
	if err := json.Unmarshal(response.Body.Bytes(), &proof); err != nil {
		t.Fatal(err)
	}
	if proof.Request != request || len(proof.Instance) != 64 {
		t.Fatalf("incorrect response binding: %+v", proof)
	}
	if response := call(begin, request, true); response.Code != http.StatusConflict {
		t.Fatalf("renewal accepted: %d", response.Code)
	}
	if response := call(check, platforminstall.MetadataLeaseCheck{Request: request, Instance: proof.Instance}, true); response.Code != http.StatusOK {
		t.Fatalf("check: %d %s", response.Code, response.Body.String())
	}
	supervisor.readOnly = true
	if response := call(check, platforminstall.MetadataLeaseCheck{Request: request, Instance: proof.Instance}, true); response.Code != http.StatusForbidden {
		t.Fatalf("read-only accepted: %d", response.Code)
	}
	for _, path := range []string{begin, check} {
		if city, covered := cityScopedObjectMutation(path); !covered || city != "city" {
			t.Fatalf("lease route outside existing city write-auth boundary: %s", path)
		}
	}
}

func TestSupervisorMetadataObservationComposition(t *testing.T) {
	supervisor := NewSupervisorMux(&fakeCityResolver{cities: map[string]*fakeState{"city": {}}}, nil, false, "", "", time.Time{})
	var now unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &now); err != nil {
		t.Fatal(err)
	}
	request := platforminstall.MetadataLeaseRequest{Transaction: strings.Repeat("a", 64), RequestSHA256: strings.Repeat("b", 64), Nonce: strings.Repeat("c", 64), Deadline: now.Nano() + 28_000_000_000, Observation: true}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	input := httptest.NewRequest(http.MethodPost, "/v0/city/city/platform/metadata-lease/begin", bytes.NewReader(body))
	input.Header.Set("Content-Type", "application/json")
	input.Header.Set("X-GC-Request", "test")
	output := httptest.NewRecorder()
	supervisor.ServeHTTP(output, input)
	var wire platforminstall.MetadataLeaseProof
	if output.Code != http.StatusOK || json.Unmarshal(output.Body.Bytes(), &wire) != nil || wire.Request != request {
		t.Fatal("observation wire binding lost", output.Code, output.Body.String())
	}
	for index := 0; index < 3; index++ {
		request.Nonce = fmt.Sprintf("%064x", index+1)
		proof, err := supervisor.humaMetadataLeaseBegin(context.Background(), &MetadataLeaseBeginInput{CityName: "city", Body: request})
		if err != nil {
			t.Fatal("consecutive observation refused", err)
		}
		if _, err := supervisor.humaMetadataLeaseCheck(context.Background(), &MetadataLeaseCheckInput{CityName: "city", Body: platforminstall.MetadataLeaseCheck{Request: request, Instance: proof.Body.Instance}}); err != nil {
			t.Fatal(err)
		}
	}
	start, results := make(chan struct{}), make(chan error, 8)
	var workers sync.WaitGroup
	for index := 0; index < 8; index++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			<-start
			candidate := request
			candidate.Nonce = fmt.Sprintf("%064x", index+10)
			_, err := supervisor.humaMetadataLeaseBegin(context.Background(), &MetadataLeaseBeginInput{CityName: "city", Body: candidate})
			results <- err
		}(index)
	}
	close(start)
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal("concurrent observation refused", err)
		}
	}
	request.Observation = false
	if _, err := supervisor.humaMetadataLeaseBegin(context.Background(), &MetadataLeaseBeginInput{CityName: "city", Body: request}); err != nil {
		t.Fatal("completed dry-run obstructed apply", err)
	}
	request.Observation = true
	if _, err := supervisor.humaMetadataLeaseBegin(context.Background(), &MetadataLeaseBeginInput{CityName: "city", Body: request}); err == nil {
		t.Fatal("observation bypassed active mutation")
	}
}

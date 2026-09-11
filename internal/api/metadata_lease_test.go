package api

import (
	"context"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/platforminstall"
)

func TestSupervisorMetadataLeaseHandlersRefuseUnavailableSupport(t *testing.T) {
	supervisor := &SupervisorMux{}
	if _, err := supervisor.humaMetadataLeaseBegin(context.Background(), &MetadataLeaseBeginInput{}); err == nil {
		t.Fatal("missing lease support was accepted")
	}
	if _, err := supervisor.humaMetadataLeaseCheck(context.Background(), &MetadataLeaseCheckInput{}); err == nil {
		t.Fatal("missing lease support was accepted")
	}
}

func TestSupervisorMetadataLeaseHandlersDoNotAcceptArbitraryRequests(t *testing.T) {
	supervisor := &SupervisorMux{metadataLease: platforminstall.NewMetadataLease()}
	request := &MetadataLeaseBeginInput{Body: platforminstall.MetadataLeaseRequest{Transaction: strings.Repeat("a", 64), Nonce: strings.Repeat("b", 64), RequestSHA256: strings.Repeat("c", 64), Deadline: 1}}
	if _, err := supervisor.humaMetadataLeaseBegin(context.Background(), request); err == nil {
		t.Fatal("accepted expired arbitrary lease")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := supervisor.humaMetadataLeaseBegin(ctx, request); err == nil {
		t.Fatal("accepted canceled request")
	}
}

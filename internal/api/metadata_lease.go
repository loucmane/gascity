package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gastownhall/gascity/internal/api/apierr"
	"github.com/gastownhall/gascity/internal/platforminstall"
)

// MetadataLeaseBeginInput contains no command, path or lifecycle authority.
type MetadataLeaseBeginInput struct {
	CityName string `path:"cityName"`
	Body     platforminstall.MetadataLeaseRequest
}

// MetadataLeaseCheckInput names the exact nonrenewing instance-bound lease.
type MetadataLeaseCheckInput struct {
	CityName string `path:"cityName"`
	Body     platforminstall.MetadataLeaseCheck
}

// MetadataLeaseOutput is uncached, current in-memory lease evidence.
type MetadataLeaseOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         platforminstall.MetadataLeaseProof
}

func (sm *SupervisorMux) registerMetadataLeaseRoutes() {
	options := func(operation *huma.Operation) {
		operation.MaxBodyBytes = 4096
		operation.Errors = []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusServiceUnavailable}
	}
	huma.Post(sm.humaAPI, "/v0/city/{cityName}/platform/metadata-lease/begin", sm.humaMetadataLeaseBegin, addMutationCSRFParam, options)
	huma.Post(sm.humaAPI, "/v0/city/{cityName}/platform/metadata-lease/check", sm.humaMetadataLeaseCheck, addMutationCSRFParam, options)
}

func (sm *SupervisorMux) metadataLeaseReady(ctx context.Context, cityName string) error {
	if ctx.Err() != nil {
		return apierr.ServiceUnavailable.Msg("metadata lease request canceled")
	}
	if sm.readOnly {
		return apierr.Forbidden.Msg("metadata leases unavailable in read-only mode")
	}
	if sm.metadataLease == nil {
		return apierr.ServiceUnavailable.Msg("metadata lease support unavailable")
	}
	if sm.resolver == nil || sm.resolver.CityState(cityName) == nil {
		return apierr.ServiceUnavailable.Msg("metadata lease requires an existing running city")
	}
	return nil
}

func (sm *SupervisorMux) humaMetadataLeaseBegin(ctx context.Context, input *MetadataLeaseBeginInput) (*MetadataLeaseOutput, error) {
	if err := sm.metadataLeaseReady(ctx, input.CityName); err != nil {
		return nil, err
	}
	proof, err := sm.metadataLease.Begin(input.Body)
	return metadataLeaseOutput(proof, err)
}

func (sm *SupervisorMux) humaMetadataLeaseCheck(ctx context.Context, input *MetadataLeaseCheckInput) (*MetadataLeaseOutput, error) {
	if err := sm.metadataLeaseReady(ctx, input.CityName); err != nil {
		return nil, err
	}
	proof, err := sm.metadataLease.Check(input.Body)
	return metadataLeaseOutput(proof, err)
}

func metadataLeaseOutput(proof platforminstall.MetadataLeaseProof, err error) (*MetadataLeaseOutput, error) {
	if errors.Is(err, platforminstall.ErrMetadataLeaseConflict) {
		return nil, apierr.ConflictWrongState.Msg("metadata lease missing, occupied, changed or expired")
	}
	if err != nil {
		return nil, apierr.InvalidRequest.Msg("metadata lease refused: " + err.Error())
	}
	return &MetadataLeaseOutput{CacheControl: "no-store", Body: proof}, nil
}

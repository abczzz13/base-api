package infraapi

import (
	"context"
	"errors"

	"github.com/abczzz13/base-api/internal/apierrors"
	geninfra "github.com/abczzz13/base-api/internal/infraoas"
	"github.com/abczzz13/base-api/internal/requestid"
)

type oasHandler struct {
	service *Service
}

var _ geninfra.Handler = (*oasHandler)(nil)

// NewOASHandler adapts the handwritten service to the generated infra transport.
func NewOASHandler(service *Service) (geninfra.Handler, error) {
	if service == nil {
		return nil, errors.New("service is required")
	}

	return &oasHandler{service: service}, nil
}

func (h *oasHandler) GetLivez(ctx context.Context) (*geninfra.ProbeResponseHeaders, error) {
	response, err := h.service.GetLivez(ctx)
	if err != nil {
		return nil, infraDefaultError(ctx, err)
	}

	wrapped := &geninfra.ProbeResponseHeaders{Response: geninfra.ProbeResponse{Status: response.Status}}
	setInfraRequestIDHeader(wrapped, ctx)

	return wrapped, nil
}

func (h *oasHandler) GetReadyz(ctx context.Context) (*geninfra.ProbeResponseHeaders, error) {
	response, err := h.service.GetReadyz(ctx)
	if err != nil {
		return nil, infraDefaultError(ctx, err)
	}

	wrapped := &geninfra.ProbeResponseHeaders{Response: geninfra.ProbeResponse{Status: response.Status}}
	setInfraRequestIDHeader(wrapped, ctx)

	return wrapped, nil
}

func (h *oasHandler) GetHealthz(ctx context.Context) (*geninfra.HealthResponseHeaders, error) {
	response, err := h.service.GetHealthz(ctx)
	if err != nil {
		return nil, infraDefaultError(ctx, err)
	}

	wrapped := &geninfra.HealthResponseHeaders{
		Response: geninfra.HealthResponse{
			Status:      response.Status,
			Version:     response.Version,
			Timestamp:   response.Timestamp,
			Environment: response.Environment,
		},
	}
	setInfraRequestIDHeader(wrapped, ctx)

	return wrapped, nil
}

func (h *oasHandler) NewError(ctx context.Context, err error) *geninfra.DefaultErrorStatusCodeWithHeaders {
	return infraDefaultError(ctx, err)
}

func infraDefaultError(ctx context.Context, err error) *geninfra.DefaultErrorStatusCodeWithHeaders {
	apiErr := apierrors.ResolveError(ctx, err)

	response := &geninfra.DefaultErrorStatusCodeWithHeaders{
		StatusCode: apiErr.StatusCode,
		Response: geninfra.ErrorResponse{
			Code:    apiErr.Code,
			Message: apiErr.Message,
		},
	}
	if apiErr.RequestID != "" {
		response.XRequestID = geninfra.NewOptString(apiErr.RequestID)
		response.Response.RequestId = geninfra.NewOptString(apiErr.RequestID)
	}

	return response
}

func setInfraRequestIDHeader[T interface{ SetXRequestID(geninfra.OptString) }](response T, ctx context.Context) {
	if requestID := requestid.FromContext(ctx); requestID != "" {
		response.SetXRequestID(geninfra.NewOptString(requestID))
	}
}

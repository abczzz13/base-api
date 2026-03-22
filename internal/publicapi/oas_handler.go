package publicapi

import (
	"context"
	"errors"

	"github.com/abczzz13/base-api/internal/apierrors"
	"github.com/abczzz13/base-api/internal/notes"
	"github.com/abczzz13/base-api/internal/publicoas"
	"github.com/abczzz13/base-api/internal/requestid"
)

type oasHandler struct {
	service      *Service
	notesService *notes.Service
}

var _ publicoas.Handler = (*oasHandler)(nil)

// NewOASHandler adapts the handwritten services to the generated public transport.
func NewOASHandler(service *Service, notesService *notes.Service) (publicoas.Handler, error) {
	if service == nil {
		return nil, errors.New("service is required")
	}
	if notesService == nil {
		return nil, errors.New("notes service is required")
	}

	return &oasHandler{service: service, notesService: notesService}, nil
}

func (h *oasHandler) GetHealthz(ctx context.Context) (publicoas.GetHealthzRes, error) {
	response, err := h.service.GetHealthz(ctx)
	if err != nil {
		return nil, publicDefaultError(ctx, err)
	}

	wrapped := &publicoas.HealthResponseHeaders{
		Response: publicoas.HealthResponse{Status: response.Status},
	}
	setPublicRequestIDHeader(wrapped, ctx)

	return wrapped, nil
}

func (h *oasHandler) NewError(ctx context.Context, err error) *publicoas.DefaultErrorStatusCodeWithHeaders {
	return publicDefaultError(ctx, err)
}

func publicDefaultError(ctx context.Context, err error) *publicoas.DefaultErrorStatusCodeWithHeaders {
	apiErr := apierrors.ResolveError(ctx, err)

	response := &publicoas.DefaultErrorStatusCodeWithHeaders{
		StatusCode: apiErr.StatusCode,
		Response: publicoas.ErrorResponse{
			Code:    apiErr.Code,
			Message: apiErr.Message,
		},
	}
	if apiErr.RequestID != "" {
		response.XRequestID = publicoas.NewOptString(apiErr.RequestID)
		response.Response.RequestId = publicoas.NewOptString(apiErr.RequestID)
	}

	return response
}

func setPublicRequestIDHeader[T interface{ SetXRequestID(publicoas.OptString) }](response T, ctx context.Context) {
	if requestID := requestid.FromContext(ctx); requestID != "" {
		response.SetXRequestID(publicoas.NewOptString(requestID))
	}
}

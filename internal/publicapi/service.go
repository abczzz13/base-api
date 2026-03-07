package publicapi

import (
	"context"
	"net/http"

	"github.com/abczzz13/base-api/internal/apierrors"
	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/publicoas"
	"github.com/abczzz13/base-api/internal/requestid"
)

var _ publicoas.Handler = (*Service)(nil)

// Service implements the public OAS-generated handler interface.
type Service struct {
	cfg config.Config
}

// NewService creates a new public API service.
func NewService(cfg config.Config) *Service {
	return &Service{cfg: cfg}
}

func (s *Service) GetHealthz(ctx context.Context) (*publicoas.HealthResponseHeaders, error) {
	response := &publicoas.HealthResponseHeaders{
		Response: publicoas.HealthResponse{
			Status: "OK",
		},
	}
	if requestID := requestid.FromContext(ctx); requestID != "" {
		response.XRequestID = publicoas.NewOptString(requestID)
	}

	return response, nil
}

func (s *Service) NewError(ctx context.Context, err error) *publicoas.DefaultErrorStatusCodeWithHeaders {
	_ = err

	return apierrors.New(http.StatusInternalServerError, "internal_error", "internal server error").WithContext(ctx).OASDefault()
}

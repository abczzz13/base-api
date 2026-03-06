package publicapi

import (
	"context"
	"net/http"

	"github.com/abczzz13/base-api/internal/apierrors"
	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/publicoas"
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

func (s *Service) GetHealthz(ctx context.Context) (*publicoas.HealthResponse, error) {
	_ = ctx

	return &publicoas.HealthResponse{
		Status: "OK",
	}, nil
}

func (s *Service) NewError(ctx context.Context, err error) *publicoas.DefaultErrorStatusCode {
	_ = ctx
	_ = err

	return apierrors.New(http.StatusInternalServerError, "internal_error", "internal server error").OASDefault()
}

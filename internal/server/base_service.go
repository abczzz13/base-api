package server

import (
	"context"
	"net/http"

	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/oas"
)

var _ oas.Handler = (*baseService)(nil)

type baseService struct {
	cfg config.Config
}

func newBaseService(cfg config.Config) *baseService {
	return &baseService{cfg: cfg}
}

func (s *baseService) GetHealthz(ctx context.Context) (*oas.HealthResponse, error) {
	_ = ctx

	return &oas.HealthResponse{
		Status: "OK",
	}, nil
}

func (s *baseService) NewError(ctx context.Context, err error) *oas.DefaultErrorStatusCode {
	_ = ctx
	_ = err

	return newDefaultError(http.StatusInternalServerError, "internal_error", "internal server error")
}

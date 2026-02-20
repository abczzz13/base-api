package server

import (
	"context"
	"net/http"
	"time"

	"github.com/abczzz13/base-api/internal/infraoas"
	"github.com/abczzz13/base-api/internal/version"
)

var _ infraoas.Handler = (*infraService)(nil)

type infraService struct {
	cfg               Config
	readinessCheckers []ReadinessChecker
}

func newInfraService(cfg Config, readinessCheckers ...ReadinessChecker) *infraService {
	return &infraService{
		cfg:               cfg,
		readinessCheckers: readinessCheckers,
	}
}

func (s *infraService) GetLivez(ctx context.Context) (*infraoas.ProbeResponse, error) {
	_ = ctx

	return &infraoas.ProbeResponse{Status: "OK"}, nil
}

func (s *infraService) GetReadyz(ctx context.Context) (*infraoas.ProbeResponse, error) {
	for _, checker := range s.readinessCheckers {
		checkCtx := ctx
		cancel := func() {}
		if s.cfg.ReadyzTimeout > 0 {
			checkCtx, cancel = context.WithTimeout(ctx, s.cfg.ReadyzTimeout)
		}

		if err := checker.CheckReadiness(checkCtx); err != nil {
			cancel()
			return nil, newInfraDefaultError(
				http.StatusServiceUnavailable,
				"not_ready",
				"service is not ready",
			)
		}
		cancel()
	}

	return &infraoas.ProbeResponse{Status: "OK"}, nil
}

func (s *infraService) GetHealthz(ctx context.Context) (*infraoas.HealthResponse, error) {
	_ = ctx

	return &infraoas.HealthResponse{
		Status:      "OK",
		Version:     version.GetVersion(),
		Timestamp:   time.Now().Format(time.RFC3339),
		Environment: s.cfg.Environment,
	}, nil
}

func (s *infraService) GetMetrics(ctx context.Context) (infraoas.GetMetricsOK, error) {
	_ = ctx

	return infraoas.GetMetricsOK{}, newInfraDefaultError(
		http.StatusNotImplemented,
		"metrics_not_configured",
		"metrics endpoint is not configured",
	)
}

func (s *infraService) GetSwagger(ctx context.Context) (infraoas.GetSwaggerOK, error) {
	_ = ctx

	return infraoas.GetSwaggerOK{}, newInfraDefaultError(
		http.StatusNotImplemented,
		"docs_not_configured",
		"swagger endpoint is not configured",
	)
}

func (s *infraService) NewError(ctx context.Context, err error) *infraoas.DefaultErrorStatusCode {
	_ = ctx
	_ = err

	return newInfraDefaultError(http.StatusInternalServerError, "internal_error", "internal server error")
}

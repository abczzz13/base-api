package infraapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/abczzz13/base-api/internal/apierrors"
	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/version"
)

// ProbeResponse is the handwritten infra probe payload.
type ProbeResponse struct {
	Status string
}

// HealthResponse is the handwritten infra health payload.
type HealthResponse struct {
	Status      string
	Version     string
	Timestamp   string
	Environment string
}

// Service contains handwritten infra API behavior, independent of generated transport types.
type Service struct {
	cfg               config.Config
	readinessCheckers []ReadinessChecker
}

// NewService creates a new infra API service.
func NewService(cfg config.Config, readinessCheckers ...ReadinessChecker) *Service {
	filtered := make([]ReadinessChecker, 0, len(readinessCheckers))
	for _, c := range readinessCheckers {
		if c != nil {
			filtered = append(filtered, c)
		}
	}

	return &Service{
		cfg:               cfg,
		readinessCheckers: filtered,
	}
}

func (s *Service) GetLivez(context.Context) (ProbeResponse, error) {
	return ProbeResponse{Status: "OK"}, nil
}

func (s *Service) GetReadyz(ctx context.Context) (ProbeResponse, error) {
	checkCtx := ctx
	cancel := func() {}
	if s.cfg.ReadyzTimeout > 0 {
		checkCtx, cancel = context.WithTimeout(ctx, s.cfg.ReadyzTimeout)
	}
	defer cancel()

	for idx, checker := range s.readinessCheckers {
		if err := checker.CheckReadiness(checkCtx); err != nil {
			slog.WarnContext(ctx, "readiness check failed",
				slog.String("checker", ReadinessCheckerLogName(checker, idx)),
				slog.Int("checker_index", idx),
				slog.Any("error", err),
			)
			return ProbeResponse{}, apierrors.New(http.StatusServiceUnavailable, "not_ready", "service is not ready")
		}
	}

	return ProbeResponse{Status: "OK"}, nil
}

func (s *Service) GetHealthz(context.Context) (HealthResponse, error) {
	return HealthResponse{
		Status:      "OK",
		Version:     version.GetVersion(),
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Environment: s.cfg.Environment,
	}, nil
}

package infraapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/abczzz13/base-api/internal/apierrors"
	"github.com/abczzz13/base-api/internal/config"
	geninfra "github.com/abczzz13/base-api/internal/infraoas"
	"github.com/abczzz13/base-api/internal/version"
)

var _ geninfra.Handler = (*Service)(nil)

// Service implements the infra OAS-generated handler interface.
type Service struct {
	cfg               config.Config
	readinessCheckers []ReadinessChecker
}

// NewService creates a new infra API service.
func NewService(cfg config.Config, readinessCheckers ...ReadinessChecker) *Service {
	return &Service{
		cfg:               cfg,
		readinessCheckers: readinessCheckers,
	}
}

func (s *Service) GetLivez(ctx context.Context) (*geninfra.ProbeResponse, error) {
	_ = ctx

	return &geninfra.ProbeResponse{Status: "OK"}, nil
}

func (s *Service) GetReadyz(ctx context.Context) (*geninfra.ProbeResponse, error) {
	checkCtx := ctx
	cancel := func() {}
	if s.cfg.ReadyzTimeout > 0 {
		checkCtx, cancel = context.WithTimeout(ctx, s.cfg.ReadyzTimeout)
	}
	defer cancel()

	for idx, checker := range s.readinessCheckers {
		if checker == nil {
			checkerName := ReadinessCheckerLogName(checker, idx)
			slog.WarnContext(
				ctx,
				"readiness check failed",
				slog.String("checker", checkerName),
				slog.Int("checker_index", idx),
				slog.Any("error", ErrNilReadinessChecker),
			)
			return nil, apierrors.New(http.StatusServiceUnavailable, "not_ready", "service is not ready").InfraOASDefault()
		}

		if err := checker.CheckReadiness(checkCtx); err != nil {
			checkerName := ReadinessCheckerLogName(checker, idx)
			slog.WarnContext(
				ctx,
				"readiness check failed",
				slog.String("checker", checkerName),
				slog.Int("checker_index", idx),
				slog.Any("error", err),
			)
			return nil, apierrors.New(http.StatusServiceUnavailable, "not_ready", "service is not ready").InfraOASDefault()
		}
	}

	return &geninfra.ProbeResponse{Status: "OK"}, nil
}

func (s *Service) GetHealthz(ctx context.Context) (*geninfra.HealthResponse, error) {
	_ = ctx

	return &geninfra.HealthResponse{
		Status:      "OK",
		Version:     version.GetVersion(),
		Timestamp:   time.Now().Format(time.RFC3339),
		Environment: s.cfg.Environment,
	}, nil
}

func (s *Service) NewError(ctx context.Context, err error) *geninfra.DefaultErrorStatusCode {
	_ = ctx
	_ = err

	return apierrors.New(http.StatusInternalServerError, "internal_error", "internal server error").InfraOASDefault()
}

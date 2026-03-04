package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/infraoas"
	"github.com/abczzz13/base-api/internal/version"
)

var _ infraoas.Handler = (*infraService)(nil)

type infraService struct {
	cfg               config.Config
	readinessCheckers []ReadinessChecker
}

func newInfraService(cfg config.Config, readinessCheckers ...ReadinessChecker) *infraService {
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
	checkCtx := ctx
	cancel := func() {}
	if s.cfg.ReadyzTimeout > 0 {
		checkCtx, cancel = context.WithTimeout(ctx, s.cfg.ReadyzTimeout)
	}
	defer cancel()

	for idx, checker := range s.readinessCheckers {
		if checker == nil {
			checkerName := readinessCheckerLogName(checker, idx)
			slog.WarnContext(
				ctx,
				"readiness check failed",
				slog.String("checker", checkerName),
				slog.Int("checker_index", idx),
				slog.Any("error", errNilReadinessChecker),
			)
			return nil, newInfraDefaultError(
				http.StatusServiceUnavailable,
				"not_ready",
				"service is not ready",
			)
		}

		if err := checker.CheckReadiness(checkCtx); err != nil {
			checkerName := readinessCheckerLogName(checker, idx)
			slog.WarnContext(
				ctx,
				"readiness check failed",
				slog.String("checker", checkerName),
				slog.Int("checker_index", idx),
				slog.Any("error", err),
			)
			return nil, newInfraDefaultError(
				http.StatusServiceUnavailable,
				"not_ready",
				"service is not ready",
			)
		}
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

func (s *infraService) NewError(ctx context.Context, err error) *infraoas.DefaultErrorStatusCode {
	_ = ctx
	_ = err

	return newInfraDefaultError(http.StatusInternalServerError, "internal_error", "internal server error")
}

func readinessCheckerLogName(checker ReadinessChecker, index int) string {
	if checker == nil {
		return fmt.Sprintf("checker_%d", index)
	}

	namedChecker, ok := checker.(interface{ Name() string })
	if !ok {
		return fmt.Sprintf("checker_%d", index)
	}

	name := strings.TrimSpace(namedChecker.Name())
	if name == "" {
		return fmt.Sprintf("checker_%d", index)
	}

	return name
}

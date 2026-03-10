package infraapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/abczzz13/base-api/internal/apierrors"
	"github.com/abczzz13/base-api/internal/config"
	geninfra "github.com/abczzz13/base-api/internal/infraoas"
	"github.com/abczzz13/base-api/internal/requestid"
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

func (s *Service) GetLivez(ctx context.Context) (*geninfra.ProbeResponseHeaders, error) {
	return probeResponseWithRequestID(ctx, geninfra.ProbeResponse{Status: "OK"}), nil
}

func (s *Service) GetReadyz(ctx context.Context) (*geninfra.ProbeResponseHeaders, error) {
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
			return nil, apierrors.New(http.StatusServiceUnavailable, "not_ready", "service is not ready").WithContext(ctx).InfraOASDefault()
		}
	}

	return probeResponseWithRequestID(ctx, geninfra.ProbeResponse{Status: "OK"}), nil
}

func (s *Service) GetHealthz(ctx context.Context) (*geninfra.HealthResponseHeaders, error) {
	response := &geninfra.HealthResponseHeaders{
		Response: geninfra.HealthResponse{
			Status:      "OK",
			Version:     version.GetVersion(),
			Timestamp:   time.Now().Format(time.RFC3339),
			Environment: s.cfg.Environment,
		},
	}
	if requestID := requestid.FromContext(ctx); requestID != "" {
		response.XRequestID = geninfra.NewOptString(requestID)
	}

	return response, nil
}

func (s *Service) NewError(ctx context.Context, err error) *geninfra.DefaultErrorStatusCodeWithHeaders {
	_ = err

	return apierrors.New(http.StatusInternalServerError, "internal_error", "internal server error").WithContext(ctx).InfraOASDefault()
}

func probeResponseWithRequestID(ctx context.Context, response geninfra.ProbeResponse) *geninfra.ProbeResponseHeaders {
	wrapped := &geninfra.ProbeResponseHeaders{Response: response}
	if requestID := requestid.FromContext(ctx); requestID != "" {
		wrapped.XRequestID = geninfra.NewOptString(requestID)
	}

	return wrapped
}

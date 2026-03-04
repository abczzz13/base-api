package server

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/telemetry"
	"github.com/abczzz13/base-api/internal/version"
)

const defaultTracingShutdownTimeout = 5 * time.Second

func setupTracing(ctx context.Context, cfg config.Config) (bool, func()) {
	telemetryShutdown := noopTracingShutdown
	tracingEnabled := cfg.OTEL.TracingEnabled
	if tracingEnabled {
		shutdown, err := telemetry.InitTracing(ctx, cfg.OTEL.TelemetryConfig(cfg.Environment, version.GetVersion()))
		if err != nil {
			slog.WarnContext(ctx, "OpenTelemetry tracing disabled", "error", err)
			tracingEnabled = false
		} else {
			telemetryShutdown = shutdown
		}
	}

	var shutdownTracingOnce sync.Once
	return tracingEnabled, func() {
		shutdownTracingOnce.Do(func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), defaultTracingShutdownTimeout)
			defer shutdownCancel()

			if err := telemetryShutdown(shutdownCtx); err != nil {
				slog.WarnContext(ctx, "shutdown tracing", "error", err)
			}
		})
	}
}

func noopTracingShutdown(context.Context) error {
	return nil
}

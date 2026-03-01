package server

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/abczzz13/base-api/internal/telemetry"
	"github.com/abczzz13/base-api/internal/version"
)

const defaultTracingShutdownTimeout = 5 * time.Second

func setupTracing(ctx context.Context, cfg *Config) func() {
	if cfg == nil {
		return func() {}
	}

	telemetryShutdown := noopTracingShutdown
	if cfg.OTEL.TracingEnabled {
		shutdown, err := telemetry.InitTracing(ctx, telemetry.Config{
			ServiceName:      cfg.OTEL.ServiceName,
			ServiceVersion:   version.GetVersion(),
			Environment:      cfg.Environment,
			TracesSampler:    cfg.OTEL.TracesSampler,
			TracesSamplerArg: cfg.OTEL.TracesSamplerArg,
		})
		if err != nil {
			slog.WarnContext(ctx, "OpenTelemetry tracing disabled", "error", err)
			cfg.OTEL.TracingEnabled = false
		} else {
			telemetryShutdown = shutdown
		}
	}

	var shutdownTracingOnce sync.Once
	return func() {
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

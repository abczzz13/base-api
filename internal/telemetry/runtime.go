package telemetry

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const defaultTracingShutdownTimeout = 5 * time.Second

// Setup configures tracing and returns the effective enabled flag and shutdown callback.
func Setup(ctx context.Context, enabled bool, cfg Config) (bool, func()) {
	shutdownTracing := noopShutdown
	tracingEnabled := enabled
	if tracingEnabled {
		shutdown, err := InitTracing(ctx, cfg)
		if err != nil {
			slog.WarnContext(ctx, "OpenTelemetry tracing disabled", "error", err)
			tracingEnabled = false
		} else {
			shutdownTracing = shutdown
		}
	}

	var shutdownTracingOnce sync.Once
	return tracingEnabled, func() {
		shutdownTracingOnce.Do(func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), defaultTracingShutdownTimeout)
			defer shutdownCancel()

			if err := shutdownTracing(shutdownCtx); err != nil {
				slog.WarnContext(ctx, "shutdown tracing", "error", err)
			}
		})
	}
}

func noopShutdown(context.Context) error {
	return nil
}

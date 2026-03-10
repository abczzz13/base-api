package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/abczzz13/base-api/internal/config"
)

// SetupRuntime configures database connectivity, migrations, retries, and metrics.
func SetupRuntime(
	ctx context.Context,
	cfg config.DBConfig,
	metricsRegisterer prometheus.Registerer,
) (*pgxpool.Pool, func(), error) {
	if err := validateRuntimeInputs(cfg, metricsRegisterer); err != nil {
		return nil, nil, err
	}

	maxAttempts := max(cfg.StartupMaxAttempts, int32(1))

	var lastErr error
	for attempt := int32(1); attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, fmt.Errorf("database startup canceled: %w", err)
		}

		pool, retryable, err := setupRuntimeStartupAttempt(ctx, cfg)
		if err == nil {
			return finalizeRuntimeSetup(metricsRegisterer, pool)
		}

		lastErr = err
		if !retryable {
			return nil, nil, fmt.Errorf("database startup failed: %w", err)
		}

		if attempt == maxAttempts {
			return nil, nil, fmt.Errorf("database startup failed after %d attempt(s): %w", maxAttempts, err)
		}

		retryIn := startupRetryDelay(cfg.StartupBackoffInitial, cfg.StartupBackoffMax, attempt)
		slog.WarnContext(
			ctx,
			"database startup attempt failed",
			slog.Int64("attempt", int64(attempt)),
			slog.Int64("max_attempts", int64(maxAttempts)),
			slog.Duration("retry_in", retryIn),
			slog.Any("error", err),
		)

		if err := sleepWithContext(ctx, retryIn); err != nil {
			return nil, nil, fmt.Errorf("database startup canceled: %w", err)
		}
	}

	if lastErr != nil {
		return nil, nil, fmt.Errorf("database startup failed: %w", lastErr)
	}

	return nil, nil, errors.New("database startup failed")
}

func validateRuntimeInputs(cfg config.DBConfig, metricsRegisterer prometheus.Registerer) error {
	if !cfg.Enabled() {
		return errors.New("database URL is required")
	}
	if metricsRegisterer == nil {
		return errors.New("metrics registerer is required")
	}

	return nil
}

func setupRuntimeStartupAttempt(
	ctx context.Context,
	cfg config.DBConfig,
) (*pgxpool.Pool, bool, error) {
	pool, err := Open(ctx, cfg)
	if err != nil {
		return nil, isRetryableError(ctx, err), fmt.Errorf("open PostgreSQL pool: %w", err)
	}

	if cfg.MigrateOnStartup {
		if err := runStartupMigrations(ctx, cfg.MigrateTimeout, pool); err != nil {
			pool.Close()
			return nil, isRetryableError(ctx, err), fmt.Errorf("run PostgreSQL migrations: %w", err)
		}
	} else {
		slog.InfoContext(ctx, "database startup migrations disabled")
	}

	return pool, false, nil
}

func runStartupMigrations(
	ctx context.Context,
	timeout time.Duration,
	pool *pgxpool.Pool,
) error {
	migrateCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		migrateCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	return Migrate(migrateCtx, pool)
}

func finalizeRuntimeSetup(
	metricsRegisterer prometheus.Registerer,
	pool *pgxpool.Pool,
) (*pgxpool.Pool, func(), error) {
	if err := RegisterPoolMetrics(metricsRegisterer, pool); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("register PostgreSQL metrics: %w", err)
	}

	var closeOnce sync.Once
	return pool, func() {
		closeOnce.Do(pool.Close)
	}, nil
}

func isRetryableError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		if ctx != nil && ctx.Err() != nil {
			return false
		}

		return true
	}

	if ctx != nil && errors.Is(ctx.Err(), context.Canceled) {
		return false
	}
	if errors.Is(err, ErrDatabaseURLRequired) || errors.Is(err, ErrInvalidDatabaseURL) {
		return false
	}

	if retryable, ok := classifyRetryableError(err); ok {
		return retryable
	}

	return true
}

func classifyRetryableError(err error) (bool, bool) {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false, false
	}

	return isRetryableSQLState(pgErr.Code), true
}

func isRetryableSQLState(code string) bool {
	if len(code) < 2 {
		return false
	}

	switch code[:2] {
	case "08", "40", "53", "58":
		return true
	case "55":
		return code == "55P03"
	case "57":
		return code == "57P01" || code == "57P02" || code == "57P03"
	default:
		return false
	}
}

func startupRetryDelay(initial, maxDelay time.Duration, attempt int32) time.Duration {
	if initial <= 0 || attempt < 1 {
		return 0
	}

	maxDelay = max(maxDelay, initial)

	delay := initial
	for i := int32(1); i < attempt; i++ {
		if delay >= maxDelay || delay > maxDelay/2 {
			return maxDelay
		}

		delay *= 2
	}

	return min(delay, maxDelay)
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/abczzz13/base-api/internal/config"
)

var (
	ErrDatabaseURLRequired = errors.New("database URL is required")
	ErrInvalidDatabaseURL  = errors.New("invalid database URL")
)

func Open(ctx context.Context, cfg config.DBConfig) (*pgxpool.Pool, error) {
	poolConfig, err := parsePoolConfig(cfg)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}

	pingCtx := ctx
	cancel := func() {}
	if cfg.ConnectTimeout > 0 {
		pingCtx, cancel = context.WithTimeout(ctx, cfg.ConnectTimeout)
	}
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}

	return pool, nil
}

func parsePoolConfig(cfg config.DBConfig) (*pgxpool.Config, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, ErrDatabaseURLRequired
	}

	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("%w: parse database URL: %w", ErrInvalidDatabaseURL, err)
	}

	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolConfig.HealthCheckPeriod = cfg.HealthCheckPeriod
	// DB_CONNECT_TIMEOUT takes precedence over any connect_timeout in DB_URL.
	if cfg.ConnectTimeout > 0 {
		poolConfig.ConnConfig.ConnectTimeout = cfg.ConnectTimeout
	}

	return poolConfig, nil
}

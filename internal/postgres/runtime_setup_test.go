package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/abczzz13/base-api/internal/config"
)

func TestSetupDatabaseValidation(t *testing.T) {
	tests := []struct {
		name         string
		cfg          config.DBConfig
		registerer   prometheus.Registerer
		wantContains string
	}{
		{
			name:         "fails when database URL is missing",
			cfg:          config.DBConfig{},
			registerer:   nil,
			wantContains: "database URL is required",
		},
		{
			name:         "fails when metrics registerer is missing",
			cfg:          testDatabaseConfig("postgres://postgres:postgres@127.0.0.1:5432/base_api?sslmode=disable"),
			registerer:   nil,
			wantContains: "metrics registerer is required",
		},
		{
			name:         "fails when database URL is invalid",
			cfg:          testDatabaseConfig("postgres://:bad"),
			registerer:   prometheus.NewRegistry(),
			wantContains: "open PostgreSQL pool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			database, shutdown, err := SetupRuntime(context.Background(), tt.cfg, tt.registerer)
			if database != nil {
				t.Fatal("setupDatabase returned unexpected database dependency")
			}
			if shutdown != nil {
				t.Fatal("setupDatabase returned unexpected shutdown function")
			}
			if err == nil {
				t.Fatal("setupDatabase returned nil error")
			}
			if !strings.Contains(err.Error(), tt.wantContains) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantContains)
			}
		})
	}
}

func TestSetupDatabaseRetriesOpenFailuresAndExhaustsAttempts(t *testing.T) {
	registerer := prometheus.NewRegistry()
	cfg := testDatabaseConfig("postgres://postgres:postgres@127.0.0.1:1/base_api?sslmode=disable")
	cfg.ConnectTimeout = 50 * time.Millisecond
	cfg.StartupMaxAttempts = 2
	cfg.StartupBackoffInitial = time.Millisecond
	cfg.StartupBackoffMax = time.Millisecond

	database, shutdown, err := SetupRuntime(context.Background(), cfg, registerer)
	if database != nil {
		t.Fatal("database should be nil on startup failure")
	}
	if shutdown != nil {
		t.Fatal("shutdown should be nil on startup failure")
	}
	if err == nil {
		t.Fatal("setupDatabase returned nil error")
	}
	if !strings.Contains(err.Error(), "database startup failed after 2 attempt(s)") {
		t.Fatalf("error %q does not contain startup attempt context", err.Error())
	}
	if !strings.Contains(err.Error(), "open PostgreSQL pool") {
		t.Fatalf("error %q does not contain open failure context", err.Error())
	}
}

func TestSetupDatabaseStopsRetriesWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	registerer := prometheus.NewRegistry()
	cfg := testDatabaseConfig("postgres://postgres:postgres@127.0.0.1:1/base_api?sslmode=disable")
	cfg.ConnectTimeout = 50 * time.Millisecond
	cfg.StartupMaxAttempts = 3
	cfg.StartupBackoffInitial = time.Millisecond
	cfg.StartupBackoffMax = time.Millisecond

	database, shutdown, err := SetupRuntime(ctx, cfg, registerer)
	if database != nil {
		t.Fatal("database should be nil when startup is canceled")
	}
	if shutdown != nil {
		t.Fatal("shutdown should be nil when startup is canceled")
	}
	if err == nil {
		t.Fatal("setupDatabase returned nil error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
	if !strings.Contains(err.Error(), "database startup canceled") {
		t.Fatalf("error %q does not contain cancellation context", err.Error())
	}
}

func TestIsRetryableError(t *testing.T) {
	startupDeadlineExceededCtx := expiredContextWithDeadline(t)

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{
			name: "context canceled is not retryable",
			ctx:  context.Background(),
			err:  context.Canceled,
			want: false,
		},
		{
			name: "per-attempt deadline exceeded is retryable",
			ctx:  context.Background(),
			err:  context.DeadlineExceeded,
			want: true,
		},
		{
			name: "startup context deadline exceeded is not retryable",
			ctx:  startupDeadlineExceededCtx,
			err:  context.DeadlineExceeded,
			want: false,
		},
		{
			name: "missing database URL is not retryable",
			ctx:  context.Background(),
			err:  ErrDatabaseURLRequired,
			want: false,
		},
		{
			name: "invalid database URL is not retryable",
			ctx:  context.Background(),
			err:  ErrInvalidDatabaseURL,
			want: false,
		},
		{
			name: "connection exception SQLSTATE is retryable",
			ctx:  context.Background(),
			err:  &pgconn.PgError{Code: "08006"},
			want: true,
		},
		{
			name: "lock not available SQLSTATE is retryable",
			ctx:  context.Background(),
			err:  &pgconn.PgError{Code: "55P03"},
			want: true,
		},
		{
			name: "syntax SQLSTATE is not retryable",
			ctx:  context.Background(),
			err:  &pgconn.PgError{Code: "42601"},
			want: false,
		},
		{
			name: "generic errors are treated as retryable",
			ctx:  context.Background(),
			err:  errors.New("network hiccup"),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableError(tt.ctx, tt.err); got != tt.want {
				t.Fatalf("isRetryableError mismatch: want %t, got %t", tt.want, got)
			}
		})
	}
}

func expiredContextWithDeadline(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	t.Cleanup(cancel)

	return ctx
}

func TestDatabaseStartupRetryDelay(t *testing.T) {
	tests := []struct {
		name    string
		initial time.Duration
		max     time.Duration
		attempt int32
		want    time.Duration
	}{
		{
			name:    "returns zero when initial delay is not configured",
			initial: 0,
			max:     time.Second,
			attempt: 1,
			want:    0,
		},
		{
			name:    "uses initial delay for first retry",
			initial: time.Second,
			max:     10 * time.Second,
			attempt: 1,
			want:    time.Second,
		},
		{
			name:    "exponentially increases delay",
			initial: time.Second,
			max:     10 * time.Second,
			attempt: 3,
			want:    4 * time.Second,
		},
		{
			name:    "caps delay at configured maximum",
			initial: time.Second,
			max:     10 * time.Second,
			attempt: 6,
			want:    10 * time.Second,
		},
		{
			name:    "uses initial delay when max is lower",
			initial: 2 * time.Second,
			max:     time.Second,
			attempt: 2,
			want:    2 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := startupRetryDelay(tt.initial, tt.max, tt.attempt); got != tt.want {
				t.Fatalf("databasestartupRetryDelay mismatch: want %s, got %s", tt.want, got)
			}
		})
	}
}

func testDatabaseConfig(url string) config.DBConfig {
	return config.DBConfig{
		URL:               url,
		MinConns:          0,
		MaxConns:          5,
		MaxConnLifetime:   time.Minute,
		MaxConnIdleTime:   30 * time.Second,
		HealthCheckPeriod: time.Second,
		ConnectTimeout:    5 * time.Second,
	}
}

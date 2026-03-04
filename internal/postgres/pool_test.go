package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/abczzz13/base-api/internal/config"
)

func TestOpenValidation(t *testing.T) {
	tests := []struct {
		name         string
		cfg          config.DBConfig
		wantContains string
	}{
		{
			name:         "fails when URL is empty",
			cfg:          config.DBConfig{},
			wantContains: "database URL is required",
		},
		{
			name: "fails when URL is malformed",
			cfg: config.DBConfig{
				URL: "postgres://:bad",
			},
			wantContains: "parse database URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := Open(context.Background(), tt.cfg)
			if pool != nil {
				t.Fatalf("Open returned unexpected pool: %#v", pool)
			}
			if err == nil {
				t.Fatal("Open returned nil error")
			}
			if !strings.Contains(err.Error(), tt.wantContains) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantContains)
			}
		})
	}
}

func TestParsePoolConfigConfiguresConnectTimeout(t *testing.T) {
	const expectedTimeout = 7 * time.Second

	poolConfig, err := parsePoolConfig(config.DBConfig{
		URL:               "postgres://base@127.0.0.1:5432/base_api?sslmode=disable",
		MinConns:          2,
		MaxConns:          11,
		MaxConnLifetime:   3 * time.Minute,
		MaxConnIdleTime:   2 * time.Minute,
		HealthCheckPeriod: 30 * time.Second,
		ConnectTimeout:    expectedTimeout,
	})
	if err != nil {
		t.Fatalf("parsePoolConfig returned error: %v", err)
	}

	if poolConfig.MinConns != 2 {
		t.Fatalf("min conns mismatch: want %d, got %d", 2, poolConfig.MinConns)
	}
	if poolConfig.MaxConns != 11 {
		t.Fatalf("max conns mismatch: want %d, got %d", 11, poolConfig.MaxConns)
	}
	if poolConfig.MaxConnLifetime != 3*time.Minute {
		t.Fatalf("max conn lifetime mismatch: want %s, got %s", 3*time.Minute, poolConfig.MaxConnLifetime)
	}
	if poolConfig.MaxConnIdleTime != 2*time.Minute {
		t.Fatalf("max conn idle time mismatch: want %s, got %s", 2*time.Minute, poolConfig.MaxConnIdleTime)
	}
	if poolConfig.HealthCheckPeriod != 30*time.Second {
		t.Fatalf("health check period mismatch: want %s, got %s", 30*time.Second, poolConfig.HealthCheckPeriod)
	}
	if poolConfig.ConnConfig.ConnectTimeout != expectedTimeout {
		t.Fatalf("connect timeout mismatch: want %s, got %s", expectedTimeout, poolConfig.ConnConfig.ConnectTimeout)
	}
}

func TestParsePoolConfigPreservesURLConnectTimeoutWhenConfigTimeoutIsDisabled(t *testing.T) {
	poolConfig, err := parsePoolConfig(config.DBConfig{
		URL:            "postgres://base@127.0.0.1:5432/base_api?sslmode=disable&connect_timeout=13",
		ConnectTimeout: 0,
	})
	if err != nil {
		t.Fatalf("parsePoolConfig returned error: %v", err)
	}

	if poolConfig.ConnConfig.ConnectTimeout != 13*time.Second {
		t.Fatalf("URL connect timeout mismatch: want %s, got %s", 13*time.Second, poolConfig.ConnConfig.ConnectTimeout)
	}
}

func TestParsePoolConfigOverridesURLConnectTimeoutWhenConfigTimeoutIsSet(t *testing.T) {
	poolConfig, err := parsePoolConfig(config.DBConfig{
		URL:            "postgres://base@127.0.0.1:5432/base_api?sslmode=disable&connect_timeout=13",
		ConnectTimeout: 4 * time.Second,
	})
	if err != nil {
		t.Fatalf("parsePoolConfig returned error: %v", err)
	}

	if poolConfig.ConnConfig.ConnectTimeout != 4*time.Second {
		t.Fatalf("connect timeout mismatch: want %s, got %s", 4*time.Second, poolConfig.ConnConfig.ConnectTimeout)
	}
}

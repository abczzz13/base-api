package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/abczzz13/base-api/internal/config"
)

func TestOpenMigrateAndMetricsIntegration(t *testing.T) {
	dbURL := strings.TrimSpace(os.Getenv("TEST_DB_URL"))
	if dbURL == "" {
		t.Skip("set TEST_DB_URL to run PostgreSQL integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := Open(ctx, config.DBConfig{
		URL:               dbURL,
		MinConns:          0,
		MaxConns:          5,
		MaxConnLifetime:   time.Minute,
		MaxConnIdleTime:   30 * time.Second,
		HealthCheckPeriod: time.Second,
		ConnectTimeout:    5 * time.Second,
	})
	if err != nil {
		t.Skipf("PostgreSQL not reachable with TEST_DB_URL: %v", err)
	}
	defer pool.Close()

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}

	reg := prometheus.NewRegistry()
	if err := RegisterPoolMetrics(reg, pool); err != nil {
		t.Fatalf("RegisterPoolMetrics returned error: %v", err)
	}
	if _, err := reg.Gather(); err != nil {
		t.Fatalf("gather registered metrics: %v", err)
	}
}

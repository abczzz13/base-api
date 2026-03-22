package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestSetupDatabaseIntegrationSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	registerer := prometheus.NewRegistry()
	cfg := testDatabaseConfig(testDatabaseURL(t))
	cfg.ConnectTimeout = 3 * time.Second
	cfg.MigrateOnStartup = true
	cfg.MigrateTimeout = 10 * time.Second
	cfg.StartupMaxAttempts = 2

	database, shutdown, err := SetupRuntime(ctx, cfg, registerer)
	if err != nil {
		t.Fatalf("PostgreSQL integration unavailable: %v", err)
	}
	if database == nil {
		t.Fatal("setupDatabase returned nil database")
	}
	if shutdown == nil {
		t.Fatal("setupDatabase returned nil shutdown function")
	}

	if err := database.Ping(ctx); err != nil {
		t.Fatalf("database ping failed after setup: %v", err)
	}

	shutdown()
	shutdown()
}

func TestSetupDatabaseIntegrationFailsWhenMetricsAlreadyRegistered(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	registerer := prometheus.NewRegistry()
	cfg := testDatabaseConfig(testDatabaseURL(t))
	cfg.ConnectTimeout = 3 * time.Second
	cfg.MigrateOnStartup = false
	cfg.StartupMaxAttempts = 2

	database, shutdown, err := SetupRuntime(ctx, cfg, registerer)
	if err != nil {
		t.Fatalf("PostgreSQL integration unavailable: %v", err)
	}
	if database == nil {
		t.Fatal("initial setupDatabase returned nil database")
	}
	if shutdown == nil {
		t.Fatal("initial setupDatabase returned nil shutdown function")
	}
	t.Cleanup(shutdown)

	secondDatabase, secondShutdown, secondErr := SetupRuntime(ctx, cfg, registerer)
	if secondDatabase != nil {
		t.Fatal("second setupDatabase should not return database on metrics conflict")
	}
	if secondShutdown != nil {
		t.Fatal("second setupDatabase should not return shutdown function on metrics conflict")
	}
	if secondErr == nil {
		t.Fatal("second setupDatabase returned nil error")
	}
	if !strings.Contains(secondErr.Error(), "register PostgreSQL metrics") {
		t.Fatalf("error %q does not contain metrics registration context", secondErr.Error())
	}
}

func testDatabaseURL(tb testing.TB) string {
	tb.Helper()

	if value := strings.TrimSpace(os.Getenv("TEST_DB_URL")); value != "" {
		return value
	}

	tb.Fatal("set TEST_DB_URL to run database-backed tests")

	return ""
}

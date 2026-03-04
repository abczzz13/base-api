package server

import (
	"context"
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

	database, shutdown, err := setupDatabase(ctx, cfg, registerer)
	if err != nil {
		t.Fatalf("setupDatabase returned error: %v", err)
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

	database, shutdown, err := setupDatabase(ctx, cfg, registerer)
	if err != nil {
		t.Fatalf("initial setupDatabase returned error: %v", err)
	}
	if database == nil {
		t.Fatal("initial setupDatabase returned nil database")
	}
	if shutdown == nil {
		t.Fatal("initial setupDatabase returned nil shutdown function")
	}
	t.Cleanup(shutdown)

	secondDatabase, secondShutdown, secondErr := setupDatabase(ctx, cfg, registerer)
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

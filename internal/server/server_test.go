package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/logger"
	"github.com/abczzz13/base-api/internal/telemetry"
)

func TestRunReturnsErrorWhenConfigValidationFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := Run(
		ctx,
		lookupEnvFromMap(map[string]string{
			"API_ADDR":        "invalid-address",
			"API_INFRA_ADDR":  reserveTCPAddress(t),
			"API_ENVIRONMENT": "test",
		}),
		io.Discard,
	)
	if err == nil {
		t.Fatalf("Run returned nil error for invalid configured address")
	}
	if !strings.Contains(err.Error(), "load config") {
		t.Fatalf("Run error does not identify config loading failure: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid-address") {
		t.Fatalf("Run error does not include invalid address context: %v", err)
	}
}

func TestRunReturnsErrorWhenDatabaseConfigurationFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := Run(
		ctx,
		lookupEnvFromMap(map[string]string{
			"API_ADDR":        reserveTCPAddress(t),
			"API_INFRA_ADDR":  reserveTCPAddress(t),
			"API_ENVIRONMENT": "test",
			"DB_URL":          "postgres://:bad",
		}),
		io.Discard,
	)
	if err == nil {
		t.Fatalf("Run returned nil error for invalid database configuration")
	}
	if !strings.Contains(err.Error(), "configure database") {
		t.Fatalf("Run error does not identify database setup failure: %v", err)
	}
}

func TestRunReturnsErrorWhenDatabaseURLIsMissing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := Run(
		ctx,
		lookupEnvFromMap(map[string]string{
			"API_ADDR":        reserveTCPAddress(t),
			"API_INFRA_ADDR":  reserveTCPAddress(t),
			"API_ENVIRONMENT": "test",
		}),
		io.Discard,
	)
	if err == nil {
		t.Fatal("Run returned nil error when DB_URL is missing")
	}
	if !strings.Contains(err.Error(), "configure database") {
		t.Fatalf("Run error does not identify database setup failure: %v", err)
	}
	if !strings.Contains(err.Error(), "database URL is required") {
		t.Fatalf("Run error does not include missing DB_URL context: %v", err)
	}
}

func TestLogStartupConfigurationRecordsSafeSummary(t *testing.T) {
	previousLogger := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	var logs bytes.Buffer
	logger.New(logger.Config{
		Format:      logger.FormatJSON,
		Level:       slog.LevelInfo,
		Environment: "test",
		Writer:      &logs,
	})

	samplerArg := 0.25
	cfg := config.Config{
		Environment:       "production",
		Address:           "0.0.0.0:8080",
		InfraAddress:      "127.0.0.1:9090",
		ReadyzTimeout:     2 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		CORS: config.CORSConfig{
			AllowedOrigins:   []string{"https://client.example", "https://admin.example"},
			AllowCredentials: true,
		},
		CSRF: config.CSRFConfig{
			Enabled:        true,
			TrustedOrigins: []string{"https://client.example"},
		},
		RequestAudit: config.RequestAuditConfig{
			TrustedProxyCIDRs: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
		},
		OTEL: config.OTELConfig{
			TracingEnabled:   true,
			TracesSampler:    telemetry.TraceSamplerTraceIDRatio,
			TracesSamplerArg: &samplerArg,
		},
		DB: config.DBConfig{
			URL:                   "postgres://db.example/base_api",
			MinConns:              2,
			MaxConns:              20,
			MigrateOnStartup:      true,
			StartupMaxAttempts:    5,
			StartupBackoffInitial: time.Second,
			StartupBackoffMax:     30 * time.Second,
		},
	}

	logStartupConfiguration(context.Background(), cfg)

	entries := decodeJSONLogLines(t, logs.String())
	if len(entries) == 0 {
		t.Fatal("expected startup configuration log entry")
	}

	entry := entries[len(entries)-1]
	if got, _ := entry["msg"].(string); got != "startup configuration" {
		t.Fatalf("log message mismatch: want %q, got %q", "startup configuration", got)
	}

	for key, want := range map[string]any{
		"request_logger_enabled":                  true,
		"request_audit_enabled":                   true,
		"request_audit_client_ip_security_mode":   "strict",
		"request_audit_client_ip_priority":        "x_forwarded_for,remote_addr",
		"request_audit_trusted_proxy_source":      "configured",
		"request_audit_trusted_proxy_cidrs_count": float64(1),
		"cors_allowed_origins_count":              float64(2),
		"csrf_trusted_origins_count":              float64(1),
		"tracing_sampler":                         string(telemetry.TraceSamplerTraceIDRatio),
	} {
		if got := entry[key]; got != want {
			t.Fatalf("field %q mismatch: want %#v, got %#v", key, want, got)
		}
	}

	if _, ok := entry["database_url"]; ok {
		t.Fatal("startup configuration log must not include database_url")
	}
	if _, ok := entry["db_url"]; ok {
		t.Fatal("startup configuration log must not include db_url")
	}
}

func decodeJSONLogLines(t *testing.T, data string) []map[string]any {
	t.Helper()

	if strings.TrimSpace(data) == "" {
		return nil
	}

	decoder := json.NewDecoder(strings.NewReader(data))
	entries := make([]map[string]any, 0)
	for {
		entry := map[string]any{}
		if err := decoder.Decode(&entry); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			t.Fatalf("decode JSON log entry: %v", err)
		}

		entries = append(entries, entry)
	}

	return entries
}

func reserveTCPAddress(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on ephemeral port: %v", err)
	}
	defer func() { _ = ln.Close() }()

	return ln.Addr().String()
}

func lookupEnvFromMap(env map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	}
}

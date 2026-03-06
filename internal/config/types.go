package config

import (
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"github.com/abczzz13/base-api/internal/logger"
	"github.com/abczzz13/base-api/internal/telemetry"
)

type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           time.Duration
}

type CSRFConfig struct {
	TrustedOrigins []string
	Enabled        bool
}

type OTELConfig struct {
	TracingEnabled   bool
	ServiceName      string
	TracesSampler    telemetry.TraceSampler
	TracesSamplerArg *float64
}

type RequestAuditConfig struct {
	Enabled           *bool
	TrustedProxyCIDRs []netip.Prefix
}

func (c RequestAuditConfig) IsEnabled() bool {
	if c.Enabled == nil {
		return true
	}

	return *c.Enabled
}

type RequestLoggerConfig struct {
	Enabled *bool
}

func (c RequestLoggerConfig) IsEnabled() bool {
	if c.Enabled == nil {
		return true
	}

	return *c.Enabled
}

type DBConfig struct {
	URL                   string
	MinConns              int32
	MaxConns              int32
	MaxConnLifetime       time.Duration
	MaxConnIdleTime       time.Duration
	HealthCheckPeriod     time.Duration
	ConnectTimeout        time.Duration
	MigrateOnStartup      bool
	MigrateTimeout        time.Duration
	StartupMaxAttempts    int32
	StartupBackoffInitial time.Duration
	StartupBackoffMax     time.Duration
}

func (c DBConfig) Enabled() bool {
	return strings.TrimSpace(c.URL) != ""
}

func (c OTELConfig) TelemetryConfig(environment, serviceVersion string) telemetry.Config {
	return telemetry.Config{
		ServiceName:      c.ServiceName,
		ServiceVersion:   serviceVersion,
		Environment:      environment,
		TracesSampler:    c.TracesSampler,
		TracesSamplerArg: c.TracesSamplerArg,
	}
}

type Config struct {
	Address           string
	InfraAddress      string
	Environment       string
	LogFormat         logger.Format
	LogLevel          slog.Level
	ReadyzTimeout     time.Duration
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	CORS              CORSConfig
	CSRF              CSRFConfig
	RequestAudit      RequestAuditConfig
	RequestLogger     RequestLoggerConfig
	OTEL              OTELConfig
	DB                DBConfig
}

package config

import (
	"log/slog"
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
	OTEL              OTELConfig
}

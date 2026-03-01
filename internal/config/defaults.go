package config

import (
	"log/slog"
	"time"

	"github.com/abczzz13/base-api/internal/logger"
	"github.com/abczzz13/base-api/internal/telemetry"
)

const (
	defaultAddress           = "0.0.0.0:8080"
	defaultInfraAddress      = "127.0.0.1:9090"
	defaultEnvironment       = "development"
	defaultReadyzTimeout     = 2 * time.Second
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 15 * time.Second
	defaultWriteTimeout      = 30 * time.Second
	defaultIdleTimeout       = 60 * time.Second
	defaultCORSMaxAge        = 5 * time.Minute
)

func defaultConfig() Config {
	return Config{
		Address:           defaultAddress,
		InfraAddress:      defaultInfraAddress,
		Environment:       defaultEnvironment,
		LogFormat:         logger.DefaultFormat,
		LogLevel:          slog.LevelInfo,
		ReadyzTimeout:     defaultReadyzTimeout,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		ReadTimeout:       defaultReadTimeout,
		WriteTimeout:      defaultWriteTimeout,
		IdleTimeout:       defaultIdleTimeout,
		CORS: CORSConfig{
			MaxAge: defaultCORSMaxAge,
		},
		CSRF: CSRFConfig{
			Enabled: true,
		},
		OTEL: OTELConfig{
			TracesSampler: telemetry.DefaultTraceSampler,
		},
	}
}

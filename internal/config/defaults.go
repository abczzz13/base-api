package config

import (
	"log/slog"
	"time"

	"github.com/abczzz13/base-api/internal/logger"
	"github.com/abczzz13/base-api/internal/publicoas"
	"github.com/abczzz13/base-api/internal/ratelimit"
	"github.com/abczzz13/base-api/internal/telemetry"
	"github.com/abczzz13/base-api/internal/valkey"
)

const (
	defaultAddress                 = "0.0.0.0:8080"
	defaultInfraAddress            = "127.0.0.1:9090"
	defaultEnvironment             = "development"
	defaultReadyzTimeout           = 2 * time.Second
	defaultReadHeaderTimeout       = 5 * time.Second
	defaultReadTimeout             = 15 * time.Second
	defaultWriteTimeout            = 30 * time.Second
	defaultIdleTimeout             = 60 * time.Second
	defaultCORSMaxAge              = 5 * time.Minute
	defaultWeatherGeocodingBaseURL = "https://geocoding-api.open-meteo.com"
	defaultWeatherForecastBaseURL  = "https://api.open-meteo.com"
	defaultWeatherTimeout          = 5 * time.Second
	defaultRateLimitEnabled        = false
	defaultRateLimitFailOpen       = true
	defaultRateLimitTimeout        = 100 * time.Millisecond
	defaultRateLimitDefaultRPS     = 5.0
	defaultRateLimitDefaultBurst   = 10
	defaultRateLimitKeyPrefix      = ratelimit.DefaultKeyPrefix
	defaultDBMinConns              = int32(0)
	defaultDBMaxConns              = int32(20)
	defaultDBMaxConnLifetime       = time.Hour
	defaultDBMaxConnIdleTime       = 30 * time.Minute
	defaultDBHealthPeriod          = time.Minute
	defaultDBConnectTimeout        = 5 * time.Second
	defaultDBMigrateOnStartup      = true
	defaultDBMigrateTimeout        = 5 * time.Minute
	defaultDBStartupMaxAttempts    = int32(5)
	defaultDBStartupBackoffInitial = time.Second
	defaultDBStartupBackoffMax     = 30 * time.Second
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
		Valkey: valkey.Config{Mode: valkey.ModeStandalone},
		RateLimit: RateLimitConfig{
			Enabled:   defaultRateLimitEnabled,
			FailOpen:  defaultRateLimitFailOpen,
			Timeout:   defaultRateLimitTimeout,
			KeyPrefix: defaultRateLimitKeyPrefix,
			DefaultPolicy: ratelimit.Policy{
				RequestsPerSecond: defaultRateLimitDefaultRPS,
				Burst:             defaultRateLimitDefaultBurst,
			},
			RouteOverrides: map[string]ratelimit.RouteOverride{
				publicoas.GetHealthzOperation: {Disabled: true},
			},
		},
		OTEL: OTELConfig{
			TracesSampler: telemetry.DefaultTraceSampler,
		},
		Weather: WeatherConfig{
			GeocodingBaseURL: defaultWeatherGeocodingBaseURL,
			ForecastBaseURL:  defaultWeatherForecastBaseURL,
			Timeout:          defaultWeatherTimeout,
		},
		DB: DBConfig{
			MinConns:              defaultDBMinConns,
			MaxConns:              defaultDBMaxConns,
			MaxConnLifetime:       defaultDBMaxConnLifetime,
			MaxConnIdleTime:       defaultDBMaxConnIdleTime,
			HealthCheckPeriod:     defaultDBHealthPeriod,
			ConnectTimeout:        defaultDBConnectTimeout,
			MigrateOnStartup:      defaultDBMigrateOnStartup,
			MigrateTimeout:        defaultDBMigrateTimeout,
			StartupMaxAttempts:    defaultDBStartupMaxAttempts,
			StartupBackoffInitial: defaultDBStartupBackoffInitial,
			StartupBackoffMax:     defaultDBStartupBackoffMax,
		},
	}
}

package config

import (
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/abczzz13/base-api/internal/logger"
	"github.com/abczzz13/base-api/internal/telemetry"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want Config
	}{
		{
			name: "uses defaults when env is empty",
			env:  map[string]string{},
			want: defaultConfig(),
		},
		{
			name: "uses explicit values",
			env: map[string]string{
				keyAPIAddr:                 "0.0.0.0:8081",
				keyAPIInfraAddr:            "127.0.0.1:9191",
				keyAPIEnvironment:          "production",
				keyLogFormat:               "json",
				keyLogLevel:                "debug",
				keyAPIReadyzTimeout:        "750ms",
				keyAPIReadHeaderTimeout:    "3s",
				keyAPIReadTimeout:          "11s",
				keyAPIWriteTimeout:         "25s",
				keyAPIIdleTimeout:          "45s",
				keyAPIRequestAuditEnabled:  "false",
				keyAPIRequestLoggerEnabled: "false",
				keyWeatherEnabled:          "true",
				keyWeatherGeocodingBaseURL: "https://geocoding-api.weather.example",
				keyWeatherForecastBaseURL:  "https://forecast.weather.example",
				keyWeatherAPIKey:           "super-secret",
				keyWeatherTimeout:          "4s",
				keyDBURL:                   "postgres://base@127.0.0.1:5432/base_api?sslmode=disable",
				keyDBMinConns:              "2",
				keyDBMaxConns:              "40",
				keyDBMaxConnLifetime:       "2h",
				keyDBMaxConnIdleTime:       "20m",
				keyDBHealthCheckPeriod:     "45s",
				keyDBConnectTimeout:        "7s",
				keyDBMigrateOnStartup:      "false",
				keyDBMigrateTimeout:        "2m",
				keyDBStartupMaxAttempts:    "7",
				keyDBStartupBackoffInitial: "2s",
				keyDBStartupBackoffMax:     "20s",
			},
			want: func() Config {
				cfg := defaultConfig()
				cfg.Address = "0.0.0.0:8081"
				cfg.InfraAddress = "127.0.0.1:9191"
				cfg.Environment = "production"
				cfg.LogFormat = logger.FormatJSON
				cfg.LogLevel = slog.LevelDebug
				cfg.ReadyzTimeout = 750 * time.Millisecond
				cfg.ReadHeaderTimeout = 3 * time.Second
				cfg.ReadTimeout = 11 * time.Second
				cfg.WriteTimeout = 25 * time.Second
				cfg.IdleTimeout = 45 * time.Second
				cfg.RequestAudit.Enabled = boolPtr(false)
				cfg.RequestLogger.Enabled = boolPtr(false)
				cfg.Weather.IntegrationEnabled = true
				cfg.Weather.GeocodingBaseURL = "https://geocoding-api.weather.example"
				cfg.Weather.ForecastBaseURL = "https://forecast.weather.example"
				cfg.Weather.APIKey = "super-secret"
				cfg.Weather.Timeout = 4 * time.Second
				cfg.DB.URL = "postgres://base@127.0.0.1:5432/base_api?sslmode=disable"
				cfg.DB.MinConns = 2
				cfg.DB.MaxConns = 40
				cfg.DB.MaxConnLifetime = 2 * time.Hour
				cfg.DB.MaxConnIdleTime = 20 * time.Minute
				cfg.DB.HealthCheckPeriod = 45 * time.Second
				cfg.DB.ConnectTimeout = 7 * time.Second
				cfg.DB.MigrateOnStartup = false
				cfg.DB.MigrateTimeout = 2 * time.Minute
				cfg.DB.StartupMaxAttempts = 7
				cfg.DB.StartupBackoffInitial = 2 * time.Second
				cfg.DB.StartupBackoffMax = 20 * time.Second
				return cfg
			}(),
		},
		{
			name: "does not use legacy environment aliases",
			env: map[string]string{
				"APP_ENV":     "qa",
				"ENVIRONMENT": "sandbox",
			},
			want: defaultConfig(),
		},
		{
			name: "parses and normalizes CORS and CSRF configuration",
			env: map[string]string{
				keyAPICORSAllowedOrigins:   " https://Example.com , * ,https://example.com/ ",
				keyAPICORSAllowedMethods:   " GET , POST ",
				keyAPICORSAllowedHeaders:   " Content-Type , Authorization ",
				keyAPICORSExposedHeaders:   " X-Request-Id ",
				keyAPICORSAllowCredentials: "false",
				keyAPICORSMaxAge:           "10m",
				keyAPICSRFTrustedOrigins:   " https://Trusted.com , https://trusted.com/ ",
				keyAPICSRFEnabled:          "false",
			},
			want: func() Config {
				cfg := defaultConfig()
				cfg.CORS.AllowedOrigins = []string{"https://example.com", "*"}
				cfg.CORS.AllowedMethods = []string{"GET", "POST"}
				cfg.CORS.AllowedHeaders = []string{"Content-Type", "Authorization"}
				cfg.CORS.ExposedHeaders = []string{"X-Request-Id"}
				cfg.CORS.AllowCredentials = false
				cfg.CORS.MaxAge = 10 * time.Minute
				cfg.CSRF.TrustedOrigins = []string{"https://trusted.com"}
				cfg.CSRF.Enabled = false
				return cfg
			}(),
		},
		{
			name: "parses request audit trusted proxy CIDRs",
			env: map[string]string{
				keyAPIRequestAuditTrustedProxyCIDRs: "10.0.0.0/8, 2001:db8::/32, 10.0.0.0/8",
			},
			want: func() Config {
				cfg := defaultConfig()
				cfg.RequestAudit.TrustedProxyCIDRs = []netip.Prefix{
					netip.MustParsePrefix("10.0.0.0/8"),
					netip.MustParsePrefix("2001:db8::/32"),
				}
				return cfg
			}(),
		},
		{
			name: "ignores weather provider overrides when integration is disabled",
			env: map[string]string{
				keyWeatherGeocodingBaseURL: "weather.example/api",
				keyWeatherForecastBaseURL:  "forecast.weather.example/v1",
				keyWeatherTimeout:          "7s",
			},
			want: func() Config {
				cfg := defaultConfig()
				cfg.Weather.GeocodingBaseURL = "weather.example/api"
				cfg.Weather.ForecastBaseURL = "forecast.weather.example/v1"
				cfg.Weather.Timeout = 7 * time.Second
				return cfg
			}(),
		},
		{
			name: "parses OTEL configuration",
			env: map[string]string{
				keyOTELServiceName:                "  base-api-custom  ",
				keyOTELTracesSampler:              "traceidratio",
				keyOTELTracesSamplerArg:           "0.25",
				keyOTELExporterOTLPEndpoint:       "http://localhost:4318",
				keyOTELExporterOTLPTracesEndpoint: "http://localhost:4318/v1/traces",
			},
			want: func() Config {
				cfg := defaultConfig()
				cfg.OTEL.ServiceName = "base-api-custom"
				cfg.OTEL.TracingEnabled = true
				cfg.OTEL.TracesSampler = telemetry.TraceSamplerTraceIDRatio
				cfg.OTEL.TracesSamplerArg = float64Ptr(0.25)
				return cfg
			}(),
		},
		{
			name: "ignores sampler configuration when tracing is not enabled",
			env: map[string]string{
				keyOTELTracesSampler:    "always_on",
				keyOTELTracesSamplerArg: "not-a-float",
			},
			want: defaultConfig(),
		},
		{
			name: "ignores sampler configuration when OTEL SDK is disabled",
			env: map[string]string{
				keyOTELSDKDisabled:          "true",
				keyOTELExporterOTLPEndpoint: "http://localhost:4318",
				keyOTELTracesSampler:        "always_on",
				keyOTELTracesSamplerArg:     "not-a-float",
			},
			want: defaultConfig(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Load(lookupEnvFromMap(tt.env))
			if err != nil {
				t.Fatalf("Load returned error: %v", err)
			}

			if diff := cmp.Diff(tt.want, got, cmpopts.EquateComparable(netip.Prefix{})); diff != "" {
				t.Fatalf("Load mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadDBConnectTimeoutDefaultsWhenOnlyURLConnectTimeoutIsSet(t *testing.T) {
	cfg, err := Load(lookupEnvFromMap(map[string]string{
		keyDBURL: "postgres://base@127.0.0.1:5432/base_api?sslmode=disable&connect_timeout=13",
	}))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.DB.ConnectTimeout != defaultDBConnectTimeout {
		t.Fatalf(
			"DB connect timeout mismatch: want default %s, got %s",
			defaultDBConnectTimeout,
			cfg.DB.ConnectTimeout,
		)
	}
}

func TestLoadDBConnectTimeoutAllowsExplicitZeroToDisableOverride(t *testing.T) {
	cfg, err := Load(lookupEnvFromMap(map[string]string{
		keyDBURL:            "postgres://base@127.0.0.1:5432/base_api?sslmode=disable&connect_timeout=13",
		keyDBConnectTimeout: "0s",
	}))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.DB.ConnectTimeout != 0 {
		t.Fatalf("DB connect timeout mismatch: want 0s, got %s", cfg.DB.ConnectTimeout)
	}
}

func TestLoadStrictValidationFailures(t *testing.T) {
	tests := []struct {
		name         string
		env          map[string]string
		wantContains []string
	}{
		{
			name: "invalid durations fail",
			env: map[string]string{
				keyAPIReadyzTimeout: "not-a-duration",
			},
			wantContains: []string{"invalid duration for API_READYZ_TIMEOUT=\"not-a-duration\""},
		},
		{
			name: "non-positive durations fail",
			env: map[string]string{
				keyAPIReadTimeout: "0s",
			},
			wantContains: []string{"non-positive duration for API_READ_TIMEOUT=\"0s\""},
		},
		{
			name: "invalid log format fails",
			env: map[string]string{
				keyLogFormat: "xml",
			},
			wantContains: []string{"invalid log format \"xml\" for LOG_FORMAT"},
		},
		{
			name: "invalid log level fails",
			env: map[string]string{
				keyLogLevel: "trace",
			},
			wantContains: []string{"invalid log level \"trace\" for LOG_LEVEL"},
		},
		{
			name: "invalid CORS allow credentials boolean fails",
			env: map[string]string{
				keyAPICORSAllowCredentials: "maybe",
			},
			wantContains: []string{"invalid boolean for API_CORS_ALLOW_CREDENTIALS=\"maybe\""},
		},
		{
			name: "invalid CSRF enabled boolean fails",
			env: map[string]string{
				keyAPICSRFEnabled: "sure",
			},
			wantContains: []string{"invalid boolean for API_CSRF_ENABLED=\"sure\""},
		},
		{
			name: "invalid OTEL SDK disabled boolean fails",
			env: map[string]string{
				keyOTELSDKDisabled: "not-a-bool",
			},
			wantContains: []string{"invalid boolean for OTEL_SDK_DISABLED=\"not-a-bool\""},
		},
		{
			name: "invalid request audit trusted proxy CIDRs fail",
			env: map[string]string{
				keyAPIRequestAuditTrustedProxyCIDRs: "10.0.0.0/8,not-a-cidr",
			},
			wantContains: []string{"invalid CIDR \"not-a-cidr\" for API_REQUEST_AUDIT_TRUSTED_PROXY_CIDRS"},
		},
		{
			name: "invalid request audit enabled boolean fails",
			env: map[string]string{
				keyAPIRequestAuditEnabled: "sure",
			},
			wantContains: []string{"invalid boolean for API_REQUEST_AUDIT_ENABLED=\"sure\""},
		},
		{
			name: "invalid request logger enabled boolean fails",
			env: map[string]string{
				keyAPIRequestLoggerEnabled: "maybe",
			},
			wantContains: []string{"invalid boolean for API_REQUEST_LOGGER_ENABLED=\"maybe\""},
		},
		{
			name: "invalid CORS and CSRF origins fail",
			env: map[string]string{
				keyAPICORSAllowedOrigins: "https://ok.example,not-an-origin",
				keyAPICSRFTrustedOrigins: "https://trusted.example/path",
			},
			wantContains: []string{
				"invalid origin \"not-an-origin\" for API_CORS_ALLOWED_ORIGINS",
				"invalid origin \"https://trusted.example/path\" for API_CSRF_TRUSTED_ORIGINS",
			},
		},
		{
			name: "wildcard origins with credentials fail",
			env: map[string]string{
				keyAPICORSAllowedOrigins:   "*",
				keyAPICORSAllowCredentials: "true",
			},
			wantContains: []string{"cannot be combined with wildcard API_CORS_ALLOWED_ORIGINS=\"*\""},
		},
		{
			name: "configured CORS with enabled CSRF requires trusted origins",
			env: map[string]string{
				keyAPICORSAllowedOrigins: "https://client.example",
			},
			wantContains: []string{"API_CSRF_ENABLED is enabled and API_CORS_ALLOWED_ORIGINS is configured, but API_CSRF_TRUSTED_ORIGINS is empty"},
		},
		{
			name: "sampler arg without ratio sampler fails",
			env: map[string]string{
				keyOTELExporterOTLPEndpoint: "http://localhost:4318",
				keyOTELTracesSampler:        "always_on",
				keyOTELTracesSamplerArg:     "0.5",
			},
			wantContains: []string{"OTEL_TRACES_SAMPLER_ARG is set but OTEL_TRACES_SAMPLER=\"always_on\" does not use a sampler argument"},
		},
		{
			name: "invalid sampler arg fails",
			env: map[string]string{
				keyOTELExporterOTLPEndpoint: "http://localhost:4318",
				keyOTELTracesSampler:        "traceidratio",
				keyOTELTracesSamplerArg:     "not-a-float",
			},
			wantContains: []string{"invalid float for OTEL_TRACES_SAMPLER_ARG=\"not-a-float\""},
		},
		{
			name: "out of range sampler arg fails",
			env: map[string]string{
				keyOTELExporterOTLPEndpoint: "http://localhost:4318",
				keyOTELTracesSampler:        "traceidratio",
				keyOTELTracesSamplerArg:     "1.5",
			},
			wantContains: []string{"out-of-range float for OTEL_TRACES_SAMPLER_ARG=\"1.5\", expected value between 0 and 1 inclusive"},
		},
		{
			name: "invalid address fails",
			env: map[string]string{
				keyAPIAddr: "invalid-address",
			},
			wantContains: []string{"invalid API_ADDR=\"invalid-address\""},
		},
		{
			name: "invalid weather enabled boolean fails",
			env: map[string]string{
				keyWeatherEnabled: "sure",
			},
			wantContains: []string{"invalid boolean for WEATHER_ENABLED=\"sure\""},
		},
		{
			name: "weather geocoding base URL must be absolute origin",
			env: map[string]string{
				keyWeatherEnabled:          "true",
				keyWeatherGeocodingBaseURL: "weather.example/api",
				keyWeatherForecastBaseURL:  "https://forecast.weather.example",
			},
			wantContains: []string{"invalid WEATHER_GEOCODING_BASE_URL=\"weather.example/api\": URL must be absolute"},
		},
		{
			name: "weather forecast base URL must be absolute origin",
			env: map[string]string{
				keyWeatherEnabled:          "true",
				keyWeatherGeocodingBaseURL: "https://geocoding-api.weather.example",
				keyWeatherForecastBaseURL:  "forecast.weather.example/v1",
			},
			wantContains: []string{"invalid WEATHER_FORECAST_BASE_URL=\"forecast.weather.example/v1\": URL must be absolute"},
		},
		{
			name: "invalid DB max connections fails",
			env: map[string]string{
				keyDBMaxConns: "not-an-int",
			},
			wantContains: []string{"invalid integer for DB_MAX_CONNS=\"not-an-int\""},
		},
		{
			name: "invalid DB min connections fails",
			env: map[string]string{
				keyDBMinConns: "-1",
			},
			wantContains: []string{"negative integer for DB_MIN_CONNS=-1"},
		},
		{
			name: "invalid DB migrate timeout fails",
			env: map[string]string{
				keyDBMigrateTimeout: "0s",
			},
			wantContains: []string{"non-positive duration for DB_MIGRATE_TIMEOUT=\"0s\""},
		},
		{
			name: "negative DB connect timeout fails",
			env: map[string]string{
				keyDBConnectTimeout: "-1s",
			},
			wantContains: []string{"negative duration for DB_CONNECT_TIMEOUT=\"-1s\""},
		},
		{
			name: "invalid DB migrate on startup boolean fails",
			env: map[string]string{
				keyDBMigrateOnStartup: "sometimes",
			},
			wantContains: []string{"invalid boolean for DB_MIGRATE_ON_STARTUP=\"sometimes\""},
		},
		{
			name: "invalid DB startup max attempts fails",
			env: map[string]string{
				keyDBStartupMaxAttempts: "0",
			},
			wantContains: []string{"non-positive integer for DB_STARTUP_MAX_ATTEMPTS=0"},
		},
		{
			name: "invalid DB startup backoff initial fails",
			env: map[string]string{
				keyDBStartupBackoffInitial: "0s",
			},
			wantContains: []string{"non-positive duration for DB_STARTUP_BACKOFF_INITIAL=\"0s\""},
		},
		{
			name: "invalid DB startup backoff max fails",
			env: map[string]string{
				keyDBStartupBackoffMax: "-1s",
			},
			wantContains: []string{"non-positive duration for DB_STARTUP_BACKOFF_MAX=\"-1s\""},
		},
		{
			name: "invalid DB startup backoff ordering fails",
			env: map[string]string{
				keyDBStartupBackoffInitial: "5s",
				keyDBStartupBackoffMax:     "2s",
			},
			wantContains: []string{"invalid database startup retry configuration: DB_STARTUP_BACKOFF_INITIAL=5s cannot exceed DB_STARTUP_BACKOFF_MAX=2s"},
		},
		{
			name: "DB min connections above max fails",
			env: map[string]string{
				keyDBMinConns: "50",
				keyDBMaxConns: "10",
			},
			wantContains: []string{"invalid database pool configuration: DB_MIN_CONNS=50 cannot exceed DB_MAX_CONNS=10"},
		},
		{
			name: "aggregates multiple failures",
			env: map[string]string{
				keyLogFormat:        "xml",
				keyAPIReadyzTimeout: "bad",
			},
			wantContains: []string{
				"invalid log format \"xml\" for LOG_FORMAT",
				"invalid duration for API_READYZ_TIMEOUT=\"bad\"",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(lookupEnvFromMap(tt.env))
			if err == nil {
				t.Fatal("Load returned nil error")
			}

			errMessage := err.Error()
			for _, want := range tt.wantContains {
				if !strings.Contains(errMessage, want) {
					t.Fatalf("error %q does not contain %q", errMessage, want)
				}
			}
		})
	}
}

func TestLoadSupportsFileBackedValuesForAllKeys(t *testing.T) {
	dir := t.TempDir()

	values := map[string]string{
		keyAPIAddr:                          "127.0.0.1:8081",
		keyAPIInfraAddr:                     "127.0.0.1:9191",
		keyAPIEnvironment:                   "production",
		keyLogFormat:                        "json",
		keyLogLevel:                         "debug",
		keyAPIReadyzTimeout:                 "750ms",
		keyAPIReadHeaderTimeout:             "3s",
		keyAPIReadTimeout:                   "11s",
		keyAPIWriteTimeout:                  "25s",
		keyAPIIdleTimeout:                   "45s",
		keyAPICORSAllowedOrigins:            "https://client.example",
		keyAPICORSAllowedMethods:            "GET,POST",
		keyAPICORSAllowedHeaders:            "Content-Type,Authorization",
		keyAPICORSExposedHeaders:            "X-Request-Id",
		keyAPICORSAllowCredentials:          "true",
		keyAPICORSMaxAge:                    "10m",
		keyAPICSRFTrustedOrigins:            "https://client.example",
		keyAPICSRFEnabled:                   "true",
		keyAPIRequestAuditEnabled:           "false",
		keyAPIRequestAuditTrustedProxyCIDRs: "10.0.0.0/8,192.168.0.0/16",
		keyAPIRequestLoggerEnabled:          "false",
		keyWeatherEnabled:                   "true",
		keyWeatherGeocodingBaseURL:          "https://geocoding-api.weather.example",
		keyWeatherForecastBaseURL:           "https://forecast.weather.example",
		keyWeatherAPIKey:                    "weather-secret",
		keyWeatherTimeout:                   "4s",
		keyOTELServiceName:                  "base-api-custom",
		keyOTELSDKDisabled:                  "false",
		keyOTELTracesSampler:                "traceidratio",
		keyOTELTracesSamplerArg:             "0.25",
		keyOTELExporterOTLPEndpoint:         "http://localhost:4318",
		keyOTELExporterOTLPTracesEndpoint:   "http://localhost:4318/v1/traces",
		keyDBURL:                            "postgres://base@127.0.0.1:5432/base_api?sslmode=disable",
		keyDBMinConns:                       "2",
		keyDBMaxConns:                       "40",
		keyDBMaxConnLifetime:                "2h",
		keyDBMaxConnIdleTime:                "20m",
		keyDBHealthCheckPeriod:              "45s",
		keyDBConnectTimeout:                 "7s",
		keyDBMigrateOnStartup:               "false",
		keyDBMigrateTimeout:                 "2m",
		keyDBStartupMaxAttempts:             "9",
		keyDBStartupBackoffInitial:          "3s",
		keyDBStartupBackoffMax:              "40s",
	}

	env := make(map[string]string, len(values))
	for key, value := range values {
		fileName := strings.ToLower(strings.ReplaceAll(key, "_", "-")) + ".txt"
		filePath := filepath.Join(dir, fileName)
		if err := os.WriteFile(filePath, []byte(value+"\n"), 0o600); err != nil {
			t.Fatalf("write %s file: %v", key, err)
		}
		env[fileEnvKey(key)] = filePath
	}

	got, err := Load(lookupEnvFromMap(env))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	want := defaultConfig()
	want.Address = "127.0.0.1:8081"
	want.InfraAddress = "127.0.0.1:9191"
	want.Environment = "production"
	want.LogFormat = logger.FormatJSON
	want.LogLevel = slog.LevelDebug
	want.ReadyzTimeout = 750 * time.Millisecond
	want.ReadHeaderTimeout = 3 * time.Second
	want.ReadTimeout = 11 * time.Second
	want.WriteTimeout = 25 * time.Second
	want.IdleTimeout = 45 * time.Second
	want.CORS.AllowedOrigins = []string{"https://client.example"}
	want.CORS.AllowedMethods = []string{"GET", "POST"}
	want.CORS.AllowedHeaders = []string{"Content-Type", "Authorization"}
	want.CORS.ExposedHeaders = []string{"X-Request-Id"}
	want.CORS.AllowCredentials = true
	want.CORS.MaxAge = 10 * time.Minute
	want.CSRF.TrustedOrigins = []string{"https://client.example"}
	want.CSRF.Enabled = true
	want.RequestAudit.Enabled = boolPtr(false)
	want.RequestAudit.TrustedProxyCIDRs = []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("192.168.0.0/16"),
	}
	want.RequestLogger.Enabled = boolPtr(false)
	want.Weather.IntegrationEnabled = true
	want.Weather.GeocodingBaseURL = "https://geocoding-api.weather.example"
	want.Weather.ForecastBaseURL = "https://forecast.weather.example"
	want.Weather.APIKey = "weather-secret"
	want.Weather.Timeout = 4 * time.Second
	want.OTEL.ServiceName = "base-api-custom"
	want.OTEL.TracingEnabled = true
	want.OTEL.TracesSampler = telemetry.TraceSamplerTraceIDRatio
	want.OTEL.TracesSamplerArg = float64Ptr(0.25)
	want.DB.URL = "postgres://base@127.0.0.1:5432/base_api?sslmode=disable"
	want.DB.MinConns = 2
	want.DB.MaxConns = 40
	want.DB.MaxConnLifetime = 2 * time.Hour
	want.DB.MaxConnIdleTime = 20 * time.Minute
	want.DB.HealthCheckPeriod = 45 * time.Second
	want.DB.ConnectTimeout = 7 * time.Second
	want.DB.MigrateOnStartup = false
	want.DB.MigrateTimeout = 2 * time.Minute
	want.DB.StartupMaxAttempts = 9
	want.DB.StartupBackoffInitial = 3 * time.Second
	want.DB.StartupBackoffMax = 40 * time.Second

	if diff := cmp.Diff(want, got, cmpopts.EquateComparable(netip.Prefix{})); diff != "" {
		t.Fatalf("Load mismatch (-want +got):\n%s", diff)
	}
}

func TestLoadUsesFileValueWhenEnvValueIsEmpty(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "log-level.txt")
	if err := os.WriteFile(filePath, []byte("debug\n"), 0o600); err != nil {
		t.Fatalf("write log level file: %v", err)
	}

	got, err := Load(lookupEnvFromMap(map[string]string{
		keyLogLevel:             "",
		fileEnvKey(keyLogLevel): filePath,
	}))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if got.LogLevel != slog.LevelDebug {
		t.Fatalf("LogLevel = %v, want %v", got.LogLevel, slog.LevelDebug)
	}
}

func TestLoadUsesEnvValueWhenFilePathIsEmpty(t *testing.T) {
	got, err := Load(lookupEnvFromMap(map[string]string{
		keyLogLevel:             "debug",
		fileEnvKey(keyLogLevel): "   ",
	}))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if got.LogLevel != slog.LevelDebug {
		t.Fatalf("LogLevel = %v, want %v", got.LogLevel, slog.LevelDebug)
	}
}

func TestLoadFailsWhenEnvAndFileKeysAreBothSet(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "log-level.txt")
	if err := os.WriteFile(filePath, []byte("info\n"), 0o600); err != nil {
		t.Fatalf("write log level file: %v", err)
	}

	_, err := Load(lookupEnvFromMap(map[string]string{
		keyLogLevel:             "debug",
		fileEnvKey(keyLogLevel): filePath,
	}))
	if err == nil {
		t.Fatal("Load returned nil error")
	}

	if want := "both LOG_LEVEL and LOG_LEVEL_FILE are set"; !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}

func TestLoadFailsWhenFileValueCannotBeRead(t *testing.T) {
	_, err := Load(lookupEnvFromMap(map[string]string{
		fileEnvKey(keyLogLevel): filepath.Join(t.TempDir(), "missing-log-level.txt"),
	}))
	if err == nil {
		t.Fatal("Load returned nil error")
	}

	if want := "read LOG_LEVEL from LOG_LEVEL_FILE"; !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}

func TestLoadFailsWhenFilePathIsEmpty(t *testing.T) {
	_, err := Load(lookupEnvFromMap(map[string]string{
		fileEnvKey(keyLogLevel): "   ",
	}))
	if err == nil {
		t.Fatal("Load returned nil error")
	}

	if want := "invalid LOG_LEVEL_FILE: file path is empty"; !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}

func lookupEnvFromMap(env map[string]string) LookupEnvFunc {
	return func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	}
}

func float64Ptr(value float64) *float64 {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

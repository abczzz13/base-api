package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

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
				keyAPIAddr:              "0.0.0.0:8081",
				keyAPIInfraAddr:         "127.0.0.1:9191",
				keyAPIEnvironment:       "production",
				keyLogFormat:            "json",
				keyLogLevel:             "debug",
				keyAPIReadyzTimeout:     "750ms",
				keyAPIReadHeaderTimeout: "3s",
				keyAPIReadTimeout:       "11s",
				keyAPIWriteTimeout:      "25s",
				keyAPIIdleTimeout:       "45s",
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

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("Load mismatch (-want +got):\n%s", diff)
			}
		})
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
		keyAPIAddr:                        "127.0.0.1:8081",
		keyAPIInfraAddr:                   "127.0.0.1:9191",
		keyAPIEnvironment:                 "production",
		keyLogFormat:                      "json",
		keyLogLevel:                       "debug",
		keyAPIReadyzTimeout:               "750ms",
		keyAPIReadHeaderTimeout:           "3s",
		keyAPIReadTimeout:                 "11s",
		keyAPIWriteTimeout:                "25s",
		keyAPIIdleTimeout:                 "45s",
		keyAPICORSAllowedOrigins:          "https://client.example",
		keyAPICORSAllowedMethods:          "GET,POST",
		keyAPICORSAllowedHeaders:          "Content-Type,Authorization",
		keyAPICORSExposedHeaders:          "X-Request-Id",
		keyAPICORSAllowCredentials:        "true",
		keyAPICORSMaxAge:                  "10m",
		keyAPICSRFTrustedOrigins:          "https://client.example",
		keyAPICSRFEnabled:                 "true",
		keyOTELServiceName:                "base-api-custom",
		keyOTELSDKDisabled:                "false",
		keyOTELTracesSampler:              "traceidratio",
		keyOTELTracesSamplerArg:           "0.25",
		keyOTELExporterOTLPEndpoint:       "http://localhost:4318",
		keyOTELExporterOTLPTracesEndpoint: "http://localhost:4318/v1/traces",
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
	want.OTEL.ServiceName = "base-api-custom"
	want.OTEL.TracingEnabled = true
	want.OTEL.TracesSampler = telemetry.TraceSamplerTraceIDRatio
	want.OTEL.TracesSamplerArg = float64Ptr(0.25)

	if diff := cmp.Diff(want, got); diff != "" {
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

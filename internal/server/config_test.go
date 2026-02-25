package server

import (
	"log/slog"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want Config
	}{
		{
			name: "uses defaults when env is empty",
			env:  map[string]string{},
			want: Config{
				Address:           "0.0.0.0:8080",
				InfraAddress:      "127.0.0.1:9090",
				Environment:       "development",
				LogFormat:         "text",
				LogLevel:          slog.LevelInfo,
				ReadyzTimeout:     2 * time.Second,
				ReadHeaderTimeout: defaultReadHeaderTimeout,
				ReadTimeout:       defaultReadTimeout,
				WriteTimeout:      defaultWriteTimeout,
				IdleTimeout:       defaultIdleTimeout,
				CORS: CORSConfig{
					MaxAge: 5 * time.Minute,
				},
				CSRF: CSRFConfig{
					Enabled: true,
				},
			},
		},
		{
			name: "uses explicit api values",
			env: map[string]string{
				"API_ADDR":                "0.0.0.0:8081",
				"API_INFRA_ADDR":          "127.0.0.1:9191",
				"API_ENVIRONMENT":         "production",
				"APP_ENV":                 "staging",
				"ENVIRONMENT":             "ignored",
				"API_READYZ_TIMEOUT":      "750ms",
				"API_READ_HEADER_TIMEOUT": "3s",
				"API_READ_TIMEOUT":        "11s",
				"API_WRITE_TIMEOUT":       "25s",
				"API_IDLE_TIMEOUT":        "45s",
			},
			want: Config{
				Address:           "0.0.0.0:8081",
				InfraAddress:      "127.0.0.1:9191",
				Environment:       "production",
				LogFormat:         "text",
				LogLevel:          slog.LevelInfo,
				ReadyzTimeout:     750 * time.Millisecond,
				ReadHeaderTimeout: 3 * time.Second,
				ReadTimeout:       11 * time.Second,
				WriteTimeout:      25 * time.Second,
				IdleTimeout:       45 * time.Second,
				CORS: CORSConfig{
					MaxAge: 5 * time.Minute,
				},
				CSRF: CSRFConfig{
					Enabled: true,
				},
			},
		},
		{
			name: "falls back to app env",
			env: map[string]string{
				"APP_ENV": "qa",
			},
			want: Config{
				Address:           "0.0.0.0:8080",
				InfraAddress:      "127.0.0.1:9090",
				Environment:       "qa",
				LogFormat:         "text",
				LogLevel:          slog.LevelInfo,
				ReadyzTimeout:     2 * time.Second,
				ReadHeaderTimeout: defaultReadHeaderTimeout,
				ReadTimeout:       defaultReadTimeout,
				WriteTimeout:      defaultWriteTimeout,
				IdleTimeout:       defaultIdleTimeout,
				CORS: CORSConfig{
					MaxAge: 5 * time.Minute,
				},
				CSRF: CSRFConfig{
					Enabled: true,
				},
			},
		},
		{
			name: "falls back to environment env",
			env: map[string]string{
				"ENVIRONMENT": "sandbox",
			},
			want: Config{
				Address:           "0.0.0.0:8080",
				InfraAddress:      "127.0.0.1:9090",
				Environment:       "sandbox",
				LogFormat:         "text",
				LogLevel:          slog.LevelInfo,
				ReadyzTimeout:     2 * time.Second,
				ReadHeaderTimeout: defaultReadHeaderTimeout,
				ReadTimeout:       defaultReadTimeout,
				WriteTimeout:      defaultWriteTimeout,
				IdleTimeout:       defaultIdleTimeout,
				CORS: CORSConfig{
					MaxAge: 5 * time.Minute,
				},
				CSRF: CSRFConfig{
					Enabled: true,
				},
			},
		},
		{
			name: "invalid readyz timeout falls back to default",
			env: map[string]string{
				"API_READYZ_TIMEOUT": "not-a-duration",
			},
			want: Config{
				Address:           "0.0.0.0:8080",
				InfraAddress:      "127.0.0.1:9090",
				Environment:       "development",
				LogFormat:         "text",
				LogLevel:          slog.LevelInfo,
				ReadyzTimeout:     2 * time.Second,
				ReadHeaderTimeout: defaultReadHeaderTimeout,
				ReadTimeout:       defaultReadTimeout,
				WriteTimeout:      defaultWriteTimeout,
				IdleTimeout:       defaultIdleTimeout,
				CORS: CORSConfig{
					MaxAge: 5 * time.Minute,
				},
				CSRF: CSRFConfig{
					Enabled: true,
				},
			},
		},
		{
			name: "non positive readyz timeout falls back to default",
			env: map[string]string{
				"API_READYZ_TIMEOUT": "-1s",
			},
			want: Config{
				Address:           "0.0.0.0:8080",
				InfraAddress:      "127.0.0.1:9090",
				Environment:       "development",
				LogFormat:         "text",
				LogLevel:          slog.LevelInfo,
				ReadyzTimeout:     2 * time.Second,
				ReadHeaderTimeout: defaultReadHeaderTimeout,
				ReadTimeout:       defaultReadTimeout,
				WriteTimeout:      defaultWriteTimeout,
				IdleTimeout:       defaultIdleTimeout,
				CORS: CORSConfig{
					MaxAge: 5 * time.Minute,
				},
				CSRF: CSRFConfig{
					Enabled: true,
				},
			},
		},
		{
			name: "invalid server timeouts fall back to defaults",
			env: map[string]string{
				"API_READ_HEADER_TIMEOUT": "0s",
				"API_READ_TIMEOUT":        "invalid",
				"API_WRITE_TIMEOUT":       "-2s",
				"API_IDLE_TIMEOUT":        "",
			},
			want: Config{
				Address:           "0.0.0.0:8080",
				InfraAddress:      "127.0.0.1:9090",
				Environment:       "development",
				LogFormat:         "text",
				LogLevel:          slog.LevelInfo,
				ReadyzTimeout:     2 * time.Second,
				ReadHeaderTimeout: defaultReadHeaderTimeout,
				ReadTimeout:       defaultReadTimeout,
				WriteTimeout:      defaultWriteTimeout,
				IdleTimeout:       defaultIdleTimeout,
				CORS: CORSConfig{
					MaxAge: 5 * time.Minute,
				},
				CSRF: CSRFConfig{
					Enabled: true,
				},
			},
		},
		{
			name: "valid log format and level",
			env: map[string]string{
				"LOG_FORMAT": "json",
				"LOG_LEVEL":  "debug",
			},
			want: Config{
				Address:           "0.0.0.0:8080",
				InfraAddress:      "127.0.0.1:9090",
				Environment:       "development",
				LogFormat:         "json",
				LogLevel:          slog.LevelDebug,
				ReadyzTimeout:     2 * time.Second,
				ReadHeaderTimeout: defaultReadHeaderTimeout,
				ReadTimeout:       defaultReadTimeout,
				WriteTimeout:      defaultWriteTimeout,
				IdleTimeout:       defaultIdleTimeout,
				CORS: CORSConfig{
					MaxAge: 5 * time.Minute,
				},
				CSRF: CSRFConfig{
					Enabled: true,
				},
			},
		},
		{
			name: "invalid log format falls back to default",
			env: map[string]string{
				"LOG_FORMAT": "xml",
			},
			want: Config{
				Address:           "0.0.0.0:8080",
				InfraAddress:      "127.0.0.1:9090",
				Environment:       "development",
				LogFormat:         "text",
				LogLevel:          slog.LevelInfo,
				ReadyzTimeout:     2 * time.Second,
				ReadHeaderTimeout: defaultReadHeaderTimeout,
				ReadTimeout:       defaultReadTimeout,
				WriteTimeout:      defaultWriteTimeout,
				IdleTimeout:       defaultIdleTimeout,
				CORS: CORSConfig{
					MaxAge: 5 * time.Minute,
				},
				CSRF: CSRFConfig{
					Enabled: true,
				},
			},
		},
		{
			name: "invalid log level falls back to default",
			env: map[string]string{
				"LOG_LEVEL": "trace",
			},
			want: Config{
				Address:           "0.0.0.0:8080",
				InfraAddress:      "127.0.0.1:9090",
				Environment:       "development",
				LogFormat:         "text",
				LogLevel:          slog.LevelInfo,
				ReadyzTimeout:     2 * time.Second,
				ReadHeaderTimeout: defaultReadHeaderTimeout,
				ReadTimeout:       defaultReadTimeout,
				WriteTimeout:      defaultWriteTimeout,
				IdleTimeout:       defaultIdleTimeout,
				CORS: CORSConfig{
					MaxAge: 5 * time.Minute,
				},
				CSRF: CSRFConfig{
					Enabled: true,
				},
			},
		},
		{
			name: "parses and normalizes CORS and CSRF configuration",
			env: map[string]string{
				"API_CORS_ALLOWED_ORIGINS":   " https://Example.com , * ,https://example.com/ ",
				"API_CORS_ALLOWED_METHODS":   " GET , POST ",
				"API_CORS_ALLOWED_HEADERS":   " Content-Type , Authorization ",
				"API_CORS_EXPOSED_HEADERS":   " X-Request-Id ",
				"API_CORS_ALLOW_CREDENTIALS": "true",
				"API_CORS_MAX_AGE":           "10m",
				"API_CSRF_TRUSTED_ORIGINS":   " https://Trusted.com , https://trusted.com/ ",
				"API_CSRF_ENABLED":           "false",
			},
			want: Config{
				Address:           "0.0.0.0:8080",
				InfraAddress:      "127.0.0.1:9090",
				Environment:       "development",
				LogFormat:         "text",
				LogLevel:          slog.LevelInfo,
				ReadyzTimeout:     2 * time.Second,
				ReadHeaderTimeout: defaultReadHeaderTimeout,
				ReadTimeout:       defaultReadTimeout,
				WriteTimeout:      defaultWriteTimeout,
				IdleTimeout:       defaultIdleTimeout,
				CORS: CORSConfig{
					AllowedOrigins:   []string{"https://example.com"},
					AllowedMethods:   []string{"GET", "POST"},
					AllowedHeaders:   []string{"Content-Type", "Authorization"},
					ExposedHeaders:   []string{"X-Request-Id"},
					AllowCredentials: true,
					MaxAge:           10 * time.Minute,
				},
				CSRF: CSRFConfig{
					TrustedOrigins: []string{"https://trusted.com"},
					Enabled:        false,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := loadConfig(getenvFromMap(tt.env))
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("loadConfig mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadConfigWithWarnings(t *testing.T) {
	tests := []struct {
		name         string
		env          map[string]string
		wantWarnings []string
	}{
		{
			name: "no warnings for valid timeout values",
			env: map[string]string{
				"API_READYZ_TIMEOUT":      "750ms",
				"API_READ_HEADER_TIMEOUT": "3s",
				"API_READ_TIMEOUT":        "11s",
				"API_WRITE_TIMEOUT":       "25s",
				"API_IDLE_TIMEOUT":        "45s",
			},
			wantWarnings: nil,
		},
		{
			name: "warnings include invalid timeout keys and values",
			env: map[string]string{
				"API_READYZ_TIMEOUT":      "invalid",
				"API_READ_HEADER_TIMEOUT": "0s",
				"API_READ_TIMEOUT":        "-1s",
				"API_WRITE_TIMEOUT":       "10",
				"API_IDLE_TIMEOUT":        "also-invalid",
			},
			wantWarnings: []string{
				"invalid duration for API_READYZ_TIMEOUT=\"invalid\", using default 2s",
				"non-positive duration for API_READ_HEADER_TIMEOUT=\"0s\", using default 5s",
				"non-positive duration for API_READ_TIMEOUT=\"-1s\", using default 15s",
				"invalid duration for API_WRITE_TIMEOUT=\"10\", using default 30s",
				"invalid duration for API_IDLE_TIMEOUT=\"also-invalid\", using default 1m0s",
			},
		},
		{
			name: "warnings for invalid log format and level",
			env: map[string]string{
				"LOG_FORMAT": "xml",
				"LOG_LEVEL":  "trace",
			},
			wantWarnings: []string{
				"invalid log format \"xml\", using default \"text\"",
				"invalid log level \"trace\", using default \"info\"",
			},
		},
		{
			name: "warnings for invalid CORS and CSRF values",
			env: map[string]string{
				"API_CORS_ALLOWED_ORIGINS":   "https://ok.example, not-an-origin",
				"API_CORS_ALLOW_CREDENTIALS": "maybe",
				"API_CORS_MAX_AGE":           "0s",
				"API_CSRF_TRUSTED_ORIGINS":   "https://trusted.example/path,https://trusted.example",
				"API_CSRF_ENABLED":           "invalid",
			},
			wantWarnings: []string{
				"invalid origin \"not-an-origin\" for API_CORS_ALLOWED_ORIGINS, ignoring",
				"invalid boolean for API_CORS_ALLOW_CREDENTIALS=\"maybe\", using default false",
				"non-positive duration for API_CORS_MAX_AGE=\"0s\", using default 5m0s",
				"invalid origin \"https://trusted.example/path\" for API_CSRF_TRUSTED_ORIGINS, ignoring",
				"invalid boolean for API_CSRF_ENABLED=\"invalid\", using default true",
			},
		},
		{
			name: "warning for wildcard origins with credentials",
			env: map[string]string{
				"API_CORS_ALLOWED_ORIGINS":   "*",
				"API_CORS_ALLOW_CREDENTIALS": "true",
			},
			wantWarnings: []string{
				"invalid CORS configuration: API_CORS_ALLOW_CREDENTIALS=true cannot be combined with wildcard API_CORS_ALLOWED_ORIGINS=\"*\"; wildcard origins are ignored",
			},
		},
		{
			name: "warning when CORS origins are configured without CSRF trusted origins",
			env: map[string]string{
				"API_CORS_ALLOWED_ORIGINS": "https://client.example",
			},
			wantWarnings: []string{
				"CSRF is enabled and CORS origins are configured, but API_CSRF_TRUSTED_ORIGINS is empty; unsafe cross-origin requests will be denied",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, gotWarnings := loadConfigWithWarnings(getenvFromMap(tt.env))
			if diff := cmp.Diff(tt.wantWarnings, gotWarnings); diff != "" {
				t.Fatalf("loadConfigWithWarnings mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func getenvFromMap(env map[string]string) func(string) string {
	return func(key string) string {
		return env[key]
	}
}

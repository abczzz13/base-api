package server

import (
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
				Address:           ":8080",
				InfraAddress:      "127.0.0.1:9090",
				Environment:       "development",
				ReadyzTimeout:     2 * time.Second,
				ReadHeaderTimeout: defaultReadHeaderTimeout,
				ReadTimeout:       defaultReadTimeout,
				WriteTimeout:      defaultWriteTimeout,
				IdleTimeout:       defaultIdleTimeout,
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
				ReadyzTimeout:     750 * time.Millisecond,
				ReadHeaderTimeout: 3 * time.Second,
				ReadTimeout:       11 * time.Second,
				WriteTimeout:      25 * time.Second,
				IdleTimeout:       45 * time.Second,
			},
		},
		{
			name: "falls back to app env",
			env: map[string]string{
				"APP_ENV": "qa",
			},
			want: Config{
				Address:           ":8080",
				InfraAddress:      "127.0.0.1:9090",
				Environment:       "qa",
				ReadyzTimeout:     2 * time.Second,
				ReadHeaderTimeout: defaultReadHeaderTimeout,
				ReadTimeout:       defaultReadTimeout,
				WriteTimeout:      defaultWriteTimeout,
				IdleTimeout:       defaultIdleTimeout,
			},
		},
		{
			name: "falls back to environment env",
			env: map[string]string{
				"ENVIRONMENT": "sandbox",
			},
			want: Config{
				Address:           ":8080",
				InfraAddress:      "127.0.0.1:9090",
				Environment:       "sandbox",
				ReadyzTimeout:     2 * time.Second,
				ReadHeaderTimeout: defaultReadHeaderTimeout,
				ReadTimeout:       defaultReadTimeout,
				WriteTimeout:      defaultWriteTimeout,
				IdleTimeout:       defaultIdleTimeout,
			},
		},
		{
			name: "invalid readyz timeout falls back to default",
			env: map[string]string{
				"API_READYZ_TIMEOUT": "not-a-duration",
			},
			want: Config{
				Address:           ":8080",
				InfraAddress:      "127.0.0.1:9090",
				Environment:       "development",
				ReadyzTimeout:     2 * time.Second,
				ReadHeaderTimeout: defaultReadHeaderTimeout,
				ReadTimeout:       defaultReadTimeout,
				WriteTimeout:      defaultWriteTimeout,
				IdleTimeout:       defaultIdleTimeout,
			},
		},
		{
			name: "non positive readyz timeout falls back to default",
			env: map[string]string{
				"API_READYZ_TIMEOUT": "-1s",
			},
			want: Config{
				Address:           ":8080",
				InfraAddress:      "127.0.0.1:9090",
				Environment:       "development",
				ReadyzTimeout:     2 * time.Second,
				ReadHeaderTimeout: defaultReadHeaderTimeout,
				ReadTimeout:       defaultReadTimeout,
				WriteTimeout:      defaultWriteTimeout,
				IdleTimeout:       defaultIdleTimeout,
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
				Address:           ":8080",
				InfraAddress:      "127.0.0.1:9090",
				Environment:       "development",
				ReadyzTimeout:     2 * time.Second,
				ReadHeaderTimeout: defaultReadHeaderTimeout,
				ReadTimeout:       defaultReadTimeout,
				WriteTimeout:      defaultWriteTimeout,
				IdleTimeout:       defaultIdleTimeout,
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

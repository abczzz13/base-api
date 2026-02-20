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
				Address:       ":8080",
				InfraAddress:  "127.0.0.1:9090",
				Environment:   "development",
				ReadyzTimeout: 2 * time.Second,
			},
		},
		{
			name: "uses explicit api values",
			env: map[string]string{
				"API_ADDR":           "0.0.0.0:8081",
				"API_INFRA_ADDR":     "127.0.0.1:9191",
				"API_ENVIRONMENT":    "production",
				"APP_ENV":            "staging",
				"ENVIRONMENT":        "ignored",
				"API_READYZ_TIMEOUT": "750ms",
			},
			want: Config{
				Address:       "0.0.0.0:8081",
				InfraAddress:  "127.0.0.1:9191",
				Environment:   "production",
				ReadyzTimeout: 750 * time.Millisecond,
			},
		},
		{
			name: "falls back to app env",
			env: map[string]string{
				"APP_ENV": "qa",
			},
			want: Config{
				Address:       ":8080",
				InfraAddress:  "127.0.0.1:9090",
				Environment:   "qa",
				ReadyzTimeout: 2 * time.Second,
			},
		},
		{
			name: "falls back to environment env",
			env: map[string]string{
				"ENVIRONMENT": "sandbox",
			},
			want: Config{
				Address:       ":8080",
				InfraAddress:  "127.0.0.1:9090",
				Environment:   "sandbox",
				ReadyzTimeout: 2 * time.Second,
			},
		},
		{
			name: "invalid readyz timeout falls back to default",
			env: map[string]string{
				"API_READYZ_TIMEOUT": "not-a-duration",
			},
			want: Config{
				Address:       ":8080",
				InfraAddress:  "127.0.0.1:9090",
				Environment:   "development",
				ReadyzTimeout: 2 * time.Second,
			},
		},
		{
			name: "non positive readyz timeout falls back to default",
			env: map[string]string{
				"API_READYZ_TIMEOUT": "-1s",
			},
			want: Config{
				Address:       ":8080",
				InfraAddress:  "127.0.0.1:9090",
				Environment:   "development",
				ReadyzTimeout: 2 * time.Second,
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

func getenvFromMap(env map[string]string) func(string) string {
	return func(key string) string {
		return env[key]
	}
}

package server

import (
	"fmt"
	"time"
)

type Config struct {
	Address           string
	InfraAddress      string
	Environment       string
	ReadyzTimeout     time.Duration
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

func loadConfig(getenv func(string) string) Config {
	cfg, _ := loadConfigWithWarnings(getenv)
	return cfg
}

func loadConfigWithWarnings(getenv func(string) string) (Config, []string) {
	cfg := Config{
		Address:           getenv("API_ADDR"),
		InfraAddress:      getenv("API_INFRA_ADDR"),
		Environment:       getenv("API_ENVIRONMENT"),
		ReadyzTimeout:     2 * time.Second,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		ReadTimeout:       defaultReadTimeout,
		WriteTimeout:      defaultWriteTimeout,
		IdleTimeout:       defaultIdleTimeout,
	}

	if cfg.Address == "" {
		cfg.Address = ":8080"
	}

	if cfg.InfraAddress == "" {
		cfg.InfraAddress = "127.0.0.1:9090"
	}

	if cfg.Environment == "" {
		cfg.Environment = getenv("APP_ENV")
	}

	if cfg.Environment == "" {
		cfg.Environment = getenv("ENVIRONMENT")
	}

	if cfg.Environment == "" {
		cfg.Environment = "development"
	}

	var warnings []string
	var warning string

	cfg.ReadyzTimeout, warning = loadPositiveDurationEnv(getenv, "API_READYZ_TIMEOUT", cfg.ReadyzTimeout)
	warnings = appendWarning(warnings, warning)

	cfg.ReadHeaderTimeout, warning = loadPositiveDurationEnv(getenv, "API_READ_HEADER_TIMEOUT", cfg.ReadHeaderTimeout)
	warnings = appendWarning(warnings, warning)

	cfg.ReadTimeout, warning = loadPositiveDurationEnv(getenv, "API_READ_TIMEOUT", cfg.ReadTimeout)
	warnings = appendWarning(warnings, warning)

	cfg.WriteTimeout, warning = loadPositiveDurationEnv(getenv, "API_WRITE_TIMEOUT", cfg.WriteTimeout)
	warnings = appendWarning(warnings, warning)

	cfg.IdleTimeout, warning = loadPositiveDurationEnv(getenv, "API_IDLE_TIMEOUT", cfg.IdleTimeout)
	warnings = appendWarning(warnings, warning)

	return cfg, warnings
}

func appendWarning(warnings []string, warning string) []string {
	if warning == "" {
		return warnings
	}

	return append(warnings, warning)
}

func loadPositiveDurationEnv(getenv func(string) string, key string, fallback time.Duration) (time.Duration, string) {
	value := getenv(key)
	if value == "" {
		return fallback, ""
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback, fmt.Sprintf("invalid duration for %s=%q, using default %s", key, value, fallback)
	}
	if parsed <= 0 {
		return fallback, fmt.Sprintf("non-positive duration for %s=%q, using default %s", key, value, fallback)
	}

	return parsed, ""
}

package server

import (
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"
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

type Config struct {
	Address           string
	InfraAddress      string
	Environment       string
	LogFormat         string
	LogLevel          slog.Level
	ReadyzTimeout     time.Duration
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	CORS              CORSConfig
	CSRF              CSRFConfig
}

func loadConfig(getenv func(string) string) Config {
	cfg, _ := loadConfigWithWarnings(getenv)
	return cfg
}

func loadConfigWithWarnings(getenv func(string) string) (Config, []string) {
	logLevelStr := strings.ToLower(strings.TrimSpace(getenv("LOG_LEVEL")))
	logLevel, _ := parseLogLevel(logLevelStr)

	cfg := Config{
		Address:           getenv("API_ADDR"),
		InfraAddress:      getenv("API_INFRA_ADDR"),
		Environment:       getenv("API_ENVIRONMENT"),
		LogFormat:         strings.ToLower(strings.TrimSpace(getenv("LOG_FORMAT"))),
		LogLevel:          logLevel,
		ReadyzTimeout:     2 * time.Second,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		ReadTimeout:       defaultReadTimeout,
		WriteTimeout:      defaultWriteTimeout,
		IdleTimeout:       defaultIdleTimeout,
	}

	if cfg.Address == "" {
		cfg.Address = "0.0.0.0:8080"
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

	if cfg.LogFormat == "" {
		cfg.LogFormat = "text"
	}

	if logLevelStr == "" {
		cfg.LogLevel = slog.LevelInfo
	}

	var warnings []string
	var warning string

	if cfg.LogFormat != "text" && cfg.LogFormat != "json" {
		warnings = append(warnings, fmt.Sprintf("invalid log format %q, using default \"text\"", cfg.LogFormat))
		cfg.LogFormat = "text"
	}

	if _, valid := parseLogLevel(logLevelStr); logLevelStr != "" && !valid {
		warnings = append(warnings, fmt.Sprintf("invalid log level %q, using default \"info\"", logLevelStr))
		cfg.LogLevel = slog.LevelInfo
	}

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

	var corsWarnings []string
	cfg.CORS, corsWarnings = loadCORSConfig(getenv)
	warnings = append(warnings, corsWarnings...)

	var csrfWarnings []string
	cfg.CSRF, csrfWarnings = loadCSRFConfig(getenv)
	warnings = append(warnings, csrfWarnings...)

	if cfg.CSRF.Enabled && len(cfg.CORS.AllowedOrigins) > 0 && len(cfg.CSRF.TrustedOrigins) == 0 {
		warnings = append(warnings, "CSRF is enabled and CORS origins are configured, but API_CSRF_TRUSTED_ORIGINS is empty; unsafe cross-origin requests will be denied")
	}

	return cfg, warnings
}

func appendWarning(warnings []string, warning string) []string {
	if warning == "" {
		return warnings
	}

	return append(warnings, warning)
}

func parseLogLevel(levelStr string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(levelStr)) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return slog.LevelInfo, false
	}
}

func loadPositiveDurationEnv(getenv func(string) string, key string, fallback time.Duration) (time.Duration, string) {
	value := strings.TrimSpace(getenv(key))
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

func loadCORSConfig(getenv func(string) string) (CORSConfig, []string) {
	cfg := CORSConfig{
		MaxAge: 5 * time.Minute,
	}
	var warnings []string

	if v := getenv("API_CORS_ALLOWED_ORIGINS"); v != "" {
		origins, invalid := normalizeOrigins(splitAndTrimCSV(v), true)
		cfg.AllowedOrigins = origins
		for _, origin := range invalid {
			warnings = append(warnings, fmt.Sprintf("invalid origin %q for API_CORS_ALLOWED_ORIGINS, ignoring", origin))
		}
	}

	if v := getenv("API_CORS_ALLOWED_METHODS"); v != "" {
		cfg.AllowedMethods = splitAndTrimCSV(v)
	}

	if v := getenv("API_CORS_ALLOWED_HEADERS"); v != "" {
		cfg.AllowedHeaders = splitAndTrimCSV(v)
	}

	if v := getenv("API_CORS_EXPOSED_HEADERS"); v != "" {
		cfg.ExposedHeaders = splitAndTrimCSV(v)
	}

	allowCredentials, warning := loadBoolEnv(getenv, "API_CORS_ALLOW_CREDENTIALS", false)
	warnings = appendWarning(warnings, warning)
	cfg.AllowCredentials = allowCredentials

	cfg.MaxAge, warning = loadPositiveDurationEnv(getenv, "API_CORS_MAX_AGE", cfg.MaxAge)
	warnings = appendWarning(warnings, warning)

	if cfg.AllowCredentials {
		filteredOrigins := make([]string, 0, len(cfg.AllowedOrigins))
		ignoredWildcard := false
		for _, origin := range cfg.AllowedOrigins {
			if origin == "*" {
				ignoredWildcard = true
				continue
			}
			filteredOrigins = append(filteredOrigins, origin)
		}

		if ignoredWildcard {
			warnings = append(warnings, "invalid CORS configuration: API_CORS_ALLOW_CREDENTIALS=true cannot be combined with wildcard API_CORS_ALLOWED_ORIGINS=\"*\"; wildcard origins are ignored")
			cfg.AllowedOrigins = filteredOrigins
		}
	}

	return cfg, warnings
}

func loadCSRFConfig(getenv func(string) string) (CSRFConfig, []string) {
	cfg := CSRFConfig{
		Enabled: true,
	}
	var warnings []string

	if v := getenv("API_CSRF_TRUSTED_ORIGINS"); v != "" {
		origins, invalid := normalizeOrigins(splitAndTrimCSV(v), false)
		cfg.TrustedOrigins = origins
		for _, origin := range invalid {
			warnings = append(warnings, fmt.Sprintf("invalid origin %q for API_CSRF_TRUSTED_ORIGINS, ignoring", origin))
		}
	}

	enabled, warning := loadBoolEnv(getenv, "API_CSRF_ENABLED", true)
	warnings = appendWarning(warnings, warning)
	cfg.Enabled = enabled

	return cfg, warnings
}

func loadBoolEnv(getenv func(string) string, key string, fallback bool) (bool, string) {
	value := strings.TrimSpace(getenv(key))
	if value == "" {
		return fallback, ""
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback, fmt.Sprintf("invalid boolean for %s=%q, using default %t", key, value, fallback)
	}

	return parsed, ""
}

func splitAndTrimCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}

	return result
}

func normalizeOrigins(origins []string, allowWildcard bool) ([]string, []string) {
	result := make([]string, 0, len(origins))
	invalid := make([]string, 0)
	seen := make(map[string]struct{}, len(origins))

	for _, origin := range origins {
		if allowWildcard && origin == "*" {
			if _, exists := seen[origin]; !exists {
				result = append(result, origin)
				seen[origin] = struct{}{}
			}
			continue
		}

		normalized, ok := normalizeOrigin(origin)
		if !ok {
			invalid = append(invalid, origin)
			continue
		}

		if _, exists := seen[normalized]; exists {
			continue
		}

		result = append(result, normalized)
		seen[normalized] = struct{}{}
	}

	return result, invalid
}

func normalizeOrigin(origin string) (string, bool) {
	parsed, err := url.Parse(origin)
	if err != nil {
		return "", false
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}

	if parsed.Host == "" {
		return "", false
	}

	if parsed.User != nil {
		return "", false
	}

	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}

	if parsed.Path != "" && parsed.Path != "/" {
		return "", false
	}

	return fmt.Sprintf("%s://%s", scheme, strings.ToLower(parsed.Host)), true
}

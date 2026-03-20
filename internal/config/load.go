package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/abczzz13/base-api/internal/logger"
	"github.com/abczzz13/base-api/internal/ratelimit"
	"github.com/abczzz13/base-api/internal/telemetry"
	"github.com/abczzz13/base-api/internal/valkey"
)

type LookupEnvFunc func(string) (string, bool)

type readFileFunc func(string) ([]byte, error)

type loader struct {
	lookupEnv LookupEnvFunc
	readFile  readFileFunc
}

func Load(lookupEnv LookupEnvFunc) (Config, error) {
	return loadWithReader(lookupEnv, os.ReadFile)
}

func loadWithReader(lookupEnv LookupEnvFunc, readFile readFileFunc) (Config, error) {
	return loader{lookupEnv: lookupEnv, readFile: readFile}.load()
}

func (l loader) load() (Config, error) {
	if l.lookupEnv == nil {
		return Config{}, errors.New("lookup env function is required")
	}
	if l.readFile == nil {
		return Config{}, errors.New("read file function is required")
	}

	cfg := defaultConfig()
	errList := make([]error, 0)

	l.loadCore(&cfg, &errList)
	l.loadTimeouts(&cfg, &errList)
	l.loadCORS(&cfg, &errList)
	l.loadCSRF(&cfg, &errList)
	l.loadClientIP(&cfg, &errList)
	l.loadRequestAudit(&cfg, &errList)
	l.loadRequestLogger(&cfg, &errList)
	l.loadRateLimit(&cfg, &errList)
	l.loadValkey(&cfg, &errList)
	l.loadOTEL(&cfg, &errList)
	l.loadWeather(&cfg, &errList)
	l.loadDB(&cfg, &errList)
	validateConfig(cfg, &errList)

	if len(errList) > 0 {
		return Config{}, errors.Join(errList...)
	}

	return cfg, nil
}

func (l loader) loadCore(cfg *Config, errList *[]error) {
	if value, ok, err := l.resolveString(keyAPIAddr); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.Address = value
	}

	if value, ok, err := l.resolveString(keyAPIInfraAddr); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.InfraAddress = value
	}

	if value, ok, err := l.resolveString(keyAPIEnvironment); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.Environment = value
	}

	if value, ok, err := l.resolveString(keyLogFormat); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		if parsedLogFormat, valid := logger.ParseFormat(value); valid {
			cfg.LogFormat = parsedLogFormat
		} else {
			appendLoadError(errList, fmt.Errorf("invalid log format %q for %s", value, keyLogFormat))
		}
	}

	if value, ok, err := l.resolveString(keyLogLevel); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		if parsedLogLevel, valid := parseLogLevel(value); valid {
			cfg.LogLevel = parsedLogLevel
		} else {
			appendLoadError(errList, fmt.Errorf("invalid log level %q for %s", value, keyLogLevel))
		}
	}
}

func (l loader) loadTimeouts(cfg *Config, errList *[]error) {
	if value, ok, err := l.resolvePositiveDuration(keyAPIReadyzTimeout); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.ReadyzTimeout = value
	}

	if value, ok, err := l.resolvePositiveDuration(keyAPIReadHeaderTimeout); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.ReadHeaderTimeout = value
	}

	if value, ok, err := l.resolvePositiveDuration(keyAPIReadTimeout); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.ReadTimeout = value
	}

	if value, ok, err := l.resolvePositiveDuration(keyAPIWriteTimeout); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.WriteTimeout = value
	}

	if value, ok, err := l.resolvePositiveDuration(keyAPIIdleTimeout); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.IdleTimeout = value
	}
}

func (l loader) loadCORS(cfg *Config, errList *[]error) {
	if value, ok, err := l.resolveString(keyAPICORSAllowedOrigins); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		origins, invalid := normalizeOrigins(splitAndTrimCSV(value), true)
		cfg.CORS.AllowedOrigins = origins
		for _, origin := range invalid {
			appendLoadError(errList, fmt.Errorf("invalid origin %q for %s", origin, keyAPICORSAllowedOrigins))
		}
	}

	if value, ok, err := l.resolveString(keyAPICORSAllowedMethods); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.CORS.AllowedMethods = splitAndTrimCSV(value)
	}

	if value, ok, err := l.resolveString(keyAPICORSAllowedHeaders); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.CORS.AllowedHeaders = splitAndTrimCSV(value)
	}

	if value, ok, err := l.resolveString(keyAPICORSExposedHeaders); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.CORS.ExposedHeaders = splitAndTrimCSV(value)
	}

	if value, ok, err := l.resolveBool(keyAPICORSAllowCredentials); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.CORS.AllowCredentials = value
	}

	if value, ok, err := l.resolvePositiveDuration(keyAPICORSMaxAge); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.CORS.MaxAge = value
	}
}

func (l loader) loadCSRF(cfg *Config, errList *[]error) {
	if value, ok, err := l.resolveString(keyAPICSRFTrustedOrigins); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		origins, invalid := normalizeOrigins(splitAndTrimCSV(value), false)
		cfg.CSRF.TrustedOrigins = origins
		for _, origin := range invalid {
			appendLoadError(errList, fmt.Errorf("invalid origin %q for %s", origin, keyAPICSRFTrustedOrigins))
		}
	}

	if value, ok, err := l.resolveBool(keyAPICSRFEnabled); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.CSRF.Enabled = value
	}
}

func (l loader) loadRequestAudit(cfg *Config, errList *[]error) {
	if value, ok, err := l.resolveBool(keyAPIRequestAuditEnabled); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.RequestAudit.Enabled = &value
	}
}

func (l loader) loadClientIP(cfg *Config, errList *[]error) {
	if value, ok, err := l.resolveString(keyAPITrustedProxyCIDRs); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		trustedProxyCIDRs, parseErrs := parseTrustedProxyCIDRs(value, keyAPITrustedProxyCIDRs)
		for _, parseErr := range parseErrs {
			appendLoadError(errList, parseErr)
		}
		cfg.ClientIP.TrustedProxyCIDRs = trustedProxyCIDRs
	}
}

func (l loader) loadRequestLogger(cfg *Config, errList *[]error) {
	if value, ok, err := l.resolveBool(keyAPIRequestLoggerEnabled); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.RequestLogger.Enabled = &value
	}
}

func (l loader) loadRateLimit(cfg *Config, errList *[]error) {
	if value, ok, err := l.resolveBool(keyAPIRateLimitEnabled); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.RateLimit.Enabled = value
	}

	if value, ok, err := l.resolveBool(keyAPIRateLimitFailOpen); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.RateLimit.FailOpen = value
	}

	if value, ok, err := l.resolvePositiveDuration(keyAPIRateLimitTimeout); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.RateLimit.Timeout = value
	}

	if value, ok, err := l.resolvePositiveFloat(keyAPIRateLimitDefaultRPS); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.RateLimit.DefaultPolicy.RequestsPerSecond = value
	}

	if value, ok, err := l.resolvePositiveInt(keyAPIRateLimitDefaultBurst); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.RateLimit.DefaultPolicy.Burst = value
	}

	if value, ok, err := l.resolveString(keyAPIRateLimitKeyPrefix); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.RateLimit.KeyPrefix = value
	}

	if value, ok, err := l.resolveString(keyAPIRateLimitRouteOverridesJSON); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		routeOverrides, parseErr := parseRateLimitRouteOverrides(value)
		if parseErr != nil {
			appendLoadError(errList, fmt.Errorf("invalid %s: %w", keyAPIRateLimitRouteOverridesJSON, parseErr))
		} else {
			for route, override := range routeOverrides {
				cfg.RateLimit.RouteOverrides[route] = override
			}
		}
	}
}

func (l loader) loadValkey(cfg *Config, errList *[]error) {
	if value, ok, err := l.resolveString(keyAPIValkeyMode); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.Valkey.Mode = valkey.Mode(value)
	}

	if value, ok, err := l.resolveString(keyAPIValkeyAddrs); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.Valkey.Addrs = splitAndTrimCSV(value)
	}
}

func (l loader) loadOTEL(cfg *Config, errList *[]error) {
	if value, ok, err := l.resolveString(keyOTELServiceName); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.OTEL.ServiceName = value
	}

	sdkDisabled := false
	if value, ok, err := l.resolveBool(keyOTELSDKDisabled); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		sdkDisabled = value
	}

	otlpEndpointConfigured, err := l.isConfigured(keyOTELExporterOTLPEndpoint)
	if err != nil {
		appendLoadError(errList, err)
	}

	otlpTracesEndpointConfigured, err := l.isConfigured(keyOTELExporterOTLPTracesEndpoint)
	if err != nil {
		appendLoadError(errList, err)
	}

	cfg.OTEL.TracingEnabled = (otlpEndpointConfigured || otlpTracesEndpointConfigured) && !sdkDisabled
	if !cfg.OTEL.TracingEnabled {
		return
	}

	if value, ok, err := l.resolveString(keyOTELTracesSampler); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		if parsedSampler, valid := telemetry.ParseTraceSampler(value); valid {
			cfg.OTEL.TracesSampler = parsedSampler
		} else {
			appendLoadError(errList, fmt.Errorf("invalid value %q for %s", value, keyOTELTracesSampler))
		}
	}

	samplerArgSet := false
	if value, ok, err := l.resolveFloat(keyOTELTracesSamplerArg); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		samplerArgSet = true
		cfg.OTEL.TracesSamplerArg = &value
	}

	if samplerArgSet && !cfg.OTEL.TracesSampler.UsesArgument() {
		appendLoadError(errList, fmt.Errorf(
			"%s is set but %s=%q does not use a sampler argument",
			keyOTELTracesSamplerArg,
			keyOTELTracesSampler,
			cfg.OTEL.TracesSampler,
		))
	}
}

func (l loader) loadWeather(cfg *Config, errList *[]error) {
	if value, ok, err := l.resolveString(keyWeatherGeocodingBaseURL); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.Weather.GeocodingBaseURL = value
	}

	if value, ok, err := l.resolveString(keyWeatherForecastBaseURL); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.Weather.ForecastBaseURL = value
	}

	if value, ok, err := l.resolveString(keyWeatherAPIKey); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.Weather.APIKey = value
	}

	if value, ok, err := l.resolvePositiveDuration(keyWeatherTimeout); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.Weather.Timeout = value
	}
}

func (l loader) loadDB(cfg *Config, errList *[]error) {
	if value, ok, err := l.resolveString(keyDBURL); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.DB.URL = value
	}

	if value, ok, err := l.resolveNonNegativeInt32(keyDBMinConns); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.DB.MinConns = value
	}

	if value, ok, err := l.resolvePositiveInt32(keyDBMaxConns); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.DB.MaxConns = value
	}

	if value, ok, err := l.resolvePositiveDuration(keyDBMaxConnLifetime); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.DB.MaxConnLifetime = value
	}

	if value, ok, err := l.resolvePositiveDuration(keyDBMaxConnIdleTime); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.DB.MaxConnIdleTime = value
	}

	if value, ok, err := l.resolvePositiveDuration(keyDBHealthCheckPeriod); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.DB.HealthCheckPeriod = value
	}

	if value, ok, err := l.resolveNonNegativeDuration(keyDBConnectTimeout); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.DB.ConnectTimeout = value
	}

	if value, ok, err := l.resolveBool(keyDBMigrateOnStartup); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.DB.MigrateOnStartup = value
	}

	if value, ok, err := l.resolvePositiveDuration(keyDBMigrateTimeout); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.DB.MigrateTimeout = value
	}

	if value, ok, err := l.resolvePositiveInt32(keyDBStartupMaxAttempts); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.DB.StartupMaxAttempts = value
	}

	if value, ok, err := l.resolvePositiveDuration(keyDBStartupBackoffInitial); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.DB.StartupBackoffInitial = value
	}

	if value, ok, err := l.resolvePositiveDuration(keyDBStartupBackoffMax); err != nil {
		appendLoadError(errList, err)
	} else if ok {
		cfg.DB.StartupBackoffMax = value
	}
}

func validateConfig(cfg Config, errList *[]error) {
	if cfg.CORS.AllowCredentials {
		for _, origin := range cfg.CORS.AllowedOrigins {
			if origin == "*" {
				appendLoadError(errList, fmt.Errorf(
					"invalid CORS configuration: %s=true cannot be combined with wildcard %s=\"*\"",
					keyAPICORSAllowCredentials,
					keyAPICORSAllowedOrigins,
				))
				break
			}
		}
	}

	if cfg.CSRF.Enabled && len(cfg.CORS.AllowedOrigins) > 0 && len(cfg.CSRF.TrustedOrigins) == 0 {
		appendLoadError(errList, fmt.Errorf(
			"%s is enabled and %s is configured, but %s is empty",
			keyAPICSRFEnabled,
			keyAPICORSAllowedOrigins,
			keyAPICSRFTrustedOrigins,
		))
	}

	if cfg.RateLimit.IsEnabled() {
		appendLoadError(errList, validateRateLimitConfig(cfg.RateLimit, cfg.Valkey))
	}

	appendLoadError(errList, validateAddress(keyAPIAddr, cfg.Address))
	appendLoadError(errList, validateAddress(keyAPIInfraAddr, cfg.InfraAddress))
	appendLoadError(errList, validateOptionalBaseURL(keyWeatherGeocodingBaseURL, cfg.Weather.GeocodingBaseURL))
	appendLoadError(errList, validateOptionalBaseURL(keyWeatherForecastBaseURL, cfg.Weather.ForecastBaseURL))

	weatherGeocodingConfigured := strings.TrimSpace(cfg.Weather.GeocodingBaseURL) != ""
	weatherForecastConfigured := strings.TrimSpace(cfg.Weather.ForecastBaseURL) != ""

	if weatherGeocodingConfigured != weatherForecastConfigured {
		appendLoadError(errList, fmt.Errorf(
			"invalid weather integration configuration: %s and %s must be configured together",
			keyWeatherGeocodingBaseURL,
			keyWeatherForecastBaseURL,
		))
	}

	if cfg.DB.MinConns > cfg.DB.MaxConns {
		appendLoadError(errList, fmt.Errorf(
			"invalid database pool configuration: %s=%d cannot exceed %s=%d",
			keyDBMinConns,
			cfg.DB.MinConns,
			keyDBMaxConns,
			cfg.DB.MaxConns,
		))
	}

	if cfg.DB.StartupBackoffInitial > cfg.DB.StartupBackoffMax {
		appendLoadError(errList, fmt.Errorf(
			"invalid database startup retry configuration: %s=%s cannot exceed %s=%s",
			keyDBStartupBackoffInitial,
			cfg.DB.StartupBackoffInitial,
			keyDBStartupBackoffMax,
			cfg.DB.StartupBackoffMax,
		))
	}
}

func validateRateLimitConfig(cfg RateLimitConfig, vcfg valkey.Config) error {
	if err := cfg.DefaultPolicy.Validate(); err != nil {
		return fmt.Errorf("invalid rate limit default policy: %w", err)
	}
	if err := vcfg.ValidateMode(); err != nil {
		return fmt.Errorf("invalid %s=%q: %w", keyAPIValkeyMode, vcfg.Mode, err)
	}
	if len(vcfg.Addrs) == 0 {
		return fmt.Errorf("%s is enabled but %s is empty", keyAPIRateLimitEnabled, keyAPIValkeyAddrs)
	}
	for _, addr := range vcfg.Addrs {
		if err := validateAddress(keyAPIValkeyAddrs, addr); err != nil {
			return err
		}
	}

	return nil
}

func parseRateLimitRouteOverrides(raw string) (map[string]ratelimit.RouteOverride, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()

	routeOverrides := make(map[string]ratelimit.RouteOverride)
	if err := decoder.Decode(&routeOverrides); err != nil {
		return nil, err
	}

	for route, override := range routeOverrides {
		if !isKnownPublicOperationName(route) {
			return nil, fmt.Errorf("unknown public operation name %q (known: %s)", route, strings.Join(knownPublicOperationNamesList(), ", "))
		}
		if err := override.Validate(); err != nil {
			return nil, fmt.Errorf("route %q: %w", route, err)
		}
	}

	return routeOverrides, nil
}

func parseTrustedProxyCIDRs(raw, key string) ([]netip.Prefix, []error) {
	cidrs := splitAndTrimCSV(raw)
	trustedProxyCIDRs := make([]netip.Prefix, 0, len(cidrs))
	seen := make(map[netip.Prefix]struct{}, len(cidrs))
	errList := make([]error, 0)

	for _, cidr := range cidrs {
		parsed, err := netip.ParsePrefix(cidr)
		if err != nil {
			errList = append(errList, fmt.Errorf("invalid CIDR %q for %s", cidr, key))
			continue
		}

		normalized := parsed.Masked()
		if _, alreadySeen := seen[normalized]; alreadySeen {
			continue
		}

		seen[normalized] = struct{}{}
		trustedProxyCIDRs = append(trustedProxyCIDRs, normalized)
	}

	return trustedProxyCIDRs, errList
}

func appendLoadError(errList *[]error, err error) {
	if err == nil {
		return
	}

	*errList = append(*errList, err)
}

func (l loader) resolveRaw(key string) (string, bool, error) {
	value, valueSet := l.lookupEnv(key)
	fileKey := fileEnvKey(key)
	filePath, fileSet := l.lookupEnv(fileKey)
	valueConfigured := valueSet && strings.TrimSpace(value) != ""
	trimmedFilePath := strings.TrimSpace(filePath)
	fileConfigured := fileSet && trimmedFilePath != ""

	if valueConfigured && fileConfigured {
		return "", false, fmt.Errorf("configuration conflict: both %s and %s are set", key, fileKey)
	}

	if fileConfigured {
		content, err := l.readFile(trimmedFilePath)
		if err != nil {
			return "", false, fmt.Errorf("read %s from %s=%q: %w", key, fileKey, trimmedFilePath, err)
		}

		return strings.TrimRight(string(content), "\r\n"), true, nil
	}

	if fileSet && !valueConfigured {
		return "", false, fmt.Errorf("invalid %s: file path is empty", fileKey)
	}

	if !valueSet {
		return "", false, nil
	}

	return value, true, nil
}

func (l loader) resolveString(key string) (string, bool, error) {
	value, set, err := l.resolveRaw(key)
	if err != nil || !set {
		return "", false, err
	}

	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", false, nil
	}

	return trimmed, true, nil
}

func (l loader) resolveBool(key string) (bool, bool, error) {
	value, set, err := l.resolveString(key)
	if err != nil || !set {
		return false, false, err
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, false, fmt.Errorf("invalid boolean for %s=%q", key, value)
	}

	return parsed, true, nil
}

func (l loader) resolvePositiveDuration(key string) (time.Duration, bool, error) {
	value, set, err := l.resolveString(key)
	if err != nil || !set {
		return 0, false, err
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, false, fmt.Errorf("invalid duration for %s=%q", key, value)
	}
	if parsed <= 0 {
		return 0, false, fmt.Errorf("non-positive duration for %s=%q", key, value)
	}

	return parsed, true, nil
}

func (l loader) resolveNonNegativeDuration(key string) (time.Duration, bool, error) {
	value, set, err := l.resolveString(key)
	if err != nil || !set {
		return 0, false, err
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, false, fmt.Errorf("invalid duration for %s=%q", key, value)
	}
	if parsed < 0 {
		return 0, false, fmt.Errorf("negative duration for %s=%q", key, value)
	}

	return parsed, true, nil
}

func (l loader) resolvePositiveFloat(key string) (float64, bool, error) {
	value, set, err := l.resolveString(key)
	if err != nil || !set {
		return 0, false, err
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, false, fmt.Errorf("invalid float for %s=%q", key, value)
	}
	if parsed <= 0 {
		return 0, false, fmt.Errorf("non-positive float for %s=%q", key, value)
	}

	return parsed, true, nil
}

func (l loader) resolveFloat(key string) (float64, bool, error) {
	value, set, err := l.resolveString(key)
	if err != nil || !set {
		return 0, false, err
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, false, fmt.Errorf("invalid float for %s=%q", key, value)
	}

	if parsed < 0 || parsed > 1 {
		return 0, false, fmt.Errorf("out-of-range float for %s=%q, expected value between 0 and 1 inclusive", key, value)
	}

	return parsed, true, nil
}

func (l loader) resolvePositiveInt(key string) (int, bool, error) {
	value, set, err := l.resolveString(key)
	if err != nil || !set {
		return 0, false, err
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, false, fmt.Errorf("invalid integer for %s=%q", key, value)
	}
	if parsed <= 0 {
		return 0, false, fmt.Errorf("non-positive integer for %s=%d", key, parsed)
	}

	return parsed, true, nil
}

func (l loader) resolvePositiveInt32(key string) (int32, bool, error) {
	parsed, set, err := l.resolveInt32(key)
	if err != nil || !set {
		return 0, set, err
	}

	if parsed <= 0 {
		return 0, false, fmt.Errorf("non-positive integer for %s=%d", key, parsed)
	}

	return parsed, true, nil
}

func (l loader) resolveNonNegativeInt32(key string) (int32, bool, error) {
	parsed, set, err := l.resolveInt32(key)
	if err != nil || !set {
		return 0, set, err
	}

	if parsed < 0 {
		return 0, false, fmt.Errorf("negative integer for %s=%d", key, parsed)
	}

	return parsed, true, nil
}

func (l loader) resolveInt32(key string) (int32, bool, error) {
	value, set, err := l.resolveString(key)
	if err != nil || !set {
		return 0, false, err
	}

	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, false, fmt.Errorf("invalid integer for %s=%q", key, value)
	}

	return int32(parsed), true, nil
}

func (l loader) isConfigured(key string) (bool, error) {
	_, set, err := l.resolveString(key)
	if err != nil {
		return false, err
	}

	return set, nil
}

func parseLogLevel(level string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(level)) {
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

func validateAddress(key, value string) error {
	if _, _, err := net.SplitHostPort(value); err != nil {
		return fmt.Errorf("invalid %s=%q: %w", key, value, err)
	}

	return nil
}

func validateOptionalBaseURL(key, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("invalid %s=%q: parse URL: %w", key, value, err)
	}
	if !parsed.IsAbs() {
		return fmt.Errorf("invalid %s=%q: URL must be absolute", key, value)
	}
	if parsed.User != nil {
		return fmt.Errorf("invalid %s=%q: URL must not include user info", key, value)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("invalid %s=%q: URL must not include query or fragment", key, value)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return fmt.Errorf("invalid %s=%q: URL must not include a path", key, value)
	}

	return nil
}

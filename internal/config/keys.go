package config

const envFileSuffix = "_FILE"

const (
	keyAPIAddr      = "API_ADDR"
	keyAPIInfraAddr = "API_INFRA_ADDR"

	keyAPIEnvironment = "API_ENVIRONMENT"

	keyLogFormat = "LOG_FORMAT"
	keyLogLevel  = "LOG_LEVEL"

	keyAPIReadyzTimeout     = "API_READYZ_TIMEOUT"
	keyAPIReadHeaderTimeout = "API_READ_HEADER_TIMEOUT"
	keyAPIReadTimeout       = "API_READ_TIMEOUT"
	keyAPIWriteTimeout      = "API_WRITE_TIMEOUT"
	keyAPIIdleTimeout       = "API_IDLE_TIMEOUT"

	keyAPICORSAllowedOrigins   = "API_CORS_ALLOWED_ORIGINS"
	keyAPICORSAllowedMethods   = "API_CORS_ALLOWED_METHODS"
	keyAPICORSAllowedHeaders   = "API_CORS_ALLOWED_HEADERS"
	keyAPICORSExposedHeaders   = "API_CORS_EXPOSED_HEADERS"
	keyAPICORSAllowCredentials = "API_CORS_ALLOW_CREDENTIALS" //nolint:gosec // Env var name, not a credential value.
	keyAPICORSMaxAge           = "API_CORS_MAX_AGE"

	keyAPICSRFTrustedOrigins = "API_CSRF_TRUSTED_ORIGINS"
	keyAPICSRFEnabled        = "API_CSRF_ENABLED"

	keyAPITrustedProxyCIDRs = "API_TRUSTED_PROXY_CIDRS"

	keyAPIRequestAuditEnabled         = "API_REQUEST_AUDIT_ENABLED"
	keyAPIRequestLoggerEnabled        = "API_REQUEST_LOGGER_ENABLED"
	keyAPIRateLimitEnabled            = "API_RATE_LIMIT_ENABLED"
	keyAPIRateLimitFailOpen           = "API_RATE_LIMIT_FAIL_OPEN"
	keyAPIRateLimitTimeout            = "API_RATE_LIMIT_TIMEOUT"
	keyAPIRateLimitDefaultRPS         = "API_RATE_LIMIT_DEFAULT_RPS"
	keyAPIRateLimitDefaultBurst       = "API_RATE_LIMIT_DEFAULT_BURST"
	keyAPIRateLimitRouteOverridesJSON = "API_RATE_LIMIT_ROUTE_OVERRIDES_JSON"
	keyAPIRateLimitKeyPrefix          = "API_RATE_LIMIT_KEY_PREFIX"

	keyAPIValkeyMode  = "API_VALKEY_MODE"
	keyAPIValkeyAddrs = "API_VALKEY_ADDRS"

	keyOTELServiceName                = "OTEL_SERVICE_NAME"
	keyOTELSDKDisabled                = "OTEL_SDK_DISABLED"
	keyOTELTracesSampler              = "OTEL_TRACES_SAMPLER"
	keyOTELTracesSamplerArg           = "OTEL_TRACES_SAMPLER_ARG"
	keyOTELExporterOTLPEndpoint       = "OTEL_EXPORTER_OTLP_ENDPOINT"
	keyOTELExporterOTLPTracesEndpoint = "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"

	keyWeatherGeocodingBaseURL = "WEATHER_GEOCODING_BASE_URL"
	keyWeatherForecastBaseURL  = "WEATHER_FORECAST_BASE_URL"
	keyWeatherAPIKey           = "WEATHER_API_KEY" //nolint:gosec // Env var name, not a credential value.
	keyWeatherTimeout          = "WEATHER_TIMEOUT"

	keyDBURL                   = "DB_URL"
	keyDBMinConns              = "DB_MIN_CONNS"
	keyDBMaxConns              = "DB_MAX_CONNS"
	keyDBMaxConnLifetime       = "DB_MAX_CONN_LIFETIME"
	keyDBMaxConnIdleTime       = "DB_MAX_CONN_IDLE_TIME"
	keyDBHealthCheckPeriod     = "DB_HEALTH_CHECK_PERIOD"
	keyDBConnectTimeout        = "DB_CONNECT_TIMEOUT"
	keyDBMigrateOnStartup      = "DB_MIGRATE_ON_STARTUP"
	keyDBMigrateTimeout        = "DB_MIGRATE_TIMEOUT"
	keyDBStartupMaxAttempts    = "DB_STARTUP_MAX_ATTEMPTS"
	keyDBStartupBackoffInitial = "DB_STARTUP_BACKOFF_INITIAL"
	keyDBStartupBackoffMax     = "DB_STARTUP_BACKOFF_MAX"
)

func fileEnvKey(key string) string {
	return key + envFileSuffix
}

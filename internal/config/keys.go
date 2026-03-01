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

	keyOTELServiceName                = "OTEL_SERVICE_NAME"
	keyOTELSDKDisabled                = "OTEL_SDK_DISABLED"
	keyOTELTracesSampler              = "OTEL_TRACES_SAMPLER"
	keyOTELTracesSamplerArg           = "OTEL_TRACES_SAMPLER_ARG"
	keyOTELExporterOTLPEndpoint       = "OTEL_EXPORTER_OTLP_ENDPOINT"
	keyOTELExporterOTLPTracesEndpoint = "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"
)

func fileEnvKey(key string) string {
	return key + envFileSuffix
}

// Package config loads and validates application configuration.
//
// Configuration is loaded from environment variables with strict fail-fast
// validation. Every supported key also accepts a companion <KEY>_FILE
// variable whose value is read from a file path. This includes runtime
// transport, telemetry, and PostgreSQL connection-pool and migration settings.
//
// For PostgreSQL connections, DB_CONNECT_TIMEOUT is applied by runtime pool
// configuration and takes precedence over any connect_timeout query parameter
// in DB_URL. Set DB_CONNECT_TIMEOUT=0s to keep connect_timeout from DB_URL.
//
// The API runtime requires DB_URL to be set and fails fast at startup when
// it is missing.
//
// DB_MIGRATE_ON_STARTUP controls whether migrations run during API startup.
// DB_STARTUP_MAX_ATTEMPTS and DB_STARTUP_BACKOFF_* control retry behavior for
// transient database open/migration failures during startup.
//
// API_READYZ_TIMEOUT is an overall readiness budget shared across all
// configured readiness checks. When API rate limiting is configured fail-open,
// Valkey is excluded from readiness so instances remain ready while rate
// limiting degrades gracefully.
package config

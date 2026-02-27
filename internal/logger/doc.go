// Package logger provides a configured slog logger with support for JSON and text formats.
// Default attributes (version, environment) are included in every log entry.
// When request context includes an active OpenTelemetry span, trace_id and span_id
// are added to log records for correlation.
//
// Configuration via environment variables:
//   - LOG_FORMAT: "json" or "text" (default: "text")
//   - LOG_LEVEL: "debug", "info", "warn", "error" (default: "info")
package logger

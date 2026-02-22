// Package logger provides a configured slog logger with support for JSON and text formats.
// Default attributes (version, environment) are included in every log entry.
//
// Configuration via environment variables:
//   - LOG_FORMAT: "json" or "text" (default: "text")
//   - LOG_LEVEL: "debug", "info", "warn", "error" (default: "info")
package logger

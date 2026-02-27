package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

type Format string

const (
	FormatText    Format = "text"
	FormatJSON    Format = "json"
	DefaultFormat        = FormatText
)

func (f Format) String() string {
	return string(f)
}

func ParseFormat(value string) (Format, bool) {
	parsed := Format(strings.ToLower(strings.TrimSpace(value)))
	if !IsValidFormat(parsed) {
		return "", false
	}

	return parsed, true
}

func IsValidFormat(format Format) bool {
	switch format {
	case FormatText, FormatJSON:
		return true
	default:
		return false
	}
}

type Config struct {
	Format      Format
	Level       slog.Level
	Version     string
	Environment string
	Writer      io.Writer
}

func New(cfg Config) {
	opts := &slog.HandlerOptions{
		AddSource: true,
		Level:     cfg.Level,
	}

	writer := cfg.Writer
	if writer == nil {
		writer = os.Stderr
	}

	var handler slog.Handler
	if cfg.Format == FormatJSON {
		handler = slog.NewJSONHandler(writer, opts)
	} else {
		handler = slog.NewTextHandler(writer, opts)
	}

	handler = newTraceContextHandler(handler)

	attrs := []slog.Attr{
		slog.String("version", cfg.Version),
		slog.String("environment", cfg.Environment),
	}

	logger := slog.New(handler.WithAttrs(attrs))
	slog.SetDefault(logger)
}

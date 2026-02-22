package logger

import (
	"io"
	"log/slog"
	"os"
)

type Config struct {
	Format      string
	Level       string
	Version     string
	Environment string
	Writer      io.Writer
}

func New(cfg Config) {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}

	writer := cfg.Writer
	if writer == nil {
		writer = os.Stderr
	}

	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(writer, opts)
	} else {
		handler = slog.NewTextHandler(writer, opts)
	}

	attrs := []slog.Attr{
		slog.String("version", cfg.Version),
		slog.String("environment", cfg.Environment),
	}

	logger := slog.New(handler.WithAttrs(attrs))
	slog.SetDefault(logger)
}

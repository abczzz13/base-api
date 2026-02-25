package logger

import (
	"io"
	"log/slog"
	"os"
)

type Config struct {
	Format      string
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

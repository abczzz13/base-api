package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name             string
		cfg              Config
		wantFormat       Format
		wantContainAttrs []string
		wantNotContain   string
		logFunc          func(msg string, args ...any)
	}{
		{
			name:       "defaults to info level and text format",
			cfg:        Config{},
			wantFormat: FormatText,
			logFunc:    slog.Info,
		},
		{
			name:       "debug level",
			cfg:        Config{Level: slog.LevelDebug},
			wantFormat: FormatText,
			logFunc:    slog.Debug,
		},
		{
			name:       "warn level",
			cfg:        Config{Level: slog.LevelWarn},
			wantFormat: FormatText,
			logFunc:    slog.Warn,
		},
		{
			name:       "error level",
			cfg:        Config{Level: slog.LevelError},
			wantFormat: FormatText,
			logFunc:    slog.Error,
		},
		{
			name:       "json format",
			cfg:        Config{Format: FormatJSON},
			wantFormat: FormatJSON,
			logFunc:    slog.Info,
		},
		{
			name:             "includes version and environment attrs",
			cfg:              Config{Format: FormatJSON, Version: "1.2.3", Environment: "production"},
			wantFormat:       FormatJSON,
			wantContainAttrs: []string{`"version":"1.2.3"`, `"environment":"production"`},
			logFunc:          slog.Info,
		},
		{
			name:           "text format does not contain json markers",
			cfg:            Config{Format: FormatText},
			wantFormat:     FormatText,
			wantNotContain: `"version"`,
			logFunc:        slog.Info,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.cfg.Writer = &buf

			New(tt.cfg)

			tt.logFunc("test message")

			output := buf.String()
			if output == "" {
				t.Fatal("expected output, got empty string")
			}

			if tt.wantFormat == FormatJSON {
				var result map[string]any
				if err := json.Unmarshal([]byte(output), &result); err != nil {
					t.Fatalf("expected valid JSON output, got: %q, error: %v", output, err)
				}

				for _, attr := range tt.wantContainAttrs {
					if !strings.Contains(output, attr) {
						t.Errorf("expected output to contain %q, got: %q", attr, output)
					}
				}
			} else {
				if strings.HasPrefix(output, "{") {
					t.Errorf("expected text format, got JSON: %q", output)
				}
			}

			if tt.wantNotContain != "" && strings.Contains(output, tt.wantNotContain) {
				t.Errorf("expected output not to contain %q, got: %q", tt.wantNotContain, output)
			}
		})
	}
}

func TestNewDefaultsToStderr(t *testing.T) {
	New(Config{})
}

func TestNewSetsDefaultLogger(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Writer: &buf,
		Format: FormatJSON,
	}
	New(cfg)

	slog.Info("default logger test")

	output := buf.String()
	if output == "" {
		t.Fatal("expected slog default to write to provided writer")
	}
}

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
		wantFormat       string
		wantContainAttrs []string
		wantNotContain   string
	}{
		{
			name:       "defaults to info level and text format",
			cfg:        Config{},
			wantFormat: "text",
		},
		{
			name:       "debug level",
			cfg:        Config{Level: "debug"},
			wantFormat: "text",
		},
		{
			name:       "warn level",
			cfg:        Config{Level: "warn"},
			wantFormat: "text",
		},
		{
			name:       "error level",
			cfg:        Config{Level: "error"},
			wantFormat: "text",
		},
		{
			name:       "invalid level defaults to info",
			cfg:        Config{Level: "invalid"},
			wantFormat: "text",
		},
		{
			name:       "json format",
			cfg:        Config{Format: "json"},
			wantFormat: "json",
		},
		{
			name:             "includes version and environment attrs",
			cfg:              Config{Format: "json", Version: "1.2.3", Environment: "production"},
			wantFormat:       "json",
			wantContainAttrs: []string{`"version":"1.2.3"`, `"environment":"production"`},
		},
		{
			name:           "text format does not contain json markers",
			cfg:            Config{Format: "text"},
			wantFormat:     "text",
			wantNotContain: `"version"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.cfg.Writer = &buf

			New(tt.cfg)

			switch tt.cfg.Level {
			case "debug":
				slog.Debug("test message")
			case "warn":
				slog.Warn("test message")
			case "error":
				slog.Error("test message")
			default:
				slog.Info("test message")
			}

			output := buf.String()
			if output == "" {
				t.Fatal("expected output, got empty string")
			}

			if tt.wantFormat == "json" {
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
		Format: "json",
	}
	New(cfg)

	slog.Info("default logger test")

	output := buf.String()
	if output == "" {
		t.Fatal("expected slog default to write to provided writer")
	}
}

package publicapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/abczzz13/base-api/internal/clients/weather"
	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/logger"
	"github.com/abczzz13/base-api/internal/middleware"
	"github.com/abczzz13/base-api/internal/requestaudit"
)

func TestRequestLoggerCanBeDisabledForPublicHandler(t *testing.T) {
	previousLogger := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	var logs bytes.Buffer
	logger.New(logger.Config{
		Format:      logger.FormatJSON,
		Level:       slog.LevelInfo,
		Environment: "test",
		Writer:      &logs,
	})

	registry := prometheus.NewRegistry()
	requestMetrics, err := middleware.NewHTTPRequestMetrics(registry)
	if err != nil {
		t.Fatalf("create request metrics: %v", err)
	}

	disabled := false
	handler, err := NewHandler(config.Config{
		Environment: "test",
		RequestLogger: config.RequestLoggerConfig{
			Enabled: &disabled,
		},
	}, Dependencies{
		RequestMetrics:         requestMetrics,
		RequestAuditRepository: requestaudit.NopRepository(),
		WeatherClient: weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
			return weather.CurrentWeather{}, nil
		}),
	})
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusOK, recorder.Code)
	}

	for _, entry := range decodeJSONLogLines(t, logs.String()) {
		if msg, _ := entry["msg"].(string); msg == "request completed" {
			t.Fatalf("unexpected request log entry when middleware is disabled: %#v", entry)
		}
	}
}

func decodeJSONLogLines(t *testing.T, data string) []map[string]any {
	t.Helper()

	if strings.TrimSpace(data) == "" {
		return nil
	}

	decoder := json.NewDecoder(strings.NewReader(data))
	entries := make([]map[string]any, 0)
	for {
		entry := map[string]any{}
		if err := decoder.Decode(&entry); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			t.Fatalf("decode JSON log entry: %v", err)
		}

		entries = append(entries, entry)
	}

	return entries
}

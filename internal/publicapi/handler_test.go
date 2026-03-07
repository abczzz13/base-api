package publicapi

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/logger"
	"github.com/abczzz13/base-api/internal/middleware"
	"github.com/abczzz13/base-api/internal/requestaudit"
)

func TestNewPublicHandlerCORSAndCSRFInteraction(t *testing.T) {
	t.Run("allows preflight for configured origin", func(t *testing.T) {
		handler, err := newPublicHandlerForTest(t, config.Config{
			Environment: "test",
			CORS: config.CORSConfig{
				AllowedOrigins: []string{"https://client.example"},
			},
			CSRF: config.CSRFConfig{Enabled: true},
		})
		if err != nil {
			t.Fatalf("NewHandler returned error: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/healthz", nil)
		req.Header.Set("Origin", "https://client.example")
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
		req.Header.Set("Access-Control-Request-Headers", "Content-Type")

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Fatalf("status mismatch: want %d, got %d", http.StatusNoContent, rr.Code)
		}

		for headerName, wantValue := range map[string]string{
			"Access-Control-Allow-Origin":  "https://client.example",
			"Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, PATCH, OPTIONS",
			"Access-Control-Allow-Headers": "Content-Type, Authorization, Accept",
		} {
			if got := rr.Header().Get(headerName); got != wantValue {
				t.Fatalf("header %q mismatch: want %q, got %q", headerName, wantValue, got)
			}
		}
	})

	t.Run("denies unsafe cross-origin request from untrusted origin", func(t *testing.T) {
		handler, err := newPublicHandlerForTest(t, config.Config{
			Environment: "test",
			CORS: config.CORSConfig{
				AllowedOrigins: []string{"https://trusted.example"},
			},
			CSRF: config.CSRFConfig{
				Enabled:        true,
				TrustedOrigins: []string{"https://trusted.example"},
			},
		})
		if err != nil {
			t.Fatalf("NewHandler returned error: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
		req.Header.Set("Origin", "https://evil.example")
		req.Header.Set("Sec-Fetch-Site", "cross-site")

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("status mismatch: want %d, got %d", http.StatusForbidden, rr.Code)
		}
		if got := rr.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
			t.Fatalf("content-type mismatch: want %q, got %q", "application/json; charset=utf-8", got)
		}
		if got := rr.Header().Get("X-Request-Id"); got == "" {
			t.Fatal("expected non-empty X-Request-Id response header")
		}
		if got := rr.Body.String(); !strings.Contains(got, `"code":"forbidden"`) || !strings.Contains(got, `"message":"cross-origin request denied"`) || !strings.Contains(got, `"requestId":"`) {
			t.Fatalf("body mismatch: got %q", got)
		}
	})

	t.Run("trusted cross-origin requests pass CSRF layer", func(t *testing.T) {
		baseCfg := config.Config{
			Environment: "test",
			CORS: config.CORSConfig{
				AllowedOrigins: []string{"https://trusted.example"},
			},
		}

		handlerWithoutCSRF, err := newPublicHandlerForTest(t, baseCfg)
		if err != nil {
			t.Fatalf("NewHandler without CSRF returned error: %v", err)
		}

		cfgWithCSRF := baseCfg
		cfgWithCSRF.CSRF = config.CSRFConfig{
			Enabled:        true,
			TrustedOrigins: []string{"https://trusted.example"},
		}
		handlerWithCSRF, err := newPublicHandlerForTest(t, cfgWithCSRF)
		if err != nil {
			t.Fatalf("NewHandler with CSRF returned error: %v", err)
		}

		newCrossSiteRequest := func() *http.Request {
			req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
			req.Header.Set("Origin", "https://trusted.example")
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			return req
		}

		rrWithoutCSRF := httptest.NewRecorder()
		handlerWithoutCSRF.ServeHTTP(rrWithoutCSRF, newCrossSiteRequest())

		if rrWithoutCSRF.Code == http.StatusForbidden {
			t.Fatalf("expected baseline request without CSRF to be non-forbidden")
		}

		rrWithCSRF := httptest.NewRecorder()
		handlerWithCSRF.ServeHTTP(rrWithCSRF, newCrossSiteRequest())

		if rrWithCSRF.Code == http.StatusForbidden {
			t.Fatalf("expected trusted origin request to pass CSRF layer")
		}
		if rrWithCSRF.Code != rrWithoutCSRF.Code {
			t.Fatalf("status mismatch with trusted CSRF origin: want %d, got %d", rrWithoutCSRF.Code, rrWithCSRF.Code)
		}
		if rrWithCSRF.Body.String() != rrWithoutCSRF.Body.String() {
			t.Fatalf("body mismatch with trusted CSRF origin: want %q, got %q", rrWithoutCSRF.Body.String(), rrWithCSRF.Body.String())
		}
	})
}

func TestNewPublicHandlerTracingIncludesTraceContextInLogs(t *testing.T) {
	previousLogger := slog.Default()
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})

	var logs bytes.Buffer
	logger.New(logger.Config{
		Format:      logger.FormatJSON,
		Level:       slog.LevelDebug,
		Environment: "test",
		Writer:      &logs,
	})

	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
	})
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	handler, err := newPublicHandlerForTest(t, config.Config{
		Environment: "test",
		OTEL: config.OTELConfig{
			TracingEnabled: true,
		},
	})
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusOK, rr.Code)
	}
	if got := rr.Header().Get(middleware.TraceIDResponseHeader); got == "" {
		t.Fatalf("expected non-empty %s response header", middleware.TraceIDResponseHeader)
	}

	entries := decodeJSONLogLines(t, logs.String())
	var completed map[string]any
	for _, entry := range entries {
		if msg, _ := entry["msg"].(string); msg == "request completed" {
			completed = entry
			break
		}
	}
	if completed == nil {
		t.Fatalf("expected request completed log entry, got %#v", entries)
	}

	traceID, ok := completed["trace_id"].(string)
	if !ok || traceID == "" {
		t.Fatalf("expected non-empty trace_id in request log, got %#v", completed["trace_id"])
	}
	spanID, ok := completed["span_id"].(string)
	if !ok || spanID == "" {
		t.Fatalf("expected non-empty span_id in request log, got %#v", completed["span_id"])
	}
}

func TestNewPublicHandlerTracingAddsOperationAttributesToSpans(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
	})
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	handler, err := newPublicHandlerForTest(t, config.Config{
		Environment: "test",
		OTEL: config.OTELConfig{
			TracingEnabled: true,
		},
	})
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusOK, rr.Code)
	}

	spans := recorder.Ended()
	if len(spans) == 0 {
		t.Fatal("expected at least one ended span")
	}

	matches := make([]sdktrace.ReadOnlySpan, 0, len(spans))
	for _, span := range spans {
		for _, attr := range span.Attributes() {
			if string(attr.Key) == "api.operation.id" && attr.Value.Type() == attribute.STRING && attr.Value.AsString() == "getHealthz" {
				matches = append(matches, span)
				break
			}
		}
	}

	if len(matches) != 1 {
		names := make([]string, 0, len(spans))
		for _, span := range spans {
			names = append(names, span.Name())
		}
		t.Fatalf("expected exactly one span with api.operation.id=getHealthz, got %d spans (names=%v)", len(matches), names)
	}

	targetSpan := matches[0]
	if got := targetSpan.Name(); got != "GET getHealthz" {
		t.Fatalf("span name mismatch: want %q, got %q", "GET getHealthz", got)
	}

	attrs := map[string]string{}
	for _, attr := range targetSpan.Attributes() {
		if attr.Value.Type() != attribute.STRING {
			continue
		}
		attrs[string(attr.Key)] = attr.Value.AsString()
	}

	for key, want := range map[string]string{
		"api.operation.id":      "getHealthz",
		"api.operation.name":    "GetHealthz",
		"api.operation.summary": "Public health endpoint",
	} {
		if got := attrs[key]; got != want {
			t.Fatalf("span attribute %q mismatch: want %q, got %q", key, want, got)
		}
	}
}

func newPublicHandlerForTest(t *testing.T, cfg config.Config) (http.Handler, error) {
	t.Helper()

	registry := prometheus.NewRegistry()
	requestMetrics, err := middleware.NewHTTPRequestMetrics(registry)
	if err != nil {
		t.Fatalf("create request metrics: %v", err)
	}

	return NewHandler(cfg, Dependencies{
		RequestMetrics:         requestMetrics,
		RequestAuditRepository: requestaudit.NopRepository(),
	})
}

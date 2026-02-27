package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTracingCapturesRequests(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
	})

	tests := []struct {
		name      string
		path      string
		wantSpans int
	}{
		{name: "metrics path is traced when middleware is applied", path: "/metrics", wantSpans: 1},
		{name: "liveness path is traced when middleware is applied", path: "/livez", wantSpans: 1},
		{name: "readiness path is traced when middleware is applied", path: "/readyz", wantSpans: 1},
		{name: "metrics trailing slash is traced when middleware is applied", path: "/metrics/", wantSpans: 1},
		{name: "regular endpoint is traced", path: "/healthz", wantSpans: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			t.Cleanup(func() {
				_ = provider.Shutdown(context.Background())
			})
			otel.SetTracerProvider(provider)

			handler := Tracing("infra")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if got := len(recorder.Ended()); got != tt.wantSpans {
				t.Fatalf("span count mismatch for %q: want %d, got %d", tt.path, tt.wantSpans, got)
			}
		})
	}
}

func TestTracingFallbackSpanNameDoesNotIncludeRawPath(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
	})

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
	})
	otel.SetTracerProvider(provider)

	handler := Tracing("public")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected exactly one span, got %d", len(spans))
	}

	if got := spans[0].Name(); got != http.MethodGet {
		t.Fatalf("span name mismatch: want %q, got %q", http.MethodGet, got)
	}
	if strings.Contains(spans[0].Name(), "/") {
		t.Fatalf("span name should not include raw path: %q", spans[0].Name())
	}
}

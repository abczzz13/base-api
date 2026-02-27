package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestTraceResponseHeader(t *testing.T) {
	t.Run("adds trace header when context has span", func(t *testing.T) {
		traceID, err := trace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
		if err != nil {
			t.Fatalf("parse trace id: %v", err)
		}
		spanID, err := trace.SpanIDFromHex("0123456789abcdef")
		if err != nil {
			t.Fatalf("parse span id: %v", err)
		}

		ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    traceID,
			SpanID:     spanID,
			TraceFlags: trace.FlagsSampled,
		}))

		handler := TraceResponseHeader()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if got := rr.Header().Get(TraceIDResponseHeader); got != traceID.String() {
			t.Fatalf("trace header mismatch: want %q, got %q", traceID.String(), got)
		}
	})

	t.Run("omits trace header without span", func(t *testing.T) {
		handler := TraceResponseHeader()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if got := rr.Header().Get(TraceIDResponseHeader); got != "" {
			t.Fatalf("expected empty %s header, got %q", TraceIDResponseHeader, got)
		}
	})
}

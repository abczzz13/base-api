package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abczzz13/base-api/internal/requestid"
)

func TestRequestIDPreservesValidInboundHeader(t *testing.T) {
	var gotRequestID string
	handler := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestID = requestid.FromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(requestid.HeaderName, "client-req-123")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if gotRequestID != "client-req-123" {
		t.Fatalf("request ID mismatch: want %q, got %q", "client-req-123", gotRequestID)
	}
	if got := rec.Header().Get(requestid.HeaderName); got != "client-req-123" {
		t.Fatalf("response header mismatch: want %q, got %q", "client-req-123", got)
	}
}

func TestRequestIDGeneratesHeaderForMissingOrInvalidInboundValue(t *testing.T) {
	tests := []struct {
		name        string
		headerValue string
	}{
		{name: "missing header"},
		{name: "invalid header", headerValue: "bad value with spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotRequestID string
			handler := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotRequestID = requestid.FromContext(r.Context())
				w.WriteHeader(http.StatusNoContent)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.headerValue != "" {
				req.Header.Set(requestid.HeaderName, tt.headerValue)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if gotRequestID == "" {
				t.Fatal("expected generated request ID in context")
			}
			if got := rec.Header().Get(requestid.HeaderName); got == "" {
				t.Fatal("expected generated request ID response header")
			} else if got != gotRequestID {
				t.Fatalf("generated request ID mismatch between header and context: header=%q context=%q", got, gotRequestID)
			}
		})
	}
}

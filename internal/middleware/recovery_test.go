package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/abczzz13/base-api/internal/requestid"
)

func TestRecovery(t *testing.T) {
	tests := []struct {
		name       string
		panicMsg   string
		wantStatus int
		wantBody   string
		wantCalled bool
	}{
		{
			name:       "recovers from panic and returns 500",
			panicMsg:   "test panic",
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"code":"internal_error","message":"internal server error"}`,
			wantCalled: false,
		},
		{
			name:       "passes through non-panicking requests",
			panicMsg:   "",
			wantStatus: http.StatusOK,
			wantBody:   "ok",
			wantCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.panicMsg != "" {
					panic(tt.panicMsg)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			})

			req := httptest.NewRequest("GET", "/test", nil)
			rec := httptest.NewRecorder()

			Recovery()(handler).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rec.Code)
			}

			if rec.Body.String() != tt.wantBody {
				t.Errorf("expected body %q, got %q", tt.wantBody, rec.Body.String())
			}

			if tt.panicMsg == "" {
				if rec.Header().Get("Content-Type") != "" {
					t.Errorf("expected no content-type for non-panic, got %s", rec.Header().Get("Content-Type"))
				}
			} else {
				if rec.Header().Get("Content-Type") != "application/json; charset=utf-8" {
					t.Errorf("expected content-type application/json; charset=utf-8, got %s", rec.Header().Get("Content-Type"))
				}
			}
		})
	}
}

func TestRecoveryIncludesRequestIDInErrorResponse(t *testing.T) {
	handler := RequestID()(Recovery()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.Header.Set(requestid.HeaderName, "req-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}

	wantBody := `{"code":"internal_error","message":"internal server error","requestId":"req-123"}`
	if rec.Body.String() != wantBody {
		t.Fatalf("expected body %q, got %q", wantBody, rec.Body.String())
	}
}

func TestRecoveryIncrementsRecoveredPanicMetricWithFallbackRequestLabels(t *testing.T) {
	requestMetrics, _ := newTestHTTPRequestMetrics(t)

	server := requestMetricsServerUnknown
	method := http.MethodGet
	route := RequestMetricsRouteUnmatched
	before := testutil.ToFloat64(requestMetrics.httpRecoveredPanicsTotal.WithLabelValues(server, method, route))

	handler := RequestMetrics(requestMetrics, RequestMetricsConfig{})(Recovery()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}

	after := testutil.ToFloat64(requestMetrics.httpRecoveredPanicsTotal.WithLabelValues(server, method, route))
	if got := after - before; got != 1 {
		t.Fatalf("recovered panic counter delta mismatch: want 1, got %v", got)
	}
}

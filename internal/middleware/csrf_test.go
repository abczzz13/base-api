package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestCSRF(t *testing.T) {
	tests := []struct {
		name          string
		cfg           CSRFConfig
		method        string
		origin        string
		secFetchSite  string
		wantStatus    int
		wantBody      string
		handlerCalled bool
	}{
		{
			name:          "allows safe methods",
			cfg:           CSRFConfig{},
			method:        "GET",
			origin:        "https://evil.com",
			wantStatus:    http.StatusOK,
			handlerCalled: true,
		},
		{
			name:          "allows same-origin POST requests",
			cfg:           CSRFConfig{},
			method:        "POST",
			secFetchSite:  "same-origin",
			wantStatus:    http.StatusOK,
			handlerCalled: true,
		},
		{
			name: "denies cross-origin POST requests without trusted origin",
			cfg: CSRFConfig{
				TrustedOrigins: []string{"https://trusted.com"},
			},
			method:        "POST",
			origin:        "https://evil.com",
			secFetchSite:  "cross-site",
			wantStatus:    http.StatusForbidden,
			wantBody:      `{"code":"forbidden","message":"cross-origin request denied"}`,
			handlerCalled: false,
		},
		{
			name: "allows cross-origin POST requests with trusted origin",
			cfg: CSRFConfig{
				TrustedOrigins: []string{"https://trusted.com"},
			},
			method:        "POST",
			origin:        "https://trusted.com",
			secFetchSite:  "cross-site",
			wantStatus:    http.StatusOK,
			handlerCalled: true,
		},
		{
			name: "ignores invalid trusted origin values",
			cfg: CSRFConfig{
				TrustedOrigins: []string{"not-a-valid-origin", "https://trusted.com"},
			},
			method:        "POST",
			origin:        "https://trusted.com",
			secFetchSite:  "cross-site",
			wantStatus:    http.StatusOK,
			handlerCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handlerCalled := false
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest(tt.method, "/test", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.secFetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tt.secFetchSite)
			}
			rec := httptest.NewRecorder()

			CSRF(tt.cfg)(handler).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rec.Code)
			}

			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Errorf("expected body %q, got %q", tt.wantBody, rec.Body.String())
			}

			if tt.wantStatus == http.StatusForbidden {
				if rec.Header().Get("Content-Type") != "application/json; charset=utf-8" {
					t.Errorf("expected content-type application/json; charset=utf-8, got %s", rec.Header().Get("Content-Type"))
				}
			}

			if handlerCalled != tt.handlerCalled {
				t.Errorf("expected handler called=%v, got %v", tt.handlerCalled, handlerCalled)
			}
		})
	}
}

func TestCSRFDenialIncrementsRejectedRequestMetric(t *testing.T) {
	requestMetrics, _ := newTestHTTPRequestMetrics(t)

	server := "csrf_rejection_metric"
	method := http.MethodPost
	route := "createSession"
	reason := RequestRejectionReasonCSRF
	before := testutil.ToFloat64(requestMetrics.httpRejectedRequestsTotal.WithLabelValues(server, method, route, reason))

	handler := RequestMetrics(requestMetrics, RequestMetricsConfig{
		Server: server,
		RouteLabel: func(*http.Request) string {
			return route
		},
	})(CSRF(CSRFConfig{
		TrustedOrigins: []string{"https://trusted.com"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(method, "/sessions", nil)
	req.Header.Set("Origin", "https://evil.com")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}

	after := testutil.ToFloat64(requestMetrics.httpRejectedRequestsTotal.WithLabelValues(server, method, route, reason))
	if got := after - before; got != 1 {
		t.Fatalf("rejected request counter delta mismatch: want 1, got %v", got)
	}
}

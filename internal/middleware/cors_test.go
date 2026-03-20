package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestCORS(t *testing.T) {
	tests := []struct {
		name               string
		cfg                CORSConfig
		method             string
		origin             string
		requestHeaders     map[string]string
		wantStatus         int
		wantHeaders        map[string]string
		wantMissingHeaders []string
		wantVaryContains   []string
		handlerCalled      bool
	}{
		{
			name:             "denies cross-origin requests by default",
			cfg:              CORSConfig{},
			method:           http.MethodGet,
			origin:           "https://example.com",
			wantStatus:       http.StatusOK,
			wantHeaders:      map[string]string{},
			wantVaryContains: []string{"Origin"},
			wantMissingHeaders: []string{
				"Access-Control-Allow-Origin",
				"Access-Control-Allow-Credentials",
			},
			handlerCalled: true,
		},
		{
			name: "adds CORS headers to allowed origin",
			cfg: CORSConfig{
				AllowedOrigins: []string{"https://example.com"},
			},
			method:           http.MethodGet,
			origin:           "https://example.com",
			wantStatus:       http.StatusOK,
			wantHeaders:      map[string]string{"Access-Control-Allow-Origin": "https://example.com"},
			wantVaryContains: []string{"Origin"},
			handlerCalled:    true,
		},
		{
			name: "disallowed origin does not receive CORS headers",
			cfg: CORSConfig{
				AllowedOrigins: []string{"https://allowed.example"},
			},
			method:           http.MethodGet,
			origin:           "https://denied.example",
			wantStatus:       http.StatusOK,
			wantHeaders:      map[string]string{},
			wantVaryContains: []string{"Origin"},
			wantMissingHeaders: []string{
				"Access-Control-Allow-Origin",
				"Access-Control-Allow-Credentials",
			},
			handlerCalled: true,
		},
		{
			name: "allows wildcard origins when credentials are disabled",
			cfg: CORSConfig{
				AllowedOrigins: []string{"*"},
			},
			method:           http.MethodGet,
			origin:           "https://any-origin.com",
			wantStatus:       http.StatusOK,
			wantHeaders:      map[string]string{"Access-Control-Allow-Origin": "https://any-origin.com"},
			wantVaryContains: []string{"Origin"},
			wantMissingHeaders: []string{
				"Access-Control-Allow-Credentials",
			},
			handlerCalled: true,
		},
		{
			name: "wildcard origin is ignored when credentials are enabled",
			cfg: CORSConfig{
				AllowedOrigins:   []string{"*"},
				AllowCredentials: true,
			},
			method:           http.MethodGet,
			origin:           "https://example.com",
			wantStatus:       http.StatusOK,
			wantHeaders:      map[string]string{},
			wantVaryContains: []string{"Origin"},
			wantMissingHeaders: []string{
				"Access-Control-Allow-Origin",
				"Access-Control-Allow-Credentials",
			},
			handlerCalled: true,
		},
		{
			name: "sets credentials header when configured with explicit allowlist",
			cfg: CORSConfig{
				AllowedOrigins:   []string{"https://example.com"},
				AllowCredentials: true,
			},
			method:     http.MethodGet,
			origin:     "https://example.com",
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin":      "https://example.com",
				"Access-Control-Allow-Credentials": "true",
			},
			wantVaryContains: []string{"Origin"},
			handlerCalled:    true,
		},
		{
			name: "handles preflight OPTIONS request",
			cfg: CORSConfig{
				AllowedOrigins: []string{"https://example.com"},
			},
			method: http.MethodOptions,
			origin: "https://example.com",
			requestHeaders: map[string]string{
				"Access-Control-Request-Method":  http.MethodPost,
				"Access-Control-Request-Headers": "Content-Type",
			},
			wantStatus: http.StatusNoContent,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin":  "https://example.com",
				"Access-Control-Max-Age":       "300",
				"Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, PATCH, OPTIONS",
				"Access-Control-Allow-Headers": "Content-Type, Authorization, Accept",
			},
			wantVaryContains: []string{
				"Origin",
				"Access-Control-Request-Method",
				"Access-Control-Request-Headers",
			},
			handlerCalled: false,
		},
		{
			name: "no CORS headers without Origin header",
			cfg: CORSConfig{
				AllowedOrigins: []string{"https://example.com"},
			},
			method:      http.MethodGet,
			origin:      "",
			wantStatus:  http.StatusOK,
			wantHeaders: map[string]string{},
			wantVaryContains: []string{
				"Origin",
			},
			wantMissingHeaders: []string{
				"Access-Control-Allow-Origin",
			},
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
			for headerName, value := range tt.requestHeaders {
				req.Header.Set(headerName, value)
			}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			rec := httptest.NewRecorder()

			CORS(tt.cfg)(handler).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rec.Code)
			}

			for header, wantValue := range tt.wantHeaders {
				gotValue := rec.Header().Get(header)
				if gotValue != wantValue {
					t.Errorf("expected %s %q, got %q", header, wantValue, gotValue)
				}
			}

			for _, header := range tt.wantMissingHeaders {
				if gotValue := rec.Header().Get(header); gotValue != "" {
					t.Errorf("expected %s to be empty, got %q", header, gotValue)
				}
			}

			varyValue := strings.Join(rec.Header().Values("Vary"), ",")
			for _, expected := range tt.wantVaryContains {
				if !strings.Contains(varyValue, expected) {
					t.Errorf("expected Vary %q to contain %q", varyValue, expected)
				}
			}

			if handlerCalled != tt.handlerCalled {
				t.Errorf("expected handler called=%v, got %v", tt.handlerCalled, handlerCalled)
			}
		})
	}
}

func TestAddVaryDeduplicatesValues(t *testing.T) {
	header := http.Header{}
	header.Add("Vary", "Origin")

	addVary(header, "Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers")

	vary := header.Values("Vary")
	if len(vary) != 3 {
		t.Fatalf("expected 3 vary values, got %d (%v)", len(vary), vary)
	}
}

func TestCORSPolicyDenialIncrementsMetric(t *testing.T) {
	requestMetrics, _ := newTestHTTPRequestMetrics(t)

	server := "cors_policy_denial_metric"
	method := http.MethodGet
	route := "GetHealthz"
	before := testutil.ToFloat64(requestMetrics.httpCORSPolicyDenialsTotal.WithLabelValues(server, method, route))

	handler := RequestMetrics(requestMetrics, RequestMetricsConfig{
		Server: server,
		RouteLabel: func(*http.Request) string {
			return route
		},
	})(CORS(CORSConfig{
		AllowedOrigins: []string{"https://allowed.example"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))

	req := httptest.NewRequest(method, "/healthz", nil)
	req.Header.Set("Origin", "https://denied.example")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}

	after := testutil.ToFloat64(requestMetrics.httpCORSPolicyDenialsTotal.WithLabelValues(server, method, route))
	if got := after - before; got != 1 {
		t.Fatalf("CORS policy denial counter delta mismatch: want 1, got %v", got)
	}
}

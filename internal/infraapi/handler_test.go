package infraapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	dto "github.com/prometheus/client_model/go"

	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/middleware"
)

func TestNewInfraHandlerRoutesMetricsThroughPromHTTP(t *testing.T) {
	requestMetrics, registry := newInfraRequestMetricsForTest(t)
	handler, err := NewHandler(config.Config{Environment: "test"}, Dependencies{
		RequestMetrics:  requestMetrics,
		MetricsGatherer: registry,
	})
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	tests := []struct {
		name                    string
		method                  string
		path                    string
		requestHeaders          map[string]string
		wantStatus              int
		wantHeaders             map[string]string
		wantContentTypeContains []string
		wantBodyContains        []string
	}{
		{
			name:       "metrics GET uses promhttp content negotiation",
			method:     http.MethodGet,
			path:       "/metrics",
			wantStatus: http.StatusOK,
			wantContentTypeContains: []string{
				"text/plain",
				"version=0.0.4",
			},
			wantBodyContains: []string{"# HELP", "# TYPE"},
		},
		{
			name:   "metrics GET supports openmetrics negotiation",
			method: http.MethodGet,
			path:   "/metrics",
			requestHeaders: map[string]string{
				"Accept": "application/openmetrics-text; version=1.0.0; charset=utf-8",
			},
			wantStatus: http.StatusOK,
			wantContentTypeContains: []string{
				"application/openmetrics-text",
			},
		},
		{
			name:       "metrics HEAD is routed through promhttp",
			method:     http.MethodHead,
			path:       "/metrics",
			wantStatus: http.StatusOK,
			wantContentTypeContains: []string{
				"text/plain",
				"version=0.0.4",
			},
		},
		{
			name:       "metrics POST is not exposed",
			method:     http.MethodPost,
			path:       "/metrics",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "metrics OPTIONS is not exposed",
			method:     http.MethodOptions,
			path:       "/metrics",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			for headerName, headerValue := range tt.requestHeaders {
				req.Header.Set(headerName, headerValue)
			}

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("status mismatch: want %d, got %d", tt.wantStatus, rr.Code)
			}

			for headerName, wantValue := range tt.wantHeaders {
				if got := rr.Header().Get(headerName); got != wantValue {
					t.Fatalf("header %q mismatch: want %q, got %q", headerName, wantValue, got)
				}
			}

			contentType := rr.Header().Get("Content-Type")
			for _, want := range tt.wantContentTypeContains {
				if !strings.Contains(contentType, want) {
					t.Fatalf("content type %q does not contain %q", contentType, want)
				}
			}

			body := rr.Body.String()
			for _, want := range tt.wantBodyContains {
				if !strings.Contains(body, want) {
					t.Fatalf("body does not contain %q", want)
				}
			}
		})
	}
}

func TestNewInfraHandlerWiresDocumentationRoutes(t *testing.T) {
	requestMetrics, registry := newInfraRequestMetricsForTest(t)
	handler, err := NewHandler(config.Config{Environment: "test"}, Dependencies{
		RequestMetrics:  requestMetrics,
		MetricsGatherer: registry,
	})
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	tests := []struct {
		name             string
		method           string
		path             string
		wantStatus       int
		wantHeaders      map[string]string
		wantBodyContains []string
	}{
		{
			name:       "swagger endpoint is exposed through infra mux",
			method:     http.MethodGet,
			path:       "/swagger",
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Content-Security-Policy": "default-src 'self'; connect-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; font-src 'self' data:; worker-src 'self' blob:; frame-ancestors 'none'; base-uri 'none'; form-action 'none'",
			},
			wantBodyContains: []string{
				"\"urls.primaryName\": \"Public API\"",
			},
		},
		{
			name:       "docs endpoint redirects through infra mux",
			method:     http.MethodGet,
			path:       "/docs",
			wantStatus: http.StatusTemporaryRedirect,
			wantHeaders: map[string]string{
				"Location": "/swagger",
			},
		},
		{
			name:       "public spec is exposed through infra mux",
			method:     http.MethodGet,
			path:       "/openapi/public.yaml",
			wantStatus: http.StatusOK,
			wantBodyContains: []string{
				"title: Base API",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("status mismatch: want %d, got %d", tt.wantStatus, rr.Code)
			}

			for headerName, wantValue := range tt.wantHeaders {
				if got := rr.Header().Get(headerName); got != wantValue {
					t.Fatalf("header %q mismatch: want %q, got %q", headerName, wantValue, got)
				}
			}

			body := rr.Body.String()
			for _, want := range tt.wantBodyContains {
				if !strings.Contains(body, want) {
					t.Fatalf("body does not contain %q", want)
				}
			}
		})
	}
}

func TestNewInfraHandlerRecoversPanicsFromMetricsGatherer(t *testing.T) {
	requestMetrics, _ := newInfraRequestMetricsForTest(t)
	handler, err := NewHandler(config.Config{Environment: "test"}, Dependencies{
		RequestMetrics:  requestMetrics,
		MetricsGatherer: panicGatherer{},
	})
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusInternalServerError, rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content-type mismatch: want %q, got %q", "application/json; charset=utf-8", got)
	}
	if got := rr.Header().Get("X-Request-Id"); got == "" {
		t.Fatal("expected non-empty X-Request-Id response header")
	}
	if got := rr.Body.String(); !strings.Contains(got, `"code":"internal_error"`) || !strings.Contains(got, `"message":"internal server error"`) || !strings.Contains(got, `"requestId":"`) {
		t.Fatalf("body mismatch: got %q", got)
	}
}

func newInfraRequestMetricsForTest(t *testing.T) (*middleware.HTTPRequestMetrics, *prometheus.Registry) {
	t.Helper()

	registry := prometheus.NewRegistry()
	if err := registry.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})); err != nil {
		t.Fatalf("register process collector: %v", err)
	}
	if err := registry.Register(collectors.NewGoCollector()); err != nil {
		t.Fatalf("register go collector: %v", err)
	}

	requestMetrics, err := middleware.NewHTTPRequestMetrics(registry)
	if err != nil {
		t.Fatalf("create request metrics: %v", err)
	}

	return requestMetrics, registry
}

type panicGatherer struct{}

func (panicGatherer) Gather() ([]*dto.MetricFamily, error) {
	panic("metrics gather panic")
}

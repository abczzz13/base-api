package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewInfraHandlerRoutesMetricsThroughPromHTTP(t *testing.T) {
	handler, err := newInfraHandler(Config{Environment: "test"})
	if err != nil {
		t.Fatalf("newInfraHandler returned error: %v", err)
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
	handler, err := newInfraHandler(Config{Environment: "test"})
	if err != nil {
		t.Fatalf("newInfraHandler returned error: %v", err)
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

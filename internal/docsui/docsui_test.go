package docsui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegisterServesDocumentationEndpoints(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux)

	tests := []struct {
		name                    string
		method                  string
		path                    string
		wantStatus              int
		wantHeaders             map[string]string
		wantContentTypeContains []string
		wantBodyContains        []string
		wantBodyNotEmpty        bool
	}{
		{
			name:       "infra spec is available as yaml",
			method:     http.MethodGet,
			path:       infraOpenAPISpecPath,
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Cache-Control": openAPISpecCacheTTL,
			},
			wantContentTypeContains: []string{"application/yaml"},
			wantBodyContains: []string{
				"openapi: 3.0.3",
				"title: Base API Infra",
			},
		},
		{
			name:       "public spec is available as yaml",
			method:     http.MethodGet,
			path:       publicOpenAPISpecPath,
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Cache-Control": openAPISpecCacheTTL,
			},
			wantContentTypeContains: []string{"application/yaml"},
			wantBodyContains: []string{
				"openapi: 3.0.3",
				"title: Base API",
			},
		},
		{
			name:       "infra spec responds to HEAD",
			method:     http.MethodHead,
			path:       infraOpenAPISpecPath,
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Cache-Control": openAPISpecCacheTTL,
			},
			wantContentTypeContains: []string{"application/yaml"},
		},
		{
			name:       "public spec responds to HEAD",
			method:     http.MethodHead,
			path:       publicOpenAPISpecPath,
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Cache-Control": openAPISpecCacheTTL,
			},
			wantContentTypeContains: []string{"application/yaml"},
		},
		{
			name:       "swagger endpoint renders docs ui",
			method:     http.MethodGet,
			path:       swaggerPath,
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Cache-Control":           swaggerPageCacheTTL,
				"Content-Security-Policy": swaggerUIContentSecurityPolicy,
			},
			wantContentTypeContains: []string{"text/html"},
			wantBodyContains: []string{
				"SwaggerUIBundle",
				"/swagger-ui/swagger-ui.css",
				"/swagger-ui/swagger-ui-bundle.js",
				"/swagger-ui/swagger-ui-standalone-preset.js",
				"./openapi/infra.yaml",
				"./openapi/public.yaml",
				"\"urls.primaryName\": \"Public API\"",
			},
			wantBodyNotEmpty: true,
		},
		{
			name:       "swagger css asset is served locally",
			method:     http.MethodGet,
			path:       swaggerUICSSPath,
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Cache-Control": swaggerAssetCacheTTL,
			},
			wantContentTypeContains: []string{"text/css"},
			wantBodyNotEmpty:        true,
		},
		{
			name:       "swagger bundle asset is served locally",
			method:     http.MethodGet,
			path:       swaggerUIBundleJSPath,
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Cache-Control": swaggerAssetCacheTTL,
			},
			wantContentTypeContains: []string{"text/javascript"},
			wantBodyNotEmpty:        true,
		},
		{
			name:       "swagger standalone preset asset is served locally",
			method:     http.MethodGet,
			path:       swaggerUIPresetJSPath,
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Cache-Control": swaggerAssetCacheTTL,
			},
			wantContentTypeContains: []string{"text/javascript"},
			wantBodyNotEmpty:        true,
		},
		{
			name:       "docs endpoint redirects to swagger",
			method:     http.MethodGet,
			path:       docsPath,
			wantStatus: http.StatusTemporaryRedirect,
			wantHeaders: map[string]string{
				"Location": swaggerPath,
			},
		},
		{
			name:       "docs trailing slash endpoint redirects to swagger",
			method:     http.MethodGet,
			path:       docsSlashPath,
			wantStatus: http.StatusTemporaryRedirect,
			wantHeaders: map[string]string{
				"Location": swaggerPath,
			},
		},
		{
			name:       "docs subpaths are not redirected",
			method:     http.MethodGet,
			path:       docsSlashPath + "nested",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)

			mux.ServeHTTP(rr, req)

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
			if tt.wantBodyNotEmpty && len(body) == 0 {
				t.Fatalf("body should not be empty")
			}
		})
	}
}

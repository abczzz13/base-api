package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abczzz13/base-api/internal/infraoas"
	"github.com/abczzz13/base-api/internal/oas"
	"github.com/google/go-cmp/cmp"
)

func TestGeneratedRoutersBehavior(t *testing.T) {
	publicHandler, err := oas.NewServer(newBaseService(Config{Environment: "test"}))
	if err != nil {
		t.Fatalf("create public server: %v", err)
	}

	infraHandler, err := infraoas.NewServer(newInfraService(Config{Environment: "test"}))
	if err != nil {
		t.Fatalf("create infra server: %v", err)
	}

	tests := []struct {
		name        string
		handler     http.Handler
		method      string
		path        string
		wantStatus  int
		wantHeaders map[string]string
		wantJSON    map[string]any
	}{
		{
			name:       "public health endpoint returns safe payload",
			handler:    publicHandler,
			method:     http.MethodGet,
			path:       "/healthz",
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Content-Type": "application/json; charset=utf-8",
			},
			wantJSON: map[string]any{"status": "OK"},
		},
		{
			name:       "public metrics route is not exposed",
			handler:    publicHandler,
			method:     http.MethodGet,
			path:       "/metrics",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "infra metrics route surfaces middleware wiring issue",
			handler:    infraHandler,
			method:     http.MethodGet,
			path:       "/metrics",
			wantStatus: http.StatusInternalServerError,
			wantHeaders: map[string]string{
				"Content-Type": "application/json; charset=utf-8",
			},
			wantJSON: map[string]any{
				"code":    "metrics_route_misconfigured",
				"message": "metrics endpoint should be handled by infra HTTP middleware",
			},
		},
		{
			name:       "infra metrics route rejects unsupported method",
			handler:    infraHandler,
			method:     http.MethodPost,
			path:       "/metrics",
			wantStatus: http.StatusMethodNotAllowed,
			wantHeaders: map[string]string{
				"Allow": "GET",
			},
		},
		{
			name:       "infra options exposes allowed methods",
			handler:    infraHandler,
			method:     http.MethodOptions,
			path:       "/metrics",
			wantStatus: http.StatusNoContent,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Methods": "GET",
				"Access-Control-Allow-Headers": "Content-Type",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)

			tt.handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("status mismatch: want %d, got %d", tt.wantStatus, rr.Code)
			}

			for headerName, wantValue := range tt.wantHeaders {
				gotValue := rr.Header().Get(headerName)
				if diff := cmp.Diff(wantValue, gotValue); diff != "" {
					t.Fatalf("header %q mismatch (-want +got):\n%s", headerName, diff)
				}
			}

			if tt.wantJSON != nil {
				var gotJSON map[string]any
				if err := json.Unmarshal(rr.Body.Bytes(), &gotJSON); err != nil {
					t.Fatalf("decode json body: %v", err)
				}

				if diff := cmp.Diff(tt.wantJSON, gotJSON); diff != "" {
					t.Fatalf("json body mismatch (-want +got):\n%s", diff)
				}
			}

		})
	}
}

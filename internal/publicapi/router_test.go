package publicapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/abczzz13/base-api/internal/clients/weather"
	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/infraapi"

	"github.com/abczzz13/base-api/internal/infraoas"
	"github.com/abczzz13/base-api/internal/publicapi"
	"github.com/abczzz13/base-api/internal/publicoas"
	"github.com/abczzz13/base-api/internal/weatherapi"
	"github.com/abczzz13/base-api/internal/weatheroas"
)

func TestGeneratedRoutersBehavior(t *testing.T) {
	coreHandler, err := publicoas.NewServer(publicapi.NewOASHandler(publicapi.NewService()))
	if err != nil {
		t.Fatalf("create public server: %v", err)
	}

	weatherHandler, err := weatheroas.NewServer(weatherapi.NewOASHandler(weatherapi.NewService(weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
		return weather.CurrentWeather{
			Provider:     "open-meteo",
			Location:     "Amsterdam",
			Condition:    "Cloudy",
			TemperatureC: 12.5,
			ObservedAt:   time.Date(2026, time.March, 7, 12, 0, 0, 0, time.UTC),
		}, nil
	}))))
	if err != nil {
		t.Fatalf("create weather server: %v", err)
	}

	publicHandler := http.NewServeMux()
	publicHandler.Handle("/weather/", weatherHandler)
	publicHandler.Handle("/", coreHandler)

	infraHandler, err := infraoas.NewServer(infraapi.NewOASHandler(infraapi.NewService(config.Config{Environment: "test"})))
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
			name:       "public weather endpoint returns current conditions",
			handler:    publicHandler,
			method:     http.MethodGet,
			path:       "/weather/current?location=Amsterdam",
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Content-Type": "application/json; charset=utf-8",
			},
			wantJSON: map[string]any{
				"provider":     "open-meteo",
				"location":     "Amsterdam",
				"condition":    "Cloudy",
				"temperatureC": 12.5,
				"observedAt":   "2026-03-07T12:00:00Z",
			},
		},
		{
			name:       "infra liveness endpoint returns safe payload",
			handler:    infraHandler,
			method:     http.MethodGet,
			path:       "/livez",
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Content-Type": "application/json; charset=utf-8",
			},
			wantJSON: map[string]any{
				"status": "OK",
			},
		},
		{
			name:       "infra metrics route is not exposed by generated router",
			handler:    infraHandler,
			method:     http.MethodGet,
			path:       "/metrics",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "infra swagger route is not exposed by generated router",
			handler:    infraHandler,
			method:     http.MethodGet,
			path:       "/swagger",
			wantStatus: http.StatusNotFound,
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

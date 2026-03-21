package infraapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/abczzz13/base-api/internal/clients/weather"
	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/infraapi"
	"github.com/abczzz13/base-api/internal/middleware"
	"github.com/abczzz13/base-api/internal/publicapi"
	"github.com/abczzz13/base-api/internal/requestaudit"
)

func TestRequestMetricsUseLowCardinalityRouteLabels(t *testing.T) {
	registry := prometheus.NewRegistry()
	requestMetrics, err := middleware.NewHTTPRequestMetrics(registry)
	if err != nil {
		t.Fatalf("create request metrics: %v", err)
	}

	publicHandler, err := publicapi.NewHandler(config.Config{Environment: "test"}, publicapi.Dependencies{
		RequestMetrics:         requestMetrics,
		RequestAuditRepository: requestaudit.NopRepository(),
		WeatherClient: weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
			return weather.CurrentWeather{}, nil
		}),
	})
	if err != nil {
		t.Fatalf("new public handler returned error: %v", err)
	}

	infraHandler, err := infraapi.NewHandler(config.Config{Environment: "test"}, infraapi.Dependencies{
		RequestMetrics:  requestMetrics,
		MetricsGatherer: registry,
	})
	if err != nil {
		t.Fatalf("new infra handler returned error: %v", err)
	}

	tests := []struct {
		name       string
		handler    http.Handler
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "public operation uses operation name route label",
			handler:    publicHandler,
			method:     http.MethodGet,
			path:       "/healthz",
			wantStatus: http.StatusOK,
		},
		{
			name:       "public unmatched path is bucketed",
			handler:    publicHandler,
			method:     http.MethodGet,
			path:       "/missing",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "infra operation uses operation name route label",
			handler:    infraHandler,
			method:     http.MethodGet,
			path:       "/livez",
			wantStatus: http.StatusOK,
		},
		{
			name:       "infra docs asset is bucketed",
			handler:    infraHandler,
			method:     http.MethodGet,
			path:       "/swagger-ui/swagger-ui.css",
			wantStatus: http.StatusOK,
		},
		{
			name:       "infra metrics endpoint uses dedicated route label",
			handler:    infraHandler,
			method:     http.MethodGet,
			path:       "/metrics",
			wantStatus: http.StatusOK,
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
		})
	}

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather prometheus metrics: %v", err)
	}

	requestsFamily, ok := metricFamilyByName(families, "base_api_http_requests_total")
	if !ok {
		t.Fatal("base_api_http_requests_total metric family not found")
	}

	for _, labels := range []map[string]string{
		{
			"server":      "public",
			"method":      http.MethodGet,
			"route":       "GetHealthz",
			"status_code": "200",
		},
		{
			"server":      "public",
			"method":      http.MethodGet,
			"route":       "unmatched",
			"status_code": "404",
		},
		{
			"server":      "infra",
			"method":      http.MethodGet,
			"route":       "GetLivez",
			"status_code": "200",
		},
		{
			"server":      "infra",
			"method":      http.MethodGet,
			"route":       "swagger_ui_asset",
			"status_code": "200",
		},
		{
			"server":      "infra",
			"method":      http.MethodGet,
			"route":       "metrics",
			"status_code": "200",
		},
	} {
		if !metricFamilyHasLabels(requestsFamily, labels) {
			t.Fatalf("base_api_http_requests_total missing labels: %#v", labels)
		}
	}

	expectedRoutes := map[string]struct{}{
		"GetHealthz":       {},
		"unmatched":        {},
		"GetLivez":         {},
		"metrics":          {},
		"swagger_ui_asset": {},
	}

	for _, metric := range requestsFamily.GetMetric() {
		route, ok := metricLabelValue(metric, "route")
		if !ok {
			t.Fatalf("route label missing from request metric: %#v", metric)
		}
		if strings.Contains(route, "/") {
			t.Fatalf("route label should be low-cardinality and not contain raw paths: %q", route)
		}
		if _, expected := expectedRoutes[route]; !expected {
			t.Fatalf("unexpected route label in request metrics: %q", route)
		}
	}
}

func metricFamilyByName(families []*dto.MetricFamily, name string) (*dto.MetricFamily, bool) {
	for _, family := range families {
		if family.GetName() == name {
			return family, true
		}
	}

	return nil, false
}

func metricFamilyHasLabels(family *dto.MetricFamily, labels map[string]string) bool {
	for _, metric := range family.GetMetric() {
		if metricHasLabels(metric, labels) {
			return true
		}
	}

	return false
}

func metricHasLabels(metric *dto.Metric, labels map[string]string) bool {
	for labelName, wantValue := range labels {
		if gotValue, ok := metricLabelValue(metric, labelName); !ok || gotValue != wantValue {
			return false
		}
	}

	return true
}

func metricLabelValue(metric *dto.Metric, labelName string) (string, bool) {
	for _, label := range metric.GetLabel() {
		if label.GetName() == labelName {
			return label.GetValue(), true
		}
	}

	return "", false
}

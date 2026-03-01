package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"
)

func TestRequestMetricsUseLowCardinalityRouteLabels(t *testing.T) {
	runtimeDeps, err := newRuntimeDependencies()
	if err != nil {
		t.Fatalf("newRuntimeDependencies returned error: %v", err)
	}

	publicHandler, err := newPublicHandler(Config{Environment: "test"}, runtimeDeps)
	if err != nil {
		t.Fatalf("newPublicHandler returned error: %v", err)
	}

	infraHandler, err := newInfraHandler(Config{Environment: "test"}, runtimeDeps)
	if err != nil {
		t.Fatalf("newInfraHandler returned error: %v", err)
	}

	tests := []struct {
		name       string
		handler    http.Handler
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "public operation uses operation id route label",
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
			name:       "infra operation uses operation id route label",
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
			name:       "infra metrics endpoint bypasses request metrics middleware",
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

	families, err := runtimeDeps.metricsGatherer.Gather()
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
			"route":       "getHealthz",
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
			"route":       "getLivez",
			"status_code": "200",
		},
		{
			"server":      "infra",
			"method":      http.MethodGet,
			"route":       "swagger_ui_asset",
			"status_code": "200",
		},
	} {
		if !metricFamilyHasLabels(requestsFamily, labels) {
			t.Fatalf("base_api_http_requests_total missing labels: %#v", labels)
		}
	}

	if metricFamilyHasLabels(requestsFamily, map[string]string{
		"server":      "infra",
		"method":      http.MethodGet,
		"route":       "metrics",
		"status_code": "200",
	}) {
		t.Fatal("expected /metrics requests to bypass request metrics middleware")
	}

	expectedRoutes := map[string]struct{}{
		"getHealthz":       {},
		"unmatched":        {},
		"getLivez":         {},
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

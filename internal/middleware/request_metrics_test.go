package middleware

import (
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

func TestRequestMetricsRecordsRequestAndDuration(t *testing.T) {
	requestMetrics, gatherer := newTestHTTPRequestMetrics(t)

	server := "middleware_request_metrics_record"
	route := "createResource"
	method := http.MethodPost
	statusCode := http.StatusCreated
	statusLabel := strconv.Itoa(statusCode)

	beforeCount := testutil.ToFloat64(requestMetrics.httpRequestsTotal.WithLabelValues(server, method, route, statusLabel))
	beforeDurationSamples := requestDurationSampleCount(t, gatherer, map[string]string{
		"server":      server,
		"method":      method,
		"route":       route,
		"status_code": statusLabel,
	})

	handler := RequestMetrics(requestMetrics, RequestMetricsConfig{
		Server: server,
		RouteLabel: func(*http.Request) string {
			return route
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
	}))

	req := httptest.NewRequest(method, "/resources", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != statusCode {
		t.Fatalf("status mismatch: want %d, got %d", statusCode, rr.Code)
	}

	afterCount := testutil.ToFloat64(requestMetrics.httpRequestsTotal.WithLabelValues(server, method, route, statusLabel))
	if got := afterCount - beforeCount; got != 1 {
		t.Fatalf("request counter delta mismatch: want 1, got %v", got)
	}

	afterDurationSamples := requestDurationSampleCount(t, gatherer, map[string]string{
		"server":      server,
		"method":      method,
		"route":       route,
		"status_code": statusLabel,
	})
	if got := afterDurationSamples - beforeDurationSamples; got != 1 {
		t.Fatalf("request duration sample count delta mismatch: want 1, got %d", got)
	}

	if got := testutil.ToFloat64(requestMetrics.httpInFlightRequests.WithLabelValues(server, method, route)); got != 0 {
		t.Fatalf("in-flight gauge mismatch: want 0 after completion, got %v", got)
	}
}

func TestRequestMetricsRecordsRequestAndResponseSizes(t *testing.T) {
	requestMetrics, gatherer := newTestHTTPRequestMetrics(t)

	server := "middleware_request_metrics_sizes"
	route := "uploadAsset"
	method := http.MethodPost
	statusCode := http.StatusCreated
	statusLabel := strconv.Itoa(statusCode)
	requestPayload := "payload-body"
	responsePayload := "created"

	beforeRequestCount, beforeRequestSum := histogramCountAndSum(t, gatherer, requestMetricsFamilyName("request_size_bytes"), map[string]string{
		"server":      server,
		"method":      method,
		"route":       route,
		"status_code": statusLabel,
	})
	beforeResponseCount, beforeResponseSum := histogramCountAndSum(t, gatherer, requestMetricsFamilyName("response_size_bytes"), map[string]string{
		"server":      server,
		"method":      method,
		"route":       route,
		"status_code": statusLabel,
	})

	handler := RequestMetrics(requestMetrics, RequestMetricsConfig{
		Server: server,
		RouteLabel: func(*http.Request) string {
			return route
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(responsePayload))
	}))

	req := httptest.NewRequest(method, "/upload", strings.NewReader(requestPayload))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != statusCode {
		t.Fatalf("status mismatch: want %d, got %d", statusCode, rr.Code)
	}

	afterRequestCount, afterRequestSum := histogramCountAndSum(t, gatherer, requestMetricsFamilyName("request_size_bytes"), map[string]string{
		"server":      server,
		"method":      method,
		"route":       route,
		"status_code": statusLabel,
	})
	if got := afterRequestCount - beforeRequestCount; got != 1 {
		t.Fatalf("request size sample count delta mismatch: want 1, got %d", got)
	}
	if got := afterRequestSum - beforeRequestSum; !floatAlmostEqual(got, float64(len(requestPayload))) {
		t.Fatalf("request size sum delta mismatch: want %d, got %f", len(requestPayload), got)
	}

	afterResponseCount, afterResponseSum := histogramCountAndSum(t, gatherer, requestMetricsFamilyName("response_size_bytes"), map[string]string{
		"server":      server,
		"method":      method,
		"route":       route,
		"status_code": statusLabel,
	})
	if got := afterResponseCount - beforeResponseCount; got != 1 {
		t.Fatalf("response size sample count delta mismatch: want 1, got %d", got)
	}
	if got := afterResponseSum - beforeResponseSum; !floatAlmostEqual(got, float64(len(responsePayload))) {
		t.Fatalf("response size sum delta mismatch: want %d, got %f", len(responsePayload), got)
	}
}

func TestRequestMetricsRecordsUnknownRequestBodySizesFromObservedReads(t *testing.T) {
	requestMetrics, gatherer := newTestHTTPRequestMetrics(t)

	server := "middleware_request_metrics_unknown_request_size"
	route := "streamUpload"
	method := http.MethodPost
	statusCode := http.StatusAccepted
	statusLabel := strconv.Itoa(statusCode)
	requestPayload := "chunked-request-body"

	beforeCount, beforeSum := histogramCountAndSum(t, gatherer, requestMetricsFamilyName("request_size_bytes"), map[string]string{
		"server":      server,
		"method":      method,
		"route":       route,
		"status_code": statusLabel,
	})

	handler := RequestMetrics(requestMetrics, RequestMetricsConfig{
		Server: server,
		RouteLabel: func(*http.Request) string {
			return route
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Fatalf("read request body: %v", err)
		}

		w.WriteHeader(statusCode)
	}))

	req := httptest.NewRequest(method, "/upload", io.NopCloser(strings.NewReader(requestPayload)))
	if req.ContentLength != -1 {
		t.Fatalf("expected unknown content length (-1), got %d", req.ContentLength)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != statusCode {
		t.Fatalf("status mismatch: want %d, got %d", statusCode, rr.Code)
	}

	afterCount, afterSum := histogramCountAndSum(t, gatherer, requestMetricsFamilyName("request_size_bytes"), map[string]string{
		"server":      server,
		"method":      method,
		"route":       route,
		"status_code": statusLabel,
	})
	if got := afterCount - beforeCount; got != 1 {
		t.Fatalf("request size sample count delta mismatch: want 1, got %d", got)
	}
	if got := afterSum - beforeSum; !floatAlmostEqual(got, float64(len(requestPayload))) {
		t.Fatalf("request size sum delta mismatch: want %d, got %f", len(requestPayload), got)
	}
}

func TestRequestMetricsRecordsResponseSizeWithRequestLogger(t *testing.T) {
	requestMetrics, gatherer := newTestHTTPRequestMetrics(t)

	server := "middleware_request_metrics_with_logger"
	route := "streamResponse"
	method := http.MethodGet
	statusCode := http.StatusOK
	statusLabel := strconv.Itoa(statusCode)
	responsePayload := "streamed-response"

	beforeCount, beforeSum := histogramCountAndSum(t, gatherer, requestMetricsFamilyName("response_size_bytes"), map[string]string{
		"server":      server,
		"method":      method,
		"route":       route,
		"status_code": statusLabel,
	})

	handler := RequestMetrics(requestMetrics, RequestMetricsConfig{
		Server: server,
		RouteLabel: func(*http.Request) string {
			return route
		},
	})(RequestLogger()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readerFrom, ok := w.(io.ReaderFrom)
		if !ok {
			t.Fatalf("expected response writer to implement io.ReaderFrom")
		}

		if _, err := readerFrom.ReadFrom(strings.NewReader(responsePayload)); err != nil {
			t.Fatalf("read response payload: %v", err)
		}
	})))

	req := httptest.NewRequest(method, "/stream", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != statusCode {
		t.Fatalf("status mismatch: want %d, got %d", statusCode, rr.Code)
	}
	if got := rr.Body.String(); got != responsePayload {
		t.Fatalf("response body mismatch: want %q, got %q", responsePayload, got)
	}

	afterCount, afterSum := histogramCountAndSum(t, gatherer, requestMetricsFamilyName("response_size_bytes"), map[string]string{
		"server":      server,
		"method":      method,
		"route":       route,
		"status_code": statusLabel,
	})
	if got := afterCount - beforeCount; got != 1 {
		t.Fatalf("response size sample count delta mismatch: want 1, got %d", got)
	}
	if got := afterSum - beforeSum; !floatAlmostEqual(got, float64(len(responsePayload))) {
		t.Fatalf("response size sum delta mismatch: want %d, got %f", len(responsePayload), got)
	}
}

func TestRequestMetricsBoundsMethodLabelCardinality(t *testing.T) {
	requestMetrics, _ := newTestHTTPRequestMetrics(t)

	server := "middleware_request_metrics_custom_method"
	route := "createTeapot"
	customMethod := "BREW"
	unknownMethod := requestMetricsMethodUnknown
	statusCode := http.StatusNoContent
	statusLabel := strconv.Itoa(statusCode)

	beforeUnknown := testutil.ToFloat64(requestMetrics.httpRequestsTotal.WithLabelValues(server, unknownMethod, route, statusLabel))
	beforeCustom := testutil.ToFloat64(requestMetrics.httpRequestsTotal.WithLabelValues(server, customMethod, route, statusLabel))

	handler := RequestMetrics(requestMetrics, RequestMetricsConfig{
		Server: server,
		RouteLabel: func(*http.Request) string {
			return route
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
	}))

	req := httptest.NewRequest(customMethod, "/teapot", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != statusCode {
		t.Fatalf("status mismatch: want %d, got %d", statusCode, rr.Code)
	}

	afterUnknown := testutil.ToFloat64(requestMetrics.httpRequestsTotal.WithLabelValues(server, unknownMethod, route, statusLabel))
	if got := afterUnknown - beforeUnknown; got != 1 {
		t.Fatalf("UNKNOWN method counter delta mismatch: want 1, got %v", got)
	}

	afterCustom := testutil.ToFloat64(requestMetrics.httpRequestsTotal.WithLabelValues(server, customMethod, route, statusLabel))
	if got := afterCustom - beforeCustom; got != 0 {
		t.Fatalf("custom method counter delta mismatch: want 0, got %v", got)
	}
}

func TestRequestMetricsMethodValue(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "empty value maps to unknown",
			value: "",
			want:  requestMetricsMethodUnknown,
		},
		{
			name:  "whitespace value maps to unknown",
			value: " \t\n ",
			want:  requestMetricsMethodUnknown,
		},
		{
			name:  "standard method is normalized to uppercase",
			value: "get",
			want:  http.MethodGet,
		},
		{
			name:  "trimmed standard method remains valid",
			value: "  patch  ",
			want:  http.MethodPatch,
		},
		{
			name:  "non-standard method maps to unknown",
			value: "BREW",
			want:  requestMetricsMethodUnknown,
		},
		{
			name:  "connect method is allowed",
			value: "connect",
			want:  http.MethodConnect,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requestMetricsMethodValue(tt.value); got != tt.want {
				t.Fatalf("request method label mismatch: want %q, got %q", tt.want, got)
			}
		})
	}
}

func TestRequestMetricsTracksInflightRequests(t *testing.T) {
	requestMetrics, _ := newTestHTTPRequestMetrics(t)

	server := "middleware_request_metrics_inflight"
	route := "slowOperation"
	method := http.MethodGet

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	handler := RequestMetrics(requestMetrics, RequestMetricsConfig{
		Server: server,
		RouteLabel: func(*http.Request) string {
			return route
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))

	go func() {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/slow", nil)
		handler.ServeHTTP(rr, req)
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for request to start")
	}

	if got := testutil.ToFloat64(requestMetrics.httpInFlightRequests.WithLabelValues(server, method, route)); got != 1 {
		t.Fatalf("in-flight gauge mismatch while request is active: want 1, got %v", got)
	}

	close(release)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for request completion")
	}

	if got := testutil.ToFloat64(requestMetrics.httpInFlightRequests.WithLabelValues(server, method, route)); got != 0 {
		t.Fatalf("in-flight gauge mismatch after request completion: want 0, got %v", got)
	}
}

func TestRequestMetricsUsesFallbackLabels(t *testing.T) {
	requestMetrics, _ := newTestHTTPRequestMetrics(t)

	server := requestMetricsServerUnknown
	route := RequestMetricsRouteUnmatched
	method := http.MethodGet
	statusLabel := strconv.Itoa(http.StatusOK)

	beforeCount := testutil.ToFloat64(requestMetrics.httpRequestsTotal.WithLabelValues(server, method, route, statusLabel))

	handler := RequestMetrics(requestMetrics, RequestMetricsConfig{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusOK, rr.Code)
	}

	afterCount := testutil.ToFloat64(requestMetrics.httpRequestsTotal.WithLabelValues(server, method, route, statusLabel))
	if got := afterCount - beforeCount; got != 1 {
		t.Fatalf("request counter delta mismatch: want 1, got %v", got)
	}
}

func TestRequestMetricsCapturesRecoveredPanics(t *testing.T) {
	requestMetrics, gatherer := newTestHTTPRequestMetrics(t)

	server := "middleware_request_metrics_panic"
	route := "panicRoute"
	method := http.MethodGet
	statusLabel := strconv.Itoa(http.StatusInternalServerError)

	beforeCount := testutil.ToFloat64(requestMetrics.httpRequestsTotal.WithLabelValues(server, method, route, statusLabel))
	beforePanics := testutil.ToFloat64(requestMetrics.httpRecoveredPanicsTotal.WithLabelValues(server, method, route))
	beforeDurationSamples := requestDurationSampleCount(t, gatherer, map[string]string{
		"server":      server,
		"method":      method,
		"route":       route,
		"status_code": statusLabel,
	})

	handler := RequestMetrics(requestMetrics, RequestMetricsConfig{
		Server: server,
		RouteLabel: func(*http.Request) string {
			return route
		},
	})(Recovery()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusInternalServerError, rr.Code)
	}

	afterCount := testutil.ToFloat64(requestMetrics.httpRequestsTotal.WithLabelValues(server, method, route, statusLabel))
	if got := afterCount - beforeCount; got != 1 {
		t.Fatalf("request counter delta mismatch: want 1, got %v", got)
	}

	afterPanics := testutil.ToFloat64(requestMetrics.httpRecoveredPanicsTotal.WithLabelValues(server, method, route))
	if got := afterPanics - beforePanics; got != 1 {
		t.Fatalf("panic counter delta mismatch: want 1, got %v", got)
	}

	afterDurationSamples := requestDurationSampleCount(t, gatherer, map[string]string{
		"server":      server,
		"method":      method,
		"route":       route,
		"status_code": statusLabel,
	})
	if got := afterDurationSamples - beforeDurationSamples; got != 1 {
		t.Fatalf("request duration sample count delta mismatch: want 1, got %d", got)
	}
}

func newTestHTTPRequestMetrics(t *testing.T) (*HTTPRequestMetrics, prometheus.Gatherer) {
	t.Helper()

	registry := prometheus.NewRegistry()
	requestMetrics, err := NewHTTPRequestMetrics(registry)
	if err != nil {
		t.Fatalf("new HTTP request metrics: %v", err)
	}

	return requestMetrics, registry
}

func requestDurationSampleCount(t *testing.T, gatherer prometheus.Gatherer, labels map[string]string) uint64 {
	t.Helper()

	count, _ := histogramCountAndSum(t, gatherer, requestMetricsFamilyName("request_duration_seconds"), labels)
	return count
}

func histogramCountAndSum(t *testing.T, gatherer prometheus.Gatherer, familyName string, labels map[string]string) (uint64, float64) {
	t.Helper()

	metric, ok := findMetricByLabels(t, gatherer, familyName, labels)
	if !ok {
		return 0, 0
	}

	histogram := metric.GetHistogram()
	if histogram == nil {
		t.Fatalf("metric %q does not contain histogram data", familyName)
	}

	return histogram.GetSampleCount(), histogram.GetSampleSum()
}

func findMetricByLabels(t *testing.T, gatherer prometheus.Gatherer, familyName string, labels map[string]string) (*dto.Metric, bool) {
	t.Helper()

	metrics, err := gatherer.Gather()
	if err != nil {
		t.Fatalf("gather prometheus metrics: %v", err)
	}

	for _, family := range metrics {
		if family.GetName() != familyName {
			continue
		}

		for _, metric := range family.GetMetric() {
			if metricHasLabels(metric, labels) {
				return metric, true
			}
		}
	}

	return nil, false
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

func requestMetricsFamilyName(suffix string) string {
	return requestMetricsNamespace + "_" + requestMetricsSubsystem + "_" + suffix
}

func floatAlmostEqual(got, want float64) bool {
	return math.Abs(got-want) < 1e-9
}

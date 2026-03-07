package middleware

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	requestMetricsNamespace = "base_api"
	requestMetricsSubsystem = "http"

	RequestMetricsRouteUnmatched = "unmatched"
	RequestRejectionReasonCSRF   = "csrf"

	requestMetricsServerUnknown = "unknown"
	requestMetricsMethodUnknown = "UNKNOWN"
	requestMetricsReasonUnknown = "unknown"
)

var requestSizeBuckets = []float64{
	64,
	256,
	1024,
	4 * 1024,
	16 * 1024,
	64 * 1024,
	256 * 1024,
	1024 * 1024,
	4 * 1024 * 1024,
	16 * 1024 * 1024,
}

type RequestMetricsConfig struct {
	Server     string
	RouteLabel func(*http.Request) string
}

type HTTPRequestMetrics struct {
	httpRequestsTotal          *prometheus.CounterVec
	httpRequestDurationSeconds *prometheus.HistogramVec
	httpRequestSizeBytes       *prometheus.HistogramVec
	httpResponseSizeBytes      *prometheus.HistogramVec
	httpInFlightRequests       *prometheus.GaugeVec
	httpRecoveredPanicsTotal   *prometheus.CounterVec
	httpRejectedRequestsTotal  *prometheus.CounterVec
	httpRateLimitErrorsTotal   *prometheus.CounterVec
	httpCORSPolicyDenialsTotal *prometheus.CounterVec
}

func NewHTTPRequestMetrics(reg prometheus.Registerer) (*HTTPRequestMetrics, error) {
	if reg == nil {
		return nil, errors.New("prometheus registerer is required")
	}

	httpRequestsTotal, err := registerCounterVec(reg, prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: requestMetricsNamespace,
			Subsystem: requestMetricsSubsystem,
			Name:      "requests_total",
			Help:      "Total number of HTTP requests handled by the service.",
		},
		[]string{"server", "method", "route", "status_code"},
	))
	if err != nil {
		return nil, fmt.Errorf("register requests_total metric: %w", err)
	}

	httpRequestDurationSeconds, err := registerHistogramVec(reg, prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: requestMetricsNamespace,
			Subsystem: requestMetricsSubsystem,
			Name:      "request_duration_seconds",
			Help:      "Histogram of HTTP request durations in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"server", "method", "route", "status_code"},
	))
	if err != nil {
		return nil, fmt.Errorf("register request_duration_seconds metric: %w", err)
	}

	httpInFlightRequests, err := registerGaugeVec(reg, prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: requestMetricsNamespace,
			Subsystem: requestMetricsSubsystem,
			Name:      "in_flight_requests",
			Help:      "Current number of in-flight HTTP requests.",
		},
		[]string{"server", "method", "route"},
	))
	if err != nil {
		return nil, fmt.Errorf("register in_flight_requests metric: %w", err)
	}

	httpRequestSizeBytes, err := registerHistogramVec(reg, prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: requestMetricsNamespace,
			Subsystem: requestMetricsSubsystem,
			Name:      "request_size_bytes",
			Help:      "Histogram of HTTP request body sizes in bytes.",
			Buckets:   requestSizeBuckets,
		},
		[]string{"server", "method", "route", "status_code"},
	))
	if err != nil {
		return nil, fmt.Errorf("register request_size_bytes metric: %w", err)
	}

	httpResponseSizeBytes, err := registerHistogramVec(reg, prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: requestMetricsNamespace,
			Subsystem: requestMetricsSubsystem,
			Name:      "response_size_bytes",
			Help:      "Histogram of HTTP response body sizes in bytes.",
			Buckets:   requestSizeBuckets,
		},
		[]string{"server", "method", "route", "status_code"},
	))
	if err != nil {
		return nil, fmt.Errorf("register response_size_bytes metric: %w", err)
	}

	httpRecoveredPanicsTotal, err := registerCounterVec(reg, prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: requestMetricsNamespace,
			Subsystem: requestMetricsSubsystem,
			Name:      "recovered_panics_total",
			Help:      "Total number of recovered panics while handling HTTP requests.",
		},
		[]string{"server", "method", "route"},
	))
	if err != nil {
		return nil, fmt.Errorf("register recovered_panics_total metric: %w", err)
	}

	httpRejectedRequestsTotal, err := registerCounterVec(reg, prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: requestMetricsNamespace,
			Subsystem: requestMetricsSubsystem,
			Name:      "rejected_requests_total",
			Help:      "Total number of HTTP requests hard-rejected by API safety middleware.",
		},
		[]string{"server", "method", "route", "reason"},
	))
	if err != nil {
		return nil, fmt.Errorf("register rejected_requests_total metric: %w", err)
	}

	httpCORSPolicyDenialsTotal, err := registerCounterVec(reg, prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: requestMetricsNamespace,
			Subsystem: requestMetricsSubsystem,
			Name:      "cors_policy_denials_total",
			Help:      "Total number of requests with an Origin header that fail CORS policy checks.",
		},
		[]string{"server", "method", "route"},
	))
	if err != nil {
		return nil, fmt.Errorf("register cors_policy_denials_total metric: %w", err)
	}

	httpRateLimitErrorsTotal, err := registerCounterVec(reg, prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: requestMetricsNamespace,
			Subsystem: requestMetricsSubsystem,
			Name:      "rate_limit_backend_errors_total",
			Help:      "Total number of Valkey-backed rate limit checks that failed and fell back to configured behavior.",
		},
		[]string{"server", "method", "route"},
	))
	if err != nil {
		return nil, fmt.Errorf("register rate_limit_backend_errors_total metric: %w", err)
	}

	return &HTTPRequestMetrics{
		httpRequestsTotal:          httpRequestsTotal,
		httpRequestDurationSeconds: httpRequestDurationSeconds,
		httpRequestSizeBytes:       httpRequestSizeBytes,
		httpResponseSizeBytes:      httpResponseSizeBytes,
		httpInFlightRequests:       httpInFlightRequests,
		httpRecoveredPanicsTotal:   httpRecoveredPanicsTotal,
		httpRejectedRequestsTotal:  httpRejectedRequestsTotal,
		httpRateLimitErrorsTotal:   httpRateLimitErrorsTotal,
		httpCORSPolicyDenialsTotal: httpCORSPolicyDenialsTotal,
	}, nil
}

func RequestMetrics(metrics *HTTPRequestMetrics, cfg RequestMetricsConfig) func(http.Handler) http.Handler {
	if metrics == nil {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	server := requestMetricsServerLabel(cfg.Server)
	routeLabel := cfg.RouteLabel
	if routeLabel == nil {
		routeLabel = func(*http.Request) string {
			return RequestMetricsRouteUnmatched
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			method := requestMetricsMethodLabel(r)
			route := requestMetricsRouteLabel(routeLabel(r))
			labels := requestMetricsLabels{
				server: server,
				method: method,
				route:  route,
			}

			nextWriter, rw := ensureObservedResponseWriter(w)
			req := requestWithMetricsContext(r, metrics, labels)
			req, requestBodyCounter := observeRequestBody(req)

			inFlight := metrics.httpInFlightRequests.WithLabelValues(server, method, route)
			inFlight.Inc()
			defer inFlight.Dec()

			startedAt := time.Now()
			next.ServeHTTP(nextWriter, req)

			statusCode := strconv.Itoa(rw.statusCode)
			duration := time.Since(startedAt).Seconds()

			metrics.httpRequestsTotal.WithLabelValues(server, method, route, statusCode).Inc()
			metrics.httpRequestDurationSeconds.WithLabelValues(server, method, route, statusCode).Observe(duration)
			metrics.httpRequestSizeBytes.WithLabelValues(server, method, route, statusCode).Observe(requestBodySizeBytes(req, requestBodyCounter))
			metrics.httpResponseSizeBytes.WithLabelValues(server, method, route, statusCode).Observe(float64(rw.bytesWritten))
		})
	}
}

func requestMetricsServerLabel(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return requestMetricsServerUnknown
	}

	return trimmed
}

func requestMetricsMethodLabel(r *http.Request) string {
	if r == nil {
		return requestMetricsMethodUnknown
	}

	return requestMetricsMethodValue(r.Method)
}

func requestMetricsMethodValue(value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "" {
		return requestMetricsMethodUnknown
	}

	switch normalized {
	case http.MethodConnect,
		http.MethodDelete,
		http.MethodGet,
		http.MethodHead,
		http.MethodOptions,
		http.MethodPatch,
		http.MethodPost,
		http.MethodPut,
		http.MethodTrace:
		return normalized
	default:
		return requestMetricsMethodUnknown
	}
}

func requestMetricsRouteLabel(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return RequestMetricsRouteUnmatched
	}

	return trimmed
}

func requestMetricsReasonLabel(reason string) string {
	trimmed := strings.ToLower(strings.TrimSpace(reason))
	if trimmed == "" {
		return requestMetricsReasonUnknown
	}

	return trimmed
}

func requestBodySizeBytes(r *http.Request, counter *countingReadCloser) float64 {
	if r == nil {
		return 0
	}

	if r.ContentLength >= 0 {
		return float64(r.ContentLength)
	}

	if counter != nil {
		return float64(counter.bytesRead)
	}

	return 0
}

func observeRequestBody(r *http.Request) (*http.Request, *countingReadCloser) {
	if r == nil || r.Body == nil || r.ContentLength >= 0 {
		return r, nil
	}

	counter := &countingReadCloser{ReadCloser: r.Body}
	r.Body = counter

	return r, counter
}

type countingReadCloser struct {
	io.ReadCloser
	bytesRead int64
}

func (r *countingReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	r.bytesRead += int64(n)

	return n, err
}

func observeRecoveredPanic(r *http.Request) {
	metricsCtx := requestMetricsContextFromRequest(r)
	if metricsCtx.metrics == nil {
		return
	}

	metricsCtx.metrics.httpRecoveredPanicsTotal.WithLabelValues(metricsCtx.labels.server, metricsCtx.labels.method, metricsCtx.labels.route).Inc()
}

func observeRejectedRequest(r *http.Request, reason string) {
	metricsCtx := requestMetricsContextFromRequest(r)
	if metricsCtx.metrics == nil {
		return
	}

	reasonLabel := requestMetricsReasonLabel(reason)
	metricsCtx.metrics.httpRejectedRequestsTotal.WithLabelValues(metricsCtx.labels.server, metricsCtx.labels.method, metricsCtx.labels.route, reasonLabel).Inc()
}

func observeCORSPolicyDenial(r *http.Request) {
	metricsCtx := requestMetricsContextFromRequest(r)
	if metricsCtx.metrics == nil {
		return
	}

	metricsCtx.metrics.httpCORSPolicyDenialsTotal.WithLabelValues(metricsCtx.labels.server, metricsCtx.labels.method, metricsCtx.labels.route).Inc()
}

func observeRateLimitBackendError(r *http.Request) {
	metricsCtx := requestMetricsContextFromRequest(r)
	if metricsCtx.metrics == nil {
		return
	}

	metricsCtx.metrics.httpRateLimitErrorsTotal.WithLabelValues(metricsCtx.labels.server, metricsCtx.labels.method, metricsCtx.labels.route).Inc()
}

type requestMetricsLabels struct {
	server string
	method string
	route  string
}

type requestMetricsContext struct {
	metrics *HTTPRequestMetrics
	labels  requestMetricsLabels
}

type requestMetricsContextKey struct{}

func requestWithMetricsContext(r *http.Request, metrics *HTTPRequestMetrics, labels requestMetricsLabels) *http.Request {
	if r == nil {
		return r
	}

	ctx := context.WithValue(r.Context(), requestMetricsContextKey{}, requestMetricsContext{metrics: metrics, labels: labels})
	return r.WithContext(ctx)
}

func requestMetricsContextFromRequest(r *http.Request) requestMetricsContext {
	metricsCtx := requestMetricsContext{
		labels: requestMetricsLabels{
			server: requestMetricsServerUnknown,
			method: requestMetricsMethodLabel(r),
			route:  RequestMetricsRouteUnmatched,
		},
	}

	if r == nil {
		return metricsCtx
	}

	if fromContext, ok := r.Context().Value(requestMetricsContextKey{}).(requestMetricsContext); ok {
		metricsCtx.metrics = fromContext.metrics
		metricsCtx.labels.server = requestMetricsServerLabel(fromContext.labels.server)
		metricsCtx.labels.method = requestMetricsMethodValue(fromContext.labels.method)
		metricsCtx.labels.route = requestMetricsRouteLabel(fromContext.labels.route)
	}

	return metricsCtx
}

func registerCounterVec(reg prometheus.Registerer, collector *prometheus.CounterVec) (*prometheus.CounterVec, error) {
	if err := reg.Register(collector); err != nil {
		var alreadyRegisteredErr prometheus.AlreadyRegisteredError
		if !errors.As(err, &alreadyRegisteredErr) {
			return nil, err
		}

		existingCollector, ok := alreadyRegisteredErr.ExistingCollector.(*prometheus.CounterVec)
		if !ok {
			return nil, fmt.Errorf("existing collector has type %T, want *prometheus.CounterVec", alreadyRegisteredErr.ExistingCollector)
		}

		return existingCollector, nil
	}

	return collector, nil
}

func registerGaugeVec(reg prometheus.Registerer, collector *prometheus.GaugeVec) (*prometheus.GaugeVec, error) {
	if err := reg.Register(collector); err != nil {
		var alreadyRegisteredErr prometheus.AlreadyRegisteredError
		if !errors.As(err, &alreadyRegisteredErr) {
			return nil, err
		}

		existingCollector, ok := alreadyRegisteredErr.ExistingCollector.(*prometheus.GaugeVec)
		if !ok {
			return nil, fmt.Errorf("existing collector has type %T, want *prometheus.GaugeVec", alreadyRegisteredErr.ExistingCollector)
		}

		return existingCollector, nil
	}

	return collector, nil
}

func registerHistogramVec(reg prometheus.Registerer, collector *prometheus.HistogramVec) (*prometheus.HistogramVec, error) {
	if err := reg.Register(collector); err != nil {
		var alreadyRegisteredErr prometheus.AlreadyRegisteredError
		if !errors.As(err, &alreadyRegisteredErr) {
			return nil, err
		}

		existingCollector, ok := alreadyRegisteredErr.ExistingCollector.(*prometheus.HistogramVec)
		if !ok {
			return nil, fmt.Errorf("existing collector has type %T, want *prometheus.HistogramVec", alreadyRegisteredErr.ExistingCollector)
		}

		return existingCollector, nil
	}

	return collector, nil
}

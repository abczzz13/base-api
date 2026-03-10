package middleware

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/abczzz13/base-api/internal/httpcapture"
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

	httpRequestsTotal, err := registerCollector(reg, prometheus.NewCounterVec(
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

	httpRequestDurationSeconds, err := registerCollector(reg, prometheus.NewHistogramVec(
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

	httpInFlightRequests, err := registerCollector(reg, prometheus.NewGaugeVec(
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

	httpRequestSizeBytes, err := registerCollector(reg, prometheus.NewHistogramVec(
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

	httpResponseSizeBytes, err := registerCollector(reg, prometheus.NewHistogramVec(
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

	httpRecoveredPanicsTotal, err := registerCollector(reg, prometheus.NewCounterVec(
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

	httpRejectedRequestsTotal, err := registerCollector(reg, prometheus.NewCounterVec(
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

	httpCORSPolicyDenialsTotal, err := registerCollector(reg, prometheus.NewCounterVec(
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

	httpRateLimitErrorsTotal, err := registerCollector(reg, prometheus.NewCounterVec(
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

	server := defaultServerLabel(cfg.Server)
	routeLabel := defaultRouteLabel(cfg.RouteLabel)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			method := requestMetricsMethodLabel(r)
			route := requestMetricsRouteLabel(routeLabel(r))
			labels := requestMetricsLabels{
				server: server,
				method: method,
				route:  route,
			}

			nextWriter, rw := httpcapture.EnsureObservedResponseWriter(w)
			req := requestWithMetricsContext(r, metrics, labels)
			req, requestBodyCounter := observeRequestBody(req)

			inFlight := metrics.httpInFlightRequests.WithLabelValues(server, method, route)
			inFlight.Inc()
			defer inFlight.Dec()

			startedAt := time.Now()
			next.ServeHTTP(nextWriter, req)

			statusCode := strconv.Itoa(rw.StatusCode)
			duration := time.Since(startedAt).Seconds()

			metrics.httpRequestsTotal.WithLabelValues(server, method, route, statusCode).Inc()
			metrics.httpRequestDurationSeconds.WithLabelValues(server, method, route, statusCode).Observe(duration)
			metrics.httpRequestSizeBytes.WithLabelValues(server, method, route, statusCode).Observe(requestBodySizeBytes(req, requestBodyCounter))
			metrics.httpResponseSizeBytes.WithLabelValues(server, method, route, statusCode).Observe(float64(rw.BytesWritten))
		})
	}
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

func registerCollector[T prometheus.Collector](reg prometheus.Registerer, collector T) (T, error) {
	if err := reg.Register(collector); err != nil {
		var alreadyRegisteredErr prometheus.AlreadyRegisteredError
		if !errors.As(err, &alreadyRegisteredErr) {
			var zero T
			return zero, err
		}

		existing, ok := alreadyRegisteredErr.ExistingCollector.(T)
		if !ok {
			var zero T
			return zero, fmt.Errorf("existing collector has type %T, want %T", alreadyRegisteredErr.ExistingCollector, collector)
		}

		return existing, nil
	}

	return collector, nil
}

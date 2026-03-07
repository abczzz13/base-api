package outboundhttp

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "base_api"
	metricsSubsystem = "http_client"

	resultSuccess        = "success"
	resultHTTP4xx        = "http_4xx"
	resultHTTP5xx        = "http_5xx"
	resultTransportError = "transport_error"

	unknownClient    = "unknown"
	unknownOperation = "unknown"
	unknownMethod    = "UNKNOWN"
	unknownStatus    = "0"
)

var sizeBuckets = []float64{
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

// Metrics tracks outbound HTTP request metrics.
type Metrics struct {
	requestsTotal          *prometheus.CounterVec
	requestDurationSeconds *prometheus.HistogramVec
	requestSizeBytes       *prometheus.HistogramVec
	responseSizeBytes      *prometheus.HistogramVec
	inFlightRequests       *prometheus.GaugeVec
}

// NewMetrics registers outbound HTTP metrics collectors.
func NewMetrics(reg prometheus.Registerer) (*Metrics, error) {
	if reg == nil {
		return nil, errors.New("prometheus registerer is required")
	}

	requestsTotal, err := registerCounterVec(reg, prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "requests_total",
			Help:      "Total number of outbound HTTP requests by result.",
		},
		[]string{"client", "operation", "method", "status_code", "result"},
	))
	if err != nil {
		return nil, fmt.Errorf("register http client requests_total metric: %w", err)
	}

	requestDurationSeconds, err := registerHistogramVec(reg, prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "request_duration_seconds",
			Help:      "Histogram of outbound HTTP request durations in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"client", "operation", "method", "status_code", "result"},
	))
	if err != nil {
		return nil, fmt.Errorf("register http client request_duration_seconds metric: %w", err)
	}

	inFlightRequests, err := registerGaugeVec(reg, prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "in_flight_requests",
			Help:      "Current number of outbound HTTP requests in flight.",
		},
		[]string{"client", "operation", "method"},
	))
	if err != nil {
		return nil, fmt.Errorf("register http client in_flight_requests metric: %w", err)
	}

	requestSizeBytes, err := registerHistogramVec(reg, prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "request_size_bytes",
			Help:      "Histogram of outbound HTTP request body sizes in bytes.",
			Buckets:   sizeBuckets,
		},
		[]string{"client", "operation", "method", "status_code", "result"},
	))
	if err != nil {
		return nil, fmt.Errorf("register http client request_size_bytes metric: %w", err)
	}

	responseSizeBytes, err := registerHistogramVec(reg, prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "response_size_bytes",
			Help:      "Histogram of outbound HTTP response body sizes in bytes.",
			Buckets:   sizeBuckets,
		},
		[]string{"client", "operation", "method", "status_code", "result"},
	))
	if err != nil {
		return nil, fmt.Errorf("register http client response_size_bytes metric: %w", err)
	}

	return &Metrics{
		requestsTotal:          requestsTotal,
		requestDurationSeconds: requestDurationSeconds,
		requestSizeBytes:       requestSizeBytes,
		responseSizeBytes:      responseSizeBytes,
		inFlightRequests:       inFlightRequests,
	}, nil
}

type metricLabels struct {
	client    string
	operation string
	method    string
}

func normalizeLabels(client, operation, method string) metricLabels {
	return metricLabels{
		client:    clientValue(client),
		operation: operationValue(operation),
		method:    methodValue(method),
	}
}

func (metrics *Metrics) observeInFlightInc(labels metricLabels) {
	if metrics == nil || metrics.inFlightRequests == nil {
		return
	}

	metrics.inFlightRequests.WithLabelValues(labels.client, labels.operation, labels.method).Inc()
}

func (metrics *Metrics) observeInFlightDec(labels metricLabels) {
	if metrics == nil || metrics.inFlightRequests == nil {
		return
	}

	metrics.inFlightRequests.WithLabelValues(labels.client, labels.operation, labels.method).Dec()
}

func (metrics *Metrics) observeCompleted(labels metricLabels, statusCode int, duration time.Duration, requestSize, responseSize int64, err error) {
	if metrics == nil {
		return
	}

	statusLabel := statusCodeValue(statusCode)
	result := resultValue(statusCode, err)

	if metrics.requestsTotal != nil {
		metrics.requestsTotal.WithLabelValues(labels.client, labels.operation, labels.method, statusLabel, result).Inc()
	}
	if metrics.requestDurationSeconds != nil {
		metrics.requestDurationSeconds.WithLabelValues(labels.client, labels.operation, labels.method, statusLabel, result).Observe(duration.Seconds())
	}
	if metrics.requestSizeBytes != nil {
		metrics.requestSizeBytes.WithLabelValues(labels.client, labels.operation, labels.method, statusLabel, result).Observe(float64(nonNegativeSize(requestSize)))
	}
	if metrics.responseSizeBytes != nil {
		metrics.responseSizeBytes.WithLabelValues(labels.client, labels.operation, labels.method, statusLabel, result).Observe(float64(nonNegativeSize(responseSize)))
	}
}

func clientValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return unknownClient
	}

	return trimmed
}

func operationValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return unknownOperation
	}

	return trimmed
}

func methodValue(value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "" {
		return unknownMethod
	}

	switch normalized {
	case "CONNECT", "DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT", "TRACE":
		return normalized
	default:
		return unknownMethod
	}
}

func statusCodeValue(value int) string {
	if value <= 0 {
		return unknownStatus
	}
	if value > 599 {
		value = 599
	}

	return strconv.Itoa(value)
}

func resultValue(statusCode int, err error) string {
	if err != nil {
		return resultTransportError
	}
	if statusCode >= 500 {
		return resultHTTP5xx
	}
	if statusCode >= 400 {
		return resultHTTP4xx
	}

	return resultSuccess
}

func nonNegativeSize(value int64) int64 {
	if value < 0 {
		return 0
	}

	return value
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

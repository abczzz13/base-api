package requestaudit

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	requestAuditMetricsNamespace = "base_api"
	requestAuditMetricsSubsystem = "request_audit"

	requestAuditServerUnknown = "unknown"

	requestAuditResultEnqueued         = "enqueued"
	requestAuditResultStored           = "stored"
	requestAuditResultDroppedQueueFull = "dropped_queue_full"
	requestAuditResultDroppedShutdown  = "dropped_shutdown"
	requestAuditResultWriteError       = "write_error"

	requestAuditWriteResultSuccess = "success"
	requestAuditWriteResultError   = "error"
	requestAuditWriteResultTimeout = "timeout"
)

// Metrics tracks request-audit queue and persistence outcomes.
type Metrics struct {
	recordsTotal         *prometheus.CounterVec
	queueDepth           prometheus.Gauge
	writeDurationSeconds *prometheus.HistogramVec
}

// NewMetrics registers request-audit metrics collectors.
func NewMetrics(reg prometheus.Registerer) (*Metrics, error) {
	if reg == nil {
		return nil, errors.New("prometheus registerer is required")
	}

	recordsTotal, err := registerRequestAuditCounterVec(reg, prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: requestAuditMetricsNamespace,
			Subsystem: requestAuditMetricsSubsystem,
			Name:      "records_total",
			Help:      "Total number of request-audit records by result.",
		},
		[]string{"server", "result"},
	))
	if err != nil {
		return nil, fmt.Errorf("register request-audit records_total metric: %w", err)
	}

	queueDepth, err := registerRequestAuditGauge(reg, prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: requestAuditMetricsNamespace,
			Subsystem: requestAuditMetricsSubsystem,
			Name:      "queue_depth",
			Help:      "Current number of request-audit records waiting in the async queue.",
		},
	))
	if err != nil {
		return nil, fmt.Errorf("register request-audit queue_depth metric: %w", err)
	}

	writeDurationSeconds, err := registerRequestAuditHistogramVec(reg, prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: requestAuditMetricsNamespace,
			Subsystem: requestAuditMetricsSubsystem,
			Name:      "write_duration_seconds",
			Help:      "Histogram of async request-audit write durations by result.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"server", "result"},
	))
	if err != nil {
		return nil, fmt.Errorf("register request-audit write_duration_seconds metric: %w", err)
	}

	metrics := &Metrics{
		recordsTotal:         recordsTotal,
		queueDepth:           queueDepth,
		writeDurationSeconds: writeDurationSeconds,
	}
	metrics.observeQueueDepth(0)

	return metrics, nil
}

func (metrics *Metrics) observeRecordResult(server, result string) {
	if metrics == nil || metrics.recordsTotal == nil {
		return
	}

	metrics.recordsTotal.WithLabelValues(requestAuditServerLabel(server), strings.TrimSpace(result)).Inc()
}

func (metrics *Metrics) observeWriteDuration(server string, duration time.Duration, result string) {
	if metrics == nil || metrics.writeDurationSeconds == nil {
		return
	}

	metrics.writeDurationSeconds.WithLabelValues(requestAuditServerLabel(server), strings.TrimSpace(result)).Observe(duration.Seconds())
}

func (metrics *Metrics) observeQueueDepth(depth int) {
	if metrics == nil || metrics.queueDepth == nil {
		return
	}

	if depth < 0 {
		depth = 0
	}

	metrics.queueDepth.Set(float64(depth))
}

func requestAuditServerLabel(server string) string {
	normalized := strings.TrimSpace(server)
	if normalized == "" {
		return requestAuditServerUnknown
	}

	return normalized
}

func registerRequestAuditCounterVec(reg prometheus.Registerer, collector *prometheus.CounterVec) (*prometheus.CounterVec, error) {
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

func registerRequestAuditGauge(reg prometheus.Registerer, collector prometheus.Gauge) (prometheus.Gauge, error) {
	if err := reg.Register(collector); err != nil {
		var alreadyRegisteredErr prometheus.AlreadyRegisteredError
		if !errors.As(err, &alreadyRegisteredErr) {
			return nil, err
		}

		existingCollector, ok := alreadyRegisteredErr.ExistingCollector.(prometheus.Gauge)
		if !ok {
			return nil, fmt.Errorf("existing collector has type %T, want prometheus.Gauge", alreadyRegisteredErr.ExistingCollector)
		}

		return existingCollector, nil
	}

	return collector, nil
}

func registerRequestAuditHistogramVec(reg prometheus.Registerer, collector *prometheus.HistogramVec) (*prometheus.HistogramVec, error) {
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

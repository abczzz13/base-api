package outboundaudit

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	outboundAuditMetricsNamespace = "base_api"
	outboundAuditMetricsSubsystem = "http_client_audit"

	outboundAuditClientUnknown = "unknown"

	outboundAuditResultEnqueued         = "enqueued"
	outboundAuditResultStored           = "stored"
	outboundAuditResultDroppedQueueFull = "dropped_queue_full"
	outboundAuditResultDroppedShutdown  = "dropped_shutdown"
	outboundAuditResultWriteError       = "write_error"

	outboundAuditWriteResultSuccess = "success"
	outboundAuditWriteResultError   = "error"
	outboundAuditWriteResultTimeout = "timeout"
)

// Metrics tracks outbound-audit queue and persistence outcomes.
type Metrics struct {
	recordsTotal         *prometheus.CounterVec
	queueDepth           prometheus.Gauge
	writeDurationSeconds *prometheus.HistogramVec
}

// NewMetrics registers outbound-audit metrics collectors.
func NewMetrics(reg prometheus.Registerer) (*Metrics, error) {
	if reg == nil {
		return nil, errors.New("prometheus registerer is required")
	}

	recordsTotal, err := registerCounterVec(reg, prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: outboundAuditMetricsNamespace,
			Subsystem: outboundAuditMetricsSubsystem,
			Name:      "records_total",
			Help:      "Total number of outbound HTTP audit records by result.",
		},
		[]string{"client", "result"},
	))
	if err != nil {
		return nil, fmt.Errorf("register outbound audit records_total metric: %w", err)
	}

	queueDepth, err := registerGauge(reg, prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: outboundAuditMetricsNamespace,
			Subsystem: outboundAuditMetricsSubsystem,
			Name:      "queue_depth",
			Help:      "Current number of outbound HTTP audit records waiting in the async queue.",
		},
	))
	if err != nil {
		return nil, fmt.Errorf("register outbound audit queue_depth metric: %w", err)
	}

	writeDurationSeconds, err := registerHistogramVec(reg, prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: outboundAuditMetricsNamespace,
			Subsystem: outboundAuditMetricsSubsystem,
			Name:      "write_duration_seconds",
			Help:      "Histogram of async outbound HTTP audit write durations by result.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"client", "result"},
	))
	if err != nil {
		return nil, fmt.Errorf("register outbound audit write_duration_seconds metric: %w", err)
	}

	metrics := &Metrics{
		recordsTotal:         recordsTotal,
		queueDepth:           queueDepth,
		writeDurationSeconds: writeDurationSeconds,
	}
	metrics.observeQueueDepth(0)

	return metrics, nil
}

func (metrics *Metrics) observeRecordResult(client, result string) {
	if metrics == nil || metrics.recordsTotal == nil {
		return
	}

	metrics.recordsTotal.WithLabelValues(clientLabel(client), strings.TrimSpace(result)).Inc()
}

func (metrics *Metrics) observeWriteDuration(client string, duration time.Duration, result string) {
	if metrics == nil || metrics.writeDurationSeconds == nil {
		return
	}

	metrics.writeDurationSeconds.WithLabelValues(clientLabel(client), strings.TrimSpace(result)).Observe(duration.Seconds())
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

func clientLabel(client string) string {
	normalized := strings.TrimSpace(client)
	if normalized == "" {
		return outboundAuditClientUnknown
	}

	return normalized
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

func registerGauge(reg prometheus.Registerer, collector prometheus.Gauge) (prometheus.Gauge, error) {
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

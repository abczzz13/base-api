package asyncaudit

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	ResultEnqueued         = "enqueued"
	ResultStored           = "stored"
	ResultDroppedQueueFull = "dropped_queue_full"
	ResultDroppedShutdown  = "dropped_shutdown"
	ResultWriteError       = "write_error"

	WriteResultSuccess = "success"
	WriteResultError   = "error"
	WriteResultTimeout = "timeout"
)

// MetricsConfig parameterizes audit metrics registration.
type MetricsConfig struct {
	Namespace    string
	Subsystem    string
	LabelName    string
	UnknownLabel string
	HelpPrefix   string
}

// Metrics tracks audit queue and persistence outcomes.
type Metrics struct {
	recordsTotal         *prometheus.CounterVec
	queueDepth           prometheus.Gauge
	writeDurationSeconds *prometheus.HistogramVec
	unknownLabel         string
}

// NewMetrics registers audit metrics collectors.
func NewMetrics(reg prometheus.Registerer, cfg MetricsConfig) (*Metrics, error) {
	if reg == nil {
		return nil, errors.New("prometheus registerer is required")
	}

	recordsTotal, err := registerCounterVec(reg, prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "records_total",
			Help:      fmt.Sprintf("Total number of %s records by result.", cfg.HelpPrefix),
		},
		[]string{cfg.LabelName, "result"},
	))
	if err != nil {
		return nil, fmt.Errorf("register %s records_total metric: %w", cfg.HelpPrefix, err)
	}

	queueDepth, err := registerGauge(reg, prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "queue_depth",
			Help:      fmt.Sprintf("Current number of %s records waiting in the async queue.", cfg.HelpPrefix),
		},
	))
	if err != nil {
		return nil, fmt.Errorf("register %s queue_depth metric: %w", cfg.HelpPrefix, err)
	}

	writeDurationSeconds, err := registerHistogramVec(reg, prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "write_duration_seconds",
			Help:      fmt.Sprintf("Histogram of async %s write durations by result.", cfg.HelpPrefix),
			Buckets:   prometheus.DefBuckets,
		},
		[]string{cfg.LabelName, "result"},
	))
	if err != nil {
		return nil, fmt.Errorf("register %s write_duration_seconds metric: %w", cfg.HelpPrefix, err)
	}

	metrics := &Metrics{
		recordsTotal:         recordsTotal,
		queueDepth:           queueDepth,
		writeDurationSeconds: writeDurationSeconds,
		unknownLabel:         cfg.UnknownLabel,
	}
	metrics.observeQueueDepth(0)

	return metrics, nil
}

func (metrics *Metrics) observeRecordResult(label, result string) {
	if metrics == nil || metrics.recordsTotal == nil {
		return
	}

	metrics.recordsTotal.WithLabelValues(metrics.normalizeLabel(label), strings.TrimSpace(result)).Inc()
}

func (metrics *Metrics) observeWriteDuration(label string, duration time.Duration, result string) {
	if metrics == nil || metrics.writeDurationSeconds == nil {
		return
	}

	metrics.writeDurationSeconds.WithLabelValues(metrics.normalizeLabel(label), strings.TrimSpace(result)).Observe(duration.Seconds())
}

func (metrics *Metrics) observeQueueDepth(depth int) {
	if metrics == nil || metrics.queueDepth == nil {
		return
	}

	depth = max(depth, 0)
	metrics.queueDepth.Set(float64(depth))
}

func (metrics *Metrics) normalizeLabel(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return metrics.unknownLabel
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

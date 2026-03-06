package requestaudit

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewMetricsValidation(t *testing.T) {
	metrics, err := NewMetrics(nil)
	if metrics != nil {
		t.Fatal("NewMetrics returned unexpected metrics instance")
	}
	if err == nil {
		t.Fatal("NewMetrics returned nil error")
	}
	if got, want := err.Error(), "prometheus registerer is required"; got != want {
		t.Fatalf("error mismatch: want %q, got %q", want, got)
	}
}

func TestNewMetricsSupportsAlreadyRegisteredCollectors(t *testing.T) {
	registry := prometheus.NewRegistry()

	metricsA, err := NewMetrics(registry)
	if err != nil {
		t.Fatalf("NewMetrics(first) returned error: %v", err)
	}

	metricsB, err := NewMetrics(registry)
	if err != nil {
		t.Fatalf("NewMetrics(second) returned error: %v", err)
	}

	metricsA.observeRecordResult("", requestAuditResultEnqueued)
	metricsB.observeQueueDepth(3)
	metricsB.observeWriteDuration("public", 25*time.Millisecond, requestAuditWriteResultSuccess)

	if got := testutil.ToFloat64(metricsA.recordsTotal.WithLabelValues(requestAuditServerUnknown, requestAuditResultEnqueued)); got != 1 {
		t.Fatalf("records counter mismatch: want 1, got %v", got)
	}
	if got := testutil.ToFloat64(metricsA.queueDepth); got != 3 {
		t.Fatalf("queue depth mismatch: want 3, got %v", got)
	}
}

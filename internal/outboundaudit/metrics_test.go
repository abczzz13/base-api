package outboundaudit

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestNewOutboundAuditMetricsRegistersSuccessfully(t *testing.T) {
	metrics, err := NewMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("NewMetrics returned error: %v", err)
	}
	if metrics == nil {
		t.Fatal("NewMetrics returned nil metrics")
	}
}

func TestNewOutboundAuditMetricsRequiresRegisterer(t *testing.T) {
	metrics, err := NewMetrics(nil)
	if metrics != nil {
		t.Fatal("NewMetrics returned unexpected metrics instance")
	}
	if err == nil {
		t.Fatal("NewMetrics returned nil error")
	}
}

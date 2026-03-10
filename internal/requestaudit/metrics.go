package requestaudit

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/abczzz13/base-api/internal/asyncaudit"
)

// Metrics tracks request-audit queue and persistence outcomes.
type Metrics = asyncaudit.Metrics

// NewMetrics registers request-audit metrics collectors.
func NewMetrics(reg prometheus.Registerer) (*Metrics, error) {
	return asyncaudit.NewMetrics(reg, asyncaudit.MetricsConfig{
		Namespace:    "base_api",
		Subsystem:    "request_audit",
		LabelName:    "server",
		UnknownLabel: "unknown",
		HelpPrefix:   "request-audit",
	})
}

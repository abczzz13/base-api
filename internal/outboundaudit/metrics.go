package outboundaudit

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/abczzz13/base-api/internal/asyncaudit"
)

// Metrics tracks outbound-audit queue and persistence outcomes.
type Metrics = asyncaudit.Metrics

// NewMetrics registers outbound-audit metrics collectors.
func NewMetrics(reg prometheus.Registerer) (*Metrics, error) {
	return asyncaudit.NewMetrics(reg, asyncaudit.MetricsConfig{
		Namespace:    "base_api",
		Subsystem:    "http_client_audit",
		LabelName:    "client",
		UnknownLabel: "unknown",
		HelpPrefix:   "outbound HTTP audit",
	})
}

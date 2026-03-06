package server

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/abczzz13/base-api/internal/middleware"
	"github.com/abczzz13/base-api/internal/requestaudit"
)

type runtimeMetrics struct {
	registry *prometheus.Registry
	http     *middleware.HTTPRequestMetrics
	audit    *requestaudit.Metrics
}

func setupMetricsRuntime() (*runtimeMetrics, error) {
	metricsRegistry := prometheus.NewRegistry()

	if err := metricsRegistry.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})); err != nil {
		return nil, fmt.Errorf("register process collector: %w", err)
	}

	if err := metricsRegistry.Register(collectors.NewGoCollector()); err != nil {
		return nil, fmt.Errorf("register go collector: %w", err)
	}

	requestMetrics, err := middleware.NewHTTPRequestMetrics(metricsRegistry)
	if err != nil {
		return nil, fmt.Errorf("create request metrics: %w", err)
	}

	requestAuditMetrics, err := requestaudit.NewMetrics(metricsRegistry)
	if err != nil {
		return nil, fmt.Errorf("create request audit metrics: %w", err)
	}

	return &runtimeMetrics{
		registry: metricsRegistry,
		http:     requestMetrics,
		audit:    requestAuditMetrics,
	}, nil
}

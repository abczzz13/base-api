package server

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/abczzz13/base-api/internal/middleware"
)

type runtimeDependencies struct {
	requestMetrics    *middleware.HTTPRequestMetrics
	metricsGatherer   prometheus.Gatherer
	metricsRegisterer prometheus.Registerer
	database          *pgxpool.Pool
}

func newRuntimeDependencies() (runtimeDependencies, error) {
	registry := prometheus.NewRegistry()

	if err := registry.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})); err != nil {
		return runtimeDependencies{}, fmt.Errorf("register process collector: %w", err)
	}

	if err := registry.Register(collectors.NewGoCollector()); err != nil {
		return runtimeDependencies{}, fmt.Errorf("register go collector: %w", err)
	}

	requestMetrics, err := middleware.NewHTTPRequestMetrics(registry)
	if err != nil {
		return runtimeDependencies{}, fmt.Errorf("create request metrics: %w", err)
	}

	return runtimeDependencies{
		requestMetrics:    requestMetrics,
		metricsGatherer:   registry,
		metricsRegisterer: registry,
	}, nil
}

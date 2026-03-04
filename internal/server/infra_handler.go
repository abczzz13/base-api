package server

import (
	"errors"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/docsui"
	"github.com/abczzz13/base-api/internal/infraoas"
	"github.com/abczzz13/base-api/internal/middleware"
)

func newInfraHandler(cfg config.Config, deps runtimeDependencies) (http.Handler, error) {
	if deps.requestMetrics == nil {
		return nil, errors.New("request metrics dependency is required")
	}
	if deps.metricsGatherer == nil {
		return nil, errors.New("metrics gatherer dependency is required")
	}

	var readinessDatabase databaseReadiness
	if deps.database != nil {
		readinessDatabase = deps.database
	}

	infraService := newInfraService(cfg, defaultReadinessCheckers(readinessDatabase)...)

	infraAPI, err := infraoas.NewServer(infraService, infraoas.WithErrorHandler(ogenErrorHandler))
	if err != nil {
		return nil, err
	}

	appMux := http.NewServeMux()
	docsui.Register(appMux)
	appMux.Handle("/", infraAPI)

	middlewares := []func(http.Handler) http.Handler{
		middleware.RequestMetrics(deps.requestMetrics, middleware.RequestMetricsConfig{
			Server:     "infra",
			RouteLabel: infraRequestMetricsRouteLabeler(infraAPI),
		}),
		middleware.RequestLogger(),
		middleware.Recovery(),
	}

	metricsHandler := middleware.NewChain(
		middleware.RequestLogger(),
		middleware.Recovery(),
	).WrapHandler(promhttp.HandlerFor(
		deps.metricsGatherer,
		promhttp.HandlerOpts{EnableOpenMetrics: true},
	))

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metricsHandler)
	mux.Handle("/", middleware.NewChain(middlewares...).WrapHandler(appMux))

	return mux, nil
}

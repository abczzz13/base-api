package infraapi

import (
	"errors"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/abczzz13/base-api/internal/apierrors"
	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/docsui"
	geninfra "github.com/abczzz13/base-api/internal/infraoas"
	"github.com/abczzz13/base-api/internal/middleware"
)

// Dependencies contains runtime dependencies needed by the infra handler.
type Dependencies struct {
	RequestMetrics  *middleware.HTTPRequestMetrics
	MetricsGatherer prometheus.Gatherer
	Database        DatabaseReadiness
	Valkey          ValkeyReadiness
}

// NewHandler creates the fully wrapped infra API HTTP handler.
func NewHandler(cfg config.Config, deps Dependencies) (http.Handler, error) {
	if deps.RequestMetrics == nil {
		return nil, errors.New("request metrics dependency is required")
	}
	if deps.MetricsGatherer == nil {
		return nil, errors.New("metrics gatherer dependency is required")
	}

	infraService := NewService(cfg, DefaultReadinessCheckers(deps.Database, deps.Valkey)...)

	infraAPI, err := geninfra.NewServer(infraService, geninfra.WithErrorHandler(apierrors.OgenErrorHandler))
	if err != nil {
		return nil, err
	}

	appMux := http.NewServeMux()
	docsui.Register(appMux)
	appMux.Handle("/", infraAPI)

	middlewares := []func(http.Handler) http.Handler{
		middleware.RequestID(),
		middleware.RequestMetrics(deps.RequestMetrics, middleware.RequestMetricsConfig{
			Server:     "infra",
			RouteLabel: requestMetricsRouteLabeler(infraAPI),
		}),
	}
	if cfg.RequestLogger.IsEnabled() {
		middlewares = append(middlewares, middleware.RequestLogger())
	}
	middlewares = append(middlewares, middleware.Recovery())

	metricsMiddlewares := make([]func(http.Handler) http.Handler, 0, 3)
	metricsMiddlewares = append(metricsMiddlewares, middleware.RequestID())
	if cfg.RequestLogger.IsEnabled() {
		metricsMiddlewares = append(metricsMiddlewares, middleware.RequestLogger())
	}
	metricsMiddlewares = append(metricsMiddlewares, middleware.Recovery())

	metricsHandler := middleware.NewChain(metricsMiddlewares...).WrapHandler(promhttp.HandlerFor(
		deps.MetricsGatherer,
		promhttp.HandlerOpts{EnableOpenMetrics: true},
	))

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metricsHandler)
	mux.Handle("/", middleware.NewChain(middlewares...).WrapHandler(appMux))

	return mux, nil
}

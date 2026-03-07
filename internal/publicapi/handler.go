package publicapi

import (
	"errors"
	"net/http"

	"github.com/abczzz13/base-api/internal/apierrors"
	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/middleware"
	"github.com/abczzz13/base-api/internal/publicoas"
	"github.com/abczzz13/base-api/internal/requestaudit"
	"github.com/abczzz13/base-api/internal/weather"
)

// Dependencies contains runtime dependencies needed by the public handler.
type Dependencies struct {
	RequestMetrics         *middleware.HTTPRequestMetrics
	RequestAuditRepository requestaudit.Repository
	WeatherClient          weather.Client
}

// NewHandler creates the fully wrapped public API HTTP handler.
func NewHandler(cfg config.Config, deps Dependencies) (http.Handler, error) {
	if deps.RequestMetrics == nil {
		return nil, errors.New("request metrics dependency is required")
	}

	baseService := NewService(cfg, deps.WeatherClient)

	serverOptions := []publicoas.ServerOption{publicoas.WithErrorHandler(apierrors.OgenErrorHandler)}
	if cfg.OTEL.TracingEnabled {
		serverOptions = append(serverOptions, publicoas.WithMiddleware(middleware.OTELOperationAttributes()))
	}

	baseAPI, err := publicoas.NewServer(baseService, serverOptions...)
	if err != nil {
		return nil, err
	}

	routeLabel := requestMetricsRouteLabeler(baseAPI)

	middlewares := make([]func(http.Handler) http.Handler, 0, 7)
	middlewares = append(middlewares,
		middleware.RequestID(),
		middleware.RequestMetrics(deps.RequestMetrics, middleware.RequestMetricsConfig{
			Server:     "public",
			RouteLabel: routeLabel,
		}),
	)

	if cfg.OTEL.TracingEnabled {
		middlewares = append(middlewares,
			middleware.Tracing("public"),
			middleware.TraceResponseHeader(),
		)
	}

	if cfg.RequestAudit.IsEnabled() {
		if deps.RequestAuditRepository == nil {
			return nil, errors.New("request audit store dependency is required")
		}

		middlewares = append(middlewares,
			middleware.RequestAudit(middleware.RequestAuditConfig{
				Store:             deps.RequestAuditRepository,
				Server:            "public",
				RouteLabel:        routeLabel,
				TrustedProxyCIDRs: cfg.RequestAudit.TrustedProxyCIDRs,
			}),
		)
	}

	if cfg.RequestLogger.IsEnabled() {
		middlewares = append(middlewares, middleware.RequestLogger())
	}

	middlewares = append(middlewares,
		middleware.Recovery(),
		middleware.CORS(middleware.CORSConfig{
			AllowedOrigins:   cfg.CORS.AllowedOrigins,
			AllowedMethods:   cfg.CORS.AllowedMethods,
			AllowedHeaders:   cfg.CORS.AllowedHeaders,
			ExposedHeaders:   cfg.CORS.ExposedHeaders,
			AllowCredentials: cfg.CORS.AllowCredentials,
			MaxAge:           cfg.CORS.MaxAge,
		}),
	)

	chain := middleware.NewChain(middlewares...)

	if cfg.CSRF.Enabled {
		chain.With(middleware.CSRF(middleware.CSRFConfig{
			TrustedOrigins: cfg.CSRF.TrustedOrigins,
		}))
	}

	return chain.WrapHandler(baseAPI), nil
}

package server

import (
	"errors"
	"net/http"

	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/middleware"
	"github.com/abczzz13/base-api/internal/oas"
)

func newPublicHandler(cfg config.Config, deps runtimeDependencies) (http.Handler, error) {
	if deps.requestMetrics == nil {
		return nil, errors.New("request metrics dependency is required")
	}

	baseService := newBaseService(cfg)

	serverOptions := []oas.ServerOption{oas.WithErrorHandler(ogenErrorHandler)}
	if cfg.OTEL.TracingEnabled {
		serverOptions = append(serverOptions, oas.WithMiddleware(middleware.OTELOperationAttributes()))
	}

	baseAPI, err := oas.NewServer(baseService, serverOptions...)
	if err != nil {
		return nil, err
	}

	middlewares := make([]func(http.Handler) http.Handler, 0, 6)
	middlewares = append(middlewares,
		middleware.RequestMetrics(deps.requestMetrics, middleware.RequestMetricsConfig{
			Server:     "public",
			RouteLabel: publicRequestMetricsRouteLabeler(baseAPI),
		}),
	)

	if cfg.OTEL.TracingEnabled {
		middlewares = append(middlewares,
			middleware.Tracing("public"),
			middleware.TraceResponseHeader(),
		)
	}

	middlewares = append(middlewares,
		middleware.RequestLogger(),
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

package publicapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/abczzz13/base-api/internal/apierrors"
	"github.com/abczzz13/base-api/internal/clientip"
	"github.com/abczzz13/base-api/internal/clients/weather"
	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/middleware"
	"github.com/abczzz13/base-api/internal/notes"
	"github.com/abczzz13/base-api/internal/publicoas"
	"github.com/abczzz13/base-api/internal/ratelimit"
	"github.com/abczzz13/base-api/internal/requestaudit"
	"github.com/abczzz13/base-api/internal/weatherapi"
	"github.com/abczzz13/base-api/internal/weatheroas"
)

// Dependencies contains runtime dependencies needed by the public handler.
type Dependencies struct {
	RequestMetrics         *middleware.HTTPRequestMetrics
	RequestAuditRepository requestaudit.Repository
	NotesRepository        notes.Repository
	RateLimiter            ratelimit.Store
	WeatherClient          weather.Client
}

// NewHandler creates the fully wrapped public API HTTP handler.
func NewHandler(cfg config.Config, deps Dependencies) (http.Handler, error) {
	if deps.RequestMetrics == nil {
		return nil, errors.New("request metrics dependency is required")
	}
	if deps.WeatherClient == nil {
		return nil, errors.New("weather client dependency is required")
	}
	if deps.NotesRepository == nil {
		return nil, errors.New("notes repository dependency is required")
	}

	baseService := NewService()
	notesService, err := notes.NewService(deps.NotesRepository, deps.WeatherClient)
	if err != nil {
		return nil, fmt.Errorf("create notes service: %w", err)
	}
	weatherService := weatherapi.NewService(deps.WeatherClient)

	var clientIPResolver *clientip.Resolver
	if cfg.RequestAudit.IsEnabled() || cfg.RateLimit.IsEnabled() {
		clientIPResolver = clientip.NewResolver("public handler", cfg.ClientIP.TrustedProxyCIDRs)
	}

	serverOptions := []publicoas.ServerOption{publicoas.WithErrorHandler(apierrors.OgenErrorHandler)}
	if cfg.OTEL.TracingEnabled {
		serverOptions = append(serverOptions, publicoas.WithMiddleware(middleware.OTELOperationAttributes()))
	}

	oasHandler, err := NewOASHandler(baseService, notesService)
	if err != nil {
		return nil, fmt.Errorf("create public OAS handler: %w", err)
	}

	baseAPI, err := publicoas.NewServer(oasHandler, serverOptions...)
	if err != nil {
		return nil, fmt.Errorf("create public OAS server: %w", err)
	}

	weatherServerOptions := []weatheroas.ServerOption{weatheroas.WithErrorHandler(apierrors.OgenErrorHandler)}
	if cfg.OTEL.TracingEnabled {
		weatherServerOptions = append(weatherServerOptions, weatheroas.WithMiddleware(middleware.OTELOperationAttributes()))
	}

	weatherOASHandler, err := weatherapi.NewOASHandler(weatherService)
	if err != nil {
		return nil, fmt.Errorf("create weather OAS handler: %w", err)
	}

	weatherAPI, err := weatheroas.NewServer(weatherOASHandler, weatherServerOptions...)
	if err != nil {
		return nil, fmt.Errorf("create weather OAS server: %w", err)
	}

	appMux := http.NewServeMux()
	appMux.Handle("/weather/", weatherAPI)
	appMux.Handle("/", baseAPI)

	routeLabel := requestMetricsRouteLabeler(coreRouteLabeler(baseAPI), weatherapi.RouteLabeler(weatherAPI))

	middlewares := make([]func(http.Handler) http.Handler, 0, 10)
	middlewares = append(middlewares,
		middleware.Recovery(),
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
				ClientIPResolver: clientIPResolver,
				Store:            deps.RequestAuditRepository,
				Server:           "public",
				RouteLabel:       routeLabel,
			}),
		)
	}

	if cfg.RequestLogger.IsEnabled() {
		middlewares = append(middlewares, middleware.RequestLogger())
	}

	middlewares = append(middlewares,
		middleware.CORS(middleware.CORSConfig{
			AllowedOrigins:   cfg.CORS.AllowedOrigins,
			AllowedMethods:   cfg.CORS.AllowedMethods,
			AllowedHeaders:   cfg.CORS.AllowedHeaders,
			ExposedHeaders:   cfg.CORS.ExposedHeaders,
			AllowCredentials: cfg.CORS.AllowCredentials,
			MaxAge:           cfg.CORS.MaxAge,
		}),
	)

	if cfg.CSRF.Enabled {
		middlewares = append(middlewares, middleware.CSRF(middleware.CSRFConfig{
			TrustedOrigins: cfg.CSRF.TrustedOrigins,
		}))
	}

	if cfg.RateLimit.IsEnabled() {
		if deps.RateLimiter == nil {
			return nil, errors.New("rate limiter dependency is required")
		}

		middlewares = append(middlewares, middleware.RateLimit(middleware.RateLimitConfig{
			ClientIPResolver: clientIPResolver,
			Store:            deps.RateLimiter,
			Server:           "public",
			RouteLabel:       routeLabel,
			BackendTimeout:   cfg.RateLimit.Timeout,
			FailOpen:         cfg.RateLimit.FailOpen,
			DefaultPolicy:    cfg.RateLimit.DefaultPolicy,
			RouteOverrides:   cfg.RateLimit.RouteOverrides,
		}))
	}

	return middleware.NewChain(middlewares...).WrapHandler(appMux), nil
}

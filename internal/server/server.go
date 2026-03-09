package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/abczzz13/base-api/internal/clients/weather"
	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/httpclient"
	"github.com/abczzz13/base-api/internal/infraapi"
	"github.com/abczzz13/base-api/internal/logger"
	"github.com/abczzz13/base-api/internal/outboundaudit"
	"github.com/abczzz13/base-api/internal/postgres"
	"github.com/abczzz13/base-api/internal/publicapi"
	"github.com/abczzz13/base-api/internal/ratelimit"
	"github.com/abczzz13/base-api/internal/requestaudit"
	"github.com/abczzz13/base-api/internal/telemetry"
	"github.com/abczzz13/base-api/internal/valkey"
	"github.com/abczzz13/base-api/internal/version"
)

func Run(
	ctx context.Context,
	lookupEnv func(string) (string, bool),
	stderr io.Writer,
) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.Load(lookupEnv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger.New(logger.Config{
		Format:      cfg.LogFormat,
		Level:       cfg.LogLevel,
		Version:     version.GetVersion(),
		Environment: cfg.Environment,
		Writer:      stderr,
	})

	buildMetadata := version.GetBuildMetadata()
	slog.InfoContext(
		ctx,
		"build metadata",
		slog.String("version", buildMetadata.Version),
		slog.String("git_commit", buildMetadata.GitCommit),
		slog.String("git_commit_short", buildMetadata.GitCommitShort),
		slog.String("git_tag", buildMetadata.GitTag),
		slog.String("git_branch", buildMetadata.GitBranch),
		slog.String("git_tree_state", buildMetadata.GitTreeState),
		slog.String("build_time", buildMetadata.BuildTime),
		slog.String("go_version", buildMetadata.GoVersion),
	)

	runtimeCleanup := NewCleanupStack()
	defer runtimeCleanup.Run()

	tracingEnabled, shutdownTracing := telemetry.Setup(
		ctx,
		cfg.OTEL.TracingEnabled,
		cfg.OTEL.TelemetryConfig(cfg.Environment, version.GetVersion()),
	)
	cfg.OTEL.TracingEnabled = tracingEnabled
	runtimeCleanup.Add(shutdownTracing)

	logStartupConfiguration(ctx, cfg)

	metricsRuntime, err := setupMetricsRuntime()
	if err != nil {
		return fmt.Errorf("configure metrics runtime: %w", err)
	}

	database, databaseShutdown, err := postgres.SetupRuntime(ctx, cfg.DB, metricsRuntime.registry)
	if err != nil {
		return fmt.Errorf("configure database: %w", err)
	}
	runtimeCleanup.Add(databaseShutdown)

	baseRequestAuditRepository := requestaudit.NewPostgresRepository(database)
	requestAuditRepository, shutdownRequestAuditRepository := requestaudit.NewAsyncRepositoryWithConfig(
		baseRequestAuditRepository,
		requestaudit.AsyncConfig{Metrics: metricsRuntime.audit},
	)
	runtimeCleanup.Add(shutdownRequestAuditRepository)

	baseOutboundAuditRepository := outboundaudit.NewPostgresRepository(database)
	outboundAuditRepository, shutdownOutboundAuditRepository := outboundaudit.NewAsyncRepositoryWithConfig(
		baseOutboundAuditRepository,
		outboundaudit.AsyncConfig{Metrics: metricsRuntime.outboundAudit},
	)
	runtimeCleanup.Add(shutdownOutboundAuditRepository)

	rateLimiter, err := setupRateLimiter(ctx, cfg, runtimeCleanup)
	if err != nil {
		return err
	}

	weatherClient, err := setupWeatherClient(cfg, metricsRuntime.httpClient, outboundAuditRepository)
	if err != nil {
		return err
	}

	publicHandler, err := publicapi.NewHandler(cfg, publicapi.Dependencies{
		RequestMetrics:         metricsRuntime.http,
		RequestAuditRepository: requestAuditRepository,
		RateLimiter:            rateLimiter,
		WeatherClient:          weatherClient,
	})
	if err != nil {
		return fmt.Errorf("create public API handler: %w", err)
	}

	infraHandler, err := infraapi.NewHandler(cfg, infraapi.Dependencies{
		RequestMetrics:  metricsRuntime.http,
		MetricsGatherer: metricsRuntime.registry,
		Database:        database,
	})
	if err != nil {
		return fmt.Errorf("create infra API handler: %w", err)
	}

	servers := []ManagedServer{
		{
			Name:   "public",
			Server: NewHTTPServer(cfg, cfg.Address, publicHandler),
		},
		{
			Name:   "infra",
			Server: NewHTTPServer(cfg, cfg.InfraAddress, infraHandler),
		},
	}

	if err := BindListeners(ctx, servers); err != nil {
		return err
	}

	serverResults := RunServers(servers)

	shutdownAll := func() error {
		shutdownErr := ShutdownServers(servers)
		runtimeCleanup.Run()
		return shutdownErr
	}

	select {
	case res := <-serverResults:
		serveErr := res.Err
		if serveErr == nil {
			serveErr = fmt.Errorf("%s server stopped unexpectedly", res.Name)
		}

		return errors.Join(serveErr, shutdownAll())
	case <-ctx.Done():
		slog.InfoContext(ctx, "shutting down")

		if err := shutdownAll(); err != nil {
			return err
		}

		for range servers {
			res := <-serverResults
			if res.Err != nil {
				return res.Err
			}
		}

		return nil
	}
}

func logStartupConfiguration(ctx context.Context, cfg config.Config) {
	attrs := []slog.Attr{
		slog.String("environment", cfg.Environment),
		slog.String("api_address", cfg.Address),
		slog.String("infra_address", cfg.InfraAddress),
		slog.Duration("timeout_readyz", cfg.ReadyzTimeout),
		slog.Duration("timeout_read_header", cfg.ReadHeaderTimeout),
		slog.Duration("timeout_read", cfg.ReadTimeout),
		slog.Duration("timeout_write", cfg.WriteTimeout),
		slog.Duration("timeout_idle", cfg.IdleTimeout),
		slog.Bool("csrf_enabled", cfg.CSRF.Enabled),
		slog.Int("csrf_trusted_origins_count", len(cfg.CSRF.TrustedOrigins)),
		slog.Int("cors_allowed_origins_count", len(cfg.CORS.AllowedOrigins)),
		slog.Bool("cors_allow_credentials", cfg.CORS.AllowCredentials),
		slog.Bool("request_logger_enabled", cfg.RequestLogger.IsEnabled()),
		slog.Bool("rate_limit_enabled", cfg.RateLimit.IsEnabled()),
		slog.Bool("rate_limit_fail_open", cfg.RateLimit.FailOpen),
		slog.Duration("rate_limit_timeout", cfg.RateLimit.Timeout),
		slog.Float64("rate_limit_default_rps", cfg.RateLimit.DefaultPolicy.RequestsPerSecond),
		slog.Int("rate_limit_default_burst", cfg.RateLimit.DefaultPolicy.Burst),
		slog.String("valkey_mode", string(cfg.Valkey.NormalizedMode())),
		slog.Int("valkey_addrs_count", len(cfg.Valkey.Addrs)),
		slog.Int("rate_limit_route_overrides_count", len(cfg.RateLimit.RouteOverrides)),
		slog.Bool("request_audit_enabled", cfg.RequestAudit.IsEnabled()),
		slog.Bool("tracing_enabled", cfg.OTEL.TracingEnabled),
		slog.String("tracing_sampler", string(cfg.OTEL.TracesSampler)),
		slog.Bool("database_enabled", cfg.DB.Enabled()),
		slog.Int64("database_min_conns", int64(cfg.DB.MinConns)),
		slog.Int64("database_max_conns", int64(cfg.DB.MaxConns)),
		slog.Bool("database_migrate_on_startup", cfg.DB.MigrateOnStartup),
		slog.Int64("database_startup_max_attempts", int64(cfg.DB.StartupMaxAttempts)),
		slog.Duration("database_startup_backoff_initial", cfg.DB.StartupBackoffInitial),
		slog.Duration("database_startup_backoff_max", cfg.DB.StartupBackoffMax),
		slog.String("client_ip_security_mode", "strict"),
		slog.String("client_ip_priority", "x_forwarded_for,remote_addr"),
		slog.String("client_ip_trusted_proxy_source", clientIPTrustedProxySource(cfg.ClientIP)),
		slog.Int("client_ip_trusted_proxy_cidrs_count", len(cfg.ClientIP.TrustedProxyCIDRs)),
	}

	if cfg.OTEL.TracesSamplerArg != nil {
		attrs = append(attrs, slog.Float64("tracing_sampler_arg", *cfg.OTEL.TracesSamplerArg))
	}

	attrs = append(attrs, slog.Bool("weather_enabled", cfg.Weather.Enabled()))

	slog.LogAttrs(ctx, slog.LevelInfo, "startup configuration", attrs...)
}

func clientIPTrustedProxySource(cfg config.ClientIPConfig) string {
	if len(cfg.TrustedProxyCIDRs) == 0 {
		return "default_local_private"
	}

	return "configured"
}

func setupRateLimiter(ctx context.Context, cfg config.Config, cleanup *CleanupStack) (ratelimit.Store, error) {
	if !cfg.RateLimit.IsEnabled() {
		return nil, nil
	}

	valkeyClient, err := valkey.NewClient(cfg.Valkey)
	if err != nil {
		if !cfg.RateLimit.FailOpen {
			return nil, fmt.Errorf("configure Valkey client: %w", err)
		}

		slog.WarnContext(ctx, "valkey client unavailable; starting in fail-open mode",
			slog.Any("error", err),
			slog.String("mode", string(cfg.Valkey.NormalizedMode())),
			slog.Int("address_count", len(cfg.Valkey.Addrs)),
		)

		return startupUnavailableRateLimiter(err), nil
	}

	cleanup.Add(func() { valkeyClient.Close() })

	store, err := ratelimit.NewValkeyStore(ratelimit.ValkeyStoreConfig{
		Client:    valkeyClient,
		KeyPrefix: cfg.RateLimit.KeyPrefix,
	})
	if err != nil {
		return nil, fmt.Errorf("configure rate limiter: %w", err)
	}

	return store, nil
}

func startupUnavailableRateLimiter(err error) ratelimit.Store {
	return ratelimit.StoreFunc(func(context.Context, string, ratelimit.Policy) (ratelimit.Decision, error) {
		if err == nil {
			return ratelimit.Decision{}, ratelimit.ErrStartupBackendUnavailable
		}

		return ratelimit.Decision{}, fmt.Errorf("%w: %w", ratelimit.ErrStartupBackendUnavailable, err)
	})
}

func setupWeatherClient(cfg config.Config, httpMetrics *httpclient.Metrics, auditRepo outboundaudit.Repository) (weather.Client, error) {
	if !cfg.Weather.Enabled() {
		return nil, nil
	}

	geocodingClient, err := httpclient.New(httpclient.Config{
		Client:          "open_meteo_geocoding",
		BaseURL:         cfg.Weather.GeocodingBaseURL,
		Metrics:         httpMetrics,
		AuditRepository: auditRepo,
	})
	if err != nil {
		return nil, fmt.Errorf("create weather geocoding client: %w", err)
	}

	forecastClient, err := httpclient.New(httpclient.Config{
		Client:          "open_meteo_forecast",
		BaseURL:         cfg.Weather.ForecastBaseURL,
		Metrics:         httpMetrics,
		AuditRepository: auditRepo,
	})
	if err != nil {
		return nil, fmt.Errorf("create weather forecast client: %w", err)
	}

	weatherClient, err := weather.New(geocodingClient, forecastClient, cfg.Weather.APIKey, cfg.Weather.Timeout)
	if err != nil {
		return nil, fmt.Errorf("configure weather integration: %w", err)
	}

	return weatherClient, nil
}

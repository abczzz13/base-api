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

	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/httpserver"
	"github.com/abczzz13/base-api/internal/infraapi"
	"github.com/abczzz13/base-api/internal/logger"
	"github.com/abczzz13/base-api/internal/outboundaudit"
	"github.com/abczzz13/base-api/internal/outboundhttp"
	"github.com/abczzz13/base-api/internal/postgres"
	"github.com/abczzz13/base-api/internal/publicapi"
	"github.com/abczzz13/base-api/internal/requestaudit"
	"github.com/abczzz13/base-api/internal/telemetry"
	"github.com/abczzz13/base-api/internal/version"
	"github.com/abczzz13/base-api/internal/weather"
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

	runtimeCleanup := httpserver.NewCleanupStack()
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

	var weatherClient weather.Client
	if cfg.Weather.Enabled() {
		weatherGeocodingClient, err := outboundhttp.New(outboundhttp.Config{
			Client:          "open_meteo_geocoding",
			BaseURL:         cfg.Weather.GeocodingBaseURL,
			Metrics:         metricsRuntime.httpClient,
			AuditRepository: outboundAuditRepository,
		})
		if err != nil {
			return fmt.Errorf("create weather geocoding client: %w", err)
		}

		weatherForecastClient, err := outboundhttp.New(outboundhttp.Config{
			Client:          "open_meteo_forecast",
			BaseURL:         cfg.Weather.ForecastBaseURL,
			Metrics:         metricsRuntime.httpClient,
			AuditRepository: outboundAuditRepository,
		})
		if err != nil {
			return fmt.Errorf("create weather forecast client: %w", err)
		}

		weatherClient, err = weather.New(weatherGeocodingClient, weatherForecastClient, cfg.Weather.APIKey, cfg.Weather.Timeout)
		if err != nil {
			return fmt.Errorf("configure weather integration: %w", err)
		}
	}

	publicHandler, err := publicapi.NewHandler(cfg, publicapi.Dependencies{
		RequestMetrics:         metricsRuntime.http,
		RequestAuditRepository: requestAuditRepository,
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

	servers := []httpserver.ManagedServer{
		{
			Name:   "public",
			Server: httpserver.NewHTTPServer(cfg, cfg.Address, publicHandler),
		},
		{
			Name:   "infra",
			Server: httpserver.NewHTTPServer(cfg, cfg.InfraAddress, infraHandler),
		},
	}

	if err := httpserver.BindListeners(ctx, servers); err != nil {
		return err
	}

	serverResults := httpserver.RunServers(servers)

	shutdownAll := func() error {
		shutdownErr := httpserver.ShutdownServers(servers)
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
		slog.String("request_audit_client_ip_security_mode", "strict"),
		slog.String("request_audit_client_ip_priority", "x_forwarded_for,remote_addr"),
		slog.String("request_audit_trusted_proxy_source", requestAuditTrustedProxySource(cfg.RequestAudit)),
		slog.Int("request_audit_trusted_proxy_cidrs_count", len(cfg.RequestAudit.TrustedProxyCIDRs)),
	}

	if cfg.OTEL.TracesSamplerArg != nil {
		attrs = append(attrs, slog.Float64("tracing_sampler_arg", *cfg.OTEL.TracesSamplerArg))
	}

	attrs = append(attrs, slog.Bool("weather_enabled", cfg.Weather.Enabled()))

	slog.LogAttrs(ctx, slog.LevelInfo, "startup configuration", attrs...)
}

func requestAuditTrustedProxySource(cfg config.RequestAuditConfig) string {
	if len(cfg.TrustedProxyCIDRs) == 0 {
		return "default_local_private"
	}

	return "configured"
}

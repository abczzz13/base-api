package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/abczzz13/base-api/internal/docsui"
	"github.com/abczzz13/base-api/internal/infraoas"
	"github.com/abczzz13/base-api/internal/logger"
	"github.com/abczzz13/base-api/internal/middleware"
	"github.com/abczzz13/base-api/internal/oas"
	"github.com/abczzz13/base-api/internal/telemetry"
	"github.com/abczzz13/base-api/internal/version"
)

type serverResult struct {
	name string
	err  error
}

type managedServer struct {
	name     string
	server   *http.Server
	listener net.Listener
}

const (
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 15 * time.Second
	defaultWriteTimeout      = 30 * time.Second
	defaultIdleTimeout       = 60 * time.Second
)

func newHTTPServer(cfg Config, addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}
}

func Run(
	ctx context.Context,
	args []string,
	getenv func(string) string,
	stdin io.Reader,
	stdout, stderr io.Writer,
) error {
	_ = args
	_ = stdin
	_ = stdout

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg, configWarnings := loadConfigWithWarnings(getenv)

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

	for _, warning := range configWarnings {
		slog.WarnContext(ctx, "config warning", slog.String("warning", warning))
	}

	telemetryShutdown := func(context.Context) error { return nil }
	if cfg.OTEL.TracingEnabled {
		var telemetryErr error
		telemetryShutdown, telemetryErr = telemetry.InitTracing(ctx, telemetry.Config{
			ServiceName:      cfg.OTEL.ServiceName,
			ServiceVersion:   version.GetVersion(),
			Environment:      cfg.Environment,
			TracesSampler:    cfg.OTEL.TracesSampler,
			TracesSamplerArg: cfg.OTEL.TracesSamplerArg,
		})
		if telemetryErr != nil {
			slog.WarnContext(ctx, "OpenTelemetry tracing disabled", "error", telemetryErr)
			cfg.OTEL.TracingEnabled = false
			telemetryShutdown = func(context.Context) error { return nil }
		}
	}

	var shutdownTracingOnce sync.Once
	shutdownTracing := func() {
		shutdownTracingOnce.Do(func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()

			if err := telemetryShutdown(shutdownCtx); err != nil {
				slog.WarnContext(ctx, "shutdown tracing", "error", err)
			}
		})
	}
	defer shutdownTracing()

	publicHandler, err := newPublicHandler(cfg)
	if err != nil {
		return fmt.Errorf("create public API handler: %w", err)
	}

	infraHandler, err := newInfraHandler(cfg)
	if err != nil {
		return fmt.Errorf("create infra API handler: %w", err)
	}

	servers := []managedServer{
		{
			name:   "public",
			server: newHTTPServer(cfg, cfg.Address, publicHandler),
		},
		{
			name:   "infra",
			server: newHTTPServer(cfg, cfg.InfraAddress, infraHandler),
		},
	}

	closeBoundListeners := func() {
		for i := range servers {
			if servers[i].listener != nil {
				_ = servers[i].listener.Close()
			}
		}
	}

	for i := range servers {
		listener, err := net.Listen("tcp", servers[i].server.Addr)
		if err != nil {
			closeBoundListeners()
			return fmt.Errorf("create %s listener: %w", servers[i].name, err)
		}

		servers[i].listener = listener

		slog.With(
			slog.String("server", servers[i].name),
			slog.String("address", listener.Addr().String()),
		).InfoContext(ctx, "server listening")
	}

	errCh := make(chan serverResult, len(servers))
	for _, s := range servers {
		go func(name string, srv *http.Server, l net.Listener) {
			if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- serverResult{name: name, err: fmt.Errorf("%s server serve: %w", name, err)}
				return
			}
			errCh <- serverResult{name: name}
		}(s.name, s.server, s.listener)
	}

	shutdownAll := func() error {
		defer shutdownTracing()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		for _, s := range servers {
			if err := s.server.Shutdown(shutdownCtx); err != nil {
				return fmt.Errorf("shutdown %s server: %w", s.name, err)
			}
		}

		return nil
	}

	select {
	case res := <-errCh:
		if err := shutdownAll(); err != nil {
			return err
		}
		if res.err != nil {
			return res.err
		}
		return fmt.Errorf("%s server stopped unexpectedly", res.name)
	case <-ctx.Done():
		slog.InfoContext(ctx, "shutting down")

		if err := shutdownAll(); err != nil {
			return err
		}

		for range servers {
			res := <-errCh
			if res.err != nil {
				return res.err
			}
		}

		return nil
	}
}

func newInfraHandler(cfg Config) (http.Handler, error) {
	infraService := newInfraService(cfg, defaultReadinessCheckers(cfg)...)

	infraAPI, err := infraoas.NewServer(infraService, infraoas.WithErrorHandler(ogenErrorHandler))
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{EnableOpenMetrics: true},
	))
	docsui.Register(mux)
	mux.Handle("/", infraAPI)

	middlewares := []func(http.Handler) http.Handler{
		middleware.RequestLogger(),
		middleware.Recovery(),
	}

	return middleware.NewChain(middlewares...).WrapHandler(mux), nil
}

func newPublicHandler(cfg Config) (http.Handler, error) {
	baseService := newBaseService(cfg)

	serverOptions := []oas.ServerOption{oas.WithErrorHandler(ogenErrorHandler)}
	if cfg.OTEL.TracingEnabled {
		serverOptions = append(serverOptions, oas.WithMiddleware(middleware.OTELOperationAttributes()))
	}

	baseAPI, err := oas.NewServer(baseService, serverOptions...)
	if err != nil {
		return nil, err
	}

	middlewares := make([]func(http.Handler) http.Handler, 0, 5)
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

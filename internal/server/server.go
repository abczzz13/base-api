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
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/abczzz13/base-api/internal/docsui"
	"github.com/abczzz13/base-api/internal/infraoas"
	"github.com/abczzz13/base-api/internal/logger"
	"github.com/abczzz13/base-api/internal/middleware"
	"github.com/abczzz13/base-api/internal/oas"
	"github.com/abczzz13/base-api/internal/version"
)

type serverResult struct {
	name string
	err  error
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

	for _, warning := range configWarnings {
		slog.WarnContext(ctx, "config warning", "warning", warning)
	}

	publicHandler, err := newPublicHandler(cfg)
	if err != nil {
		return fmt.Errorf("create public API handler: %w", err)
	}

	infraHandler, err := newInfraHandler(cfg)
	if err != nil {
		return fmt.Errorf("create infra API handler: %w", err)
	}

	servers := []struct {
		name   string
		server *http.Server
	}{
		{
			name:   "public",
			server: newHTTPServer(cfg, cfg.Address, publicHandler),
		},
		{
			name:   "infra",
			server: newHTTPServer(cfg, cfg.InfraAddress, infraHandler),
		},
	}

	listeningServers := make([]struct {
		name     string
		server   *http.Server
		listener net.Listener
	}, 0, len(servers))

	closeBoundListeners := func() {
		for _, s := range listeningServers {
			_ = s.listener.Close()
		}
	}

	for _, s := range servers {
		listener, err := net.Listen("tcp", s.server.Addr)
		if err != nil {
			closeBoundListeners()
			return fmt.Errorf("create %s listener: %w", s.name, err)
		}

		listeningServers = append(listeningServers, struct {
			name     string
			server   *http.Server
			listener net.Listener
		}{
			name:     s.name,
			server:   s.server,
			listener: listener,
		})
	}

	for _, s := range listeningServers {
		slog.With(
			slog.String("server", s.name),
			slog.String("address", s.listener.Addr().String()),
		).InfoContext(ctx, "server listening")
	}

	errCh := make(chan serverResult, len(listeningServers))
	for _, s := range listeningServers {
		go func(name string, srv *http.Server, l net.Listener) {
			if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- serverResult{name: name, err: fmt.Errorf("%s server serve: %w", name, err)}
				return
			}
			errCh <- serverResult{name: name}
		}(s.name, s.server, s.listener)
	}

	shutdownAll := func() error {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		for _, s := range listeningServers {
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

		for range listeningServers {
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

	chain := middleware.NewChain(
		middleware.RequestLogger(),
		middleware.Recovery(),
	)

	return chain.WrapHandler(mux), nil
}

func newPublicHandler(cfg Config) (http.Handler, error) {
	baseService := newBaseService(cfg)

	baseAPI, err := oas.NewServer(baseService, oas.WithErrorHandler(ogenErrorHandler))
	if err != nil {
		return nil, err
	}

	chain := middleware.NewChain(
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

	if cfg.CSRF.Enabled {
		chain.With(middleware.CSRF(middleware.CSRFConfig{
			TrustedOrigins: cfg.CSRF.TrustedOrigins,
		}))
	}

	return chain.WrapHandler(baseAPI), nil
}

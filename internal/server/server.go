package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/abczzz13/base-api/internal/infraoas"
	"github.com/abczzz13/base-api/internal/oas"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type serverResult struct {
	name string
	err  error
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

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg := loadConfig(getenv)

	baseService := newBaseService(cfg)
	publicAPI, err := oas.NewServer(baseService)
	if err != nil {
		return fmt.Errorf("create public API server: %w", err)
	}

	infraHandler, err := newInfraHandler(cfg)
	if err != nil {
		return fmt.Errorf("create infra API server: %w", err)
	}

	servers := []struct {
		name   string
		server *http.Server
	}{
		{
			name: "public",
			server: &http.Server{
				Addr:    cfg.Address,
				Handler: publicAPI,
			},
		},
		{
			name: "infra",
			server: &http.Server{
				Addr:    cfg.InfraAddress,
				Handler: infraHandler,
			},
		},
	}

	errCh := make(chan serverResult, len(servers))
	for _, s := range servers {
		go func(name string, srv *http.Server) {
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- serverResult{name: name, err: fmt.Errorf("%s server listen: %w", name, err)}
				return
			}
			errCh <- serverResult{name: name}
		}(s.name, s.server)
	}

	fmt.Fprintf(stdout, "public listening on %s (environment=%s)\n", cfg.Address, cfg.Environment)
	fmt.Fprintf(stdout, "infra listening on %s (internal only)\n", cfg.InfraAddress)

	shutdownAll := func() error {
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
		fmt.Fprintln(stderr, "shutting down")

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
	infraAPI, err := infraoas.NewServer(infraService)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	// Keep /metrics in front of the generated OAS router so promhttp can handle
	// content negotiation and compression directly.
	mux.Handle("GET /metrics", promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{EnableOpenMetrics: true},
	))
	mux.Handle("/", infraAPI)

	return mux, nil
}

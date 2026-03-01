package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/abczzz13/base-api/internal/config"
)

const defaultServerShutdownTimeout = 10 * time.Second

type serverResult struct {
	name string
	err  error
}

type managedServer struct {
	name     string
	server   *http.Server
	listener net.Listener
}

func newHTTPServer(cfg config.Config, addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}
}

func bindServerListeners(ctx context.Context, servers []managedServer) error {
	for i := range servers {
		listener, err := net.Listen("tcp", servers[i].server.Addr)
		if err != nil {
			closeBoundListeners(servers)
			return fmt.Errorf("create %s listener: %w", servers[i].name, err)
		}

		servers[i].listener = listener

		slog.With(
			slog.String("server", servers[i].name),
			slog.String("address", listener.Addr().String()),
		).InfoContext(ctx, "server listening")
	}

	return nil
}

func closeBoundListeners(servers []managedServer) {
	for i := range servers {
		if servers[i].listener != nil {
			_ = servers[i].listener.Close()
		}
	}
}

func serveServers(servers []managedServer) <-chan serverResult {
	errCh := make(chan serverResult, len(servers))
	for _, server := range servers {
		go func(name string, srv *http.Server, listener net.Listener) {
			if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- serverResult{name: name, err: fmt.Errorf("%s server serve: %w", name, err)}
				return
			}

			errCh <- serverResult{name: name}
		}(server.name, server.server, server.listener)
	}

	return errCh
}

func shutdownServers(servers []managedServer) error {
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), defaultServerShutdownTimeout)
	defer shutdownCancel()

	for _, server := range servers {
		if err := server.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown %s server: %w", server.name, err)
		}
	}

	return nil
}

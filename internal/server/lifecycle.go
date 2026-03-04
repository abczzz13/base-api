package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"sync"
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

type cleanupStack struct {
	funcs []func()
	once  sync.Once
}

func newCleanupStack() *cleanupStack {
	return &cleanupStack{}
}

func (c *cleanupStack) Add(cleanupFn func()) {
	if c == nil || cleanupFn == nil {
		return
	}

	c.funcs = append(c.funcs, cleanupFn)
}

func (c *cleanupStack) Run() {
	if c == nil {
		return
	}

	c.once.Do(func() {
		for _, cleanupFunc := range slices.Backward(c.funcs) {
			cleanupFunc()
		}
	})
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

		slog.InfoContext(ctx, "server listening",
			slog.String("server", servers[i].name),
			slog.String("address", listener.Addr().String()),
		)
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

func runServers(servers []managedServer) <-chan serverResult {
	resultsCh := make(chan serverResult, len(servers))
	for _, server := range servers {
		go func(name string, srv *http.Server, listener net.Listener) {
			if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				resultsCh <- serverResult{name: name, err: fmt.Errorf("serve %s server: %w", name, err)}
				return
			}

			resultsCh <- serverResult{name: name}
		}(server.name, server.server, server.listener)
	}

	return resultsCh
}

func shutdownServers(servers []managedServer) error {
	for _, server := range servers {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), defaultServerShutdownTimeout)
		if err := server.server.Shutdown(shutdownCtx); err != nil {
			shutdownCancel()
			return fmt.Errorf("shutdown %s server: %w", server.name, err)
		}
		shutdownCancel()
	}

	return nil
}

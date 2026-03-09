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

// Result is the outcome of a server goroutine.
type Result struct {
	Name string
	Err  error
}

// ManagedServer tracks a named HTTP server and its listener.
type ManagedServer struct {
	Name     string
	Server   *http.Server
	Listener net.Listener
}

// CleanupStack stores cleanup callbacks executed in reverse order once.
type CleanupStack struct {
	funcs []func()
	once  sync.Once
}

// NewCleanupStack creates a new cleanup stack.
func NewCleanupStack() *CleanupStack {
	return &CleanupStack{}
}

// Add appends a cleanup callback.
func (c *CleanupStack) Add(cleanupFn func()) {
	if c == nil || cleanupFn == nil {
		return
	}

	c.funcs = append(c.funcs, cleanupFn)
}

// Run executes cleanup callbacks once in reverse registration order.
func (c *CleanupStack) Run() {
	if c == nil {
		return
	}

	c.once.Do(func() {
		for _, cleanupFunc := range slices.Backward(c.funcs) {
			cleanupFunc()
		}
	})
}

// NewHTTPServer creates an http.Server with configured timeouts.
func NewHTTPServer(cfg config.Config, addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}
}

// BindListeners binds TCP listeners for all servers.
func BindListeners(ctx context.Context, servers []ManagedServer) error {
	for i := range servers {
		listener, err := net.Listen("tcp4", servers[i].Server.Addr)
		if err != nil {
			closeBoundListeners(servers)
			return fmt.Errorf("create %s listener: %w", servers[i].Name, err)
		}

		servers[i].Listener = listener

		slog.InfoContext(ctx, "server listening",
			slog.String("server", servers[i].Name),
			slog.String("address", listener.Addr().String()),
		)
	}

	return nil
}

func closeBoundListeners(servers []ManagedServer) {
	for i := range servers {
		if servers[i].Listener != nil {
			_ = servers[i].Listener.Close()
		}
	}
}

// RunServers starts all bound servers and returns a result channel.
func RunServers(servers []ManagedServer) <-chan Result {
	resultsCh := make(chan Result, len(servers))
	for _, server := range servers {
		go func(name string, srv *http.Server, listener net.Listener) {
			if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				resultsCh <- Result{Name: name, Err: fmt.Errorf("serve %s server: %w", name, err)}
				return
			}

			resultsCh <- Result{Name: name}
		}(server.Name, server.Server, server.Listener)
	}

	return resultsCh
}

// ShutdownServers gracefully shuts down all servers concurrently
// within a shared timeout.
func ShutdownServers(servers []ManagedServer) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultServerShutdownTimeout)
	defer cancel()

	var mu sync.Mutex
	var errs []error
	var wg sync.WaitGroup

	for _, server := range servers {
		wg.Add(1)
		go func(s ManagedServer) {
			defer wg.Done()
			if err := s.Server.Shutdown(ctx); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("shutdown %s server: %w", s.Name, err))
				mu.Unlock()
			}
		}(server)
	}

	wg.Wait()

	return errors.Join(errs...)
}

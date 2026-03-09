package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/abczzz13/base-api/internal/config"
)

func TestCleanupStackRunExecutesInReverseOrder(t *testing.T) {
	cleanup := NewCleanupStack()

	order := make([]string, 0, 3)
	cleanup.Add(func() {
		order = append(order, "first")
	})
	cleanup.Add(func() {
		order = append(order, "second")
	})
	cleanup.Add(nil)
	cleanup.Add(func() {
		order = append(order, "third")
	})

	cleanup.Run()

	if diff := cmp.Diff([]string{"third", "second", "first"}, order); diff != "" {
		t.Fatalf("cleanup order mismatch (-want +got):\n%s", diff)
	}
}

func TestCleanupStackRunExecutesOnce(t *testing.T) {
	cleanup := NewCleanupStack()

	count := 0
	cleanup.Add(func() {
		count++
	})

	cleanup.Run()
	cleanup.Run()

	if count != 1 {
		t.Fatalf("cleanup run count mismatch: want %d, got %d", 1, count)
	}
}

func TestNewHTTPServerUsesConfiguredTimeouts(t *testing.T) {
	cfg := config.Config{
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       11 * time.Second,
		WriteTimeout:      25 * time.Second,
		IdleTimeout:       45 * time.Second,
	}
	handler := http.NewServeMux()
	addr := "127.0.0.1:8080"

	srv := NewHTTPServer(cfg, addr, handler)

	if srv.Addr != addr {
		t.Fatalf("Addr mismatch: want %q, got %q", addr, srv.Addr)
	}
	if srv.Handler != handler {
		t.Fatalf("Handler mismatch: got unexpected handler")
	}
	if srv.ReadHeaderTimeout != cfg.ReadHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout mismatch: want %s, got %s", cfg.ReadHeaderTimeout, srv.ReadHeaderTimeout)
	}
	if srv.ReadTimeout != cfg.ReadTimeout {
		t.Fatalf("ReadTimeout mismatch: want %s, got %s", cfg.ReadTimeout, srv.ReadTimeout)
	}
	if srv.WriteTimeout != cfg.WriteTimeout {
		t.Fatalf("WriteTimeout mismatch: want %s, got %s", cfg.WriteTimeout, srv.WriteTimeout)
	}
	if srv.IdleTimeout != cfg.IdleTimeout {
		t.Fatalf("IdleTimeout mismatch: want %s, got %s", cfg.IdleTimeout, srv.IdleTimeout)
	}
}

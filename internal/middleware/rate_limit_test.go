package middleware

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/abczzz13/base-api/internal/logger"
	"github.com/abczzz13/base-api/internal/ratelimit"
)

func TestRateLimitRejectsRequestsAndRecordsMetrics(t *testing.T) {
	requestMetrics, _ := newTestHTTPRequestMetrics(t)

	server := "public"
	route := "getHealthz"
	method := http.MethodGet

	beforeRejected := testutil.ToFloat64(requestMetrics.httpRejectedRequestsTotal.WithLabelValues(server, method, route, RequestRejectionReasonRateLimit))

	handler := RequestMetrics(requestMetrics, RequestMetricsConfig{
		Server: server,
		RouteLabel: func(*http.Request) string {
			return route
		},
	})(RateLimit(RateLimitConfig{
		Store: ratelimit.StoreFunc(func(ctx context.Context, key string, policy ratelimit.Policy) (ratelimit.Decision, error) {
			if diff := cmp.Diff(ratelimit.Policy{RequestsPerSecond: 1, Burst: 2}, policy); diff != "" {
				t.Fatalf("policy mismatch (-want +got):\n%s", diff)
			}
			if got, want := key, server+":"+route+":192.0.2.10"; got != want {
				t.Fatalf("key mismatch: want %q, got %q", want, got)
			}
			return ratelimit.Decision{Allowed: false, RetryAfter: 1200 * time.Millisecond}, nil
		}),
		Server:         server,
		RouteLabel:     func(*http.Request) string { return route },
		BackendTimeout: 50 * time.Millisecond,
		FailOpen:       true,
		DefaultPolicy:  ratelimit.Policy{RequestsPerSecond: 1, Burst: 2},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))

	req := httptest.NewRequest(method, "/healthz", nil)
	req.RemoteAddr = "192.0.2.10:43123"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusTooManyRequests, rr.Code)
	}
	if got := rr.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After mismatch: want %q, got %q", "2", got)
	}
	if got := rr.Header().Get("RateLimit"); got != `"default";r=0;t=2` {
		t.Fatalf("RateLimit mismatch: want %q, got %q", `"default";r=0;t=2`, got)
	}
	if got := rr.Header().Get("RateLimit-Policy"); got != `"default";q=2;w=2` {
		t.Fatalf("RateLimit-Policy mismatch: want %q, got %q", `"default";q=2;w=2`, got)
	}

	afterRejected := testutil.ToFloat64(requestMetrics.httpRejectedRequestsTotal.WithLabelValues(server, method, route, RequestRejectionReasonRateLimit))
	if got := afterRejected - beforeRejected; got != 1 {
		t.Fatalf("rejected_requests_total delta mismatch: want 1, got %v", got)
	}
}

func TestRateLimitFailsOpenAndRecordsBackendErrors(t *testing.T) {
	requestMetrics, _ := newTestHTTPRequestMetrics(t)

	server := "public"
	route := "getHealthz"
	method := http.MethodGet

	beforeErrors := testutil.ToFloat64(requestMetrics.httpRateLimitErrorsTotal.WithLabelValues(server, method, route))

	handler := RequestMetrics(requestMetrics, RequestMetricsConfig{
		Server: server,
		RouteLabel: func(*http.Request) string {
			return route
		},
	})(RateLimit(RateLimitConfig{
		Store: ratelimit.StoreFunc(func(context.Context, string, ratelimit.Policy) (ratelimit.Decision, error) {
			return ratelimit.Decision{}, errors.New("valkey unavailable")
		}),
		Server:         server,
		RouteLabel:     func(*http.Request) string { return route },
		BackendTimeout: 50 * time.Millisecond,
		FailOpen:       true,
		DefaultPolicy:  ratelimit.Policy{RequestsPerSecond: 1, Burst: 2},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))

	req := httptest.NewRequest(method, "/healthz", nil)
	req.RemoteAddr = "192.0.2.10:43123"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusNoContent, rr.Code)
	}

	afterErrors := testutil.ToFloat64(requestMetrics.httpRateLimitErrorsTotal.WithLabelValues(server, method, route))
	if got := afterErrors - beforeErrors; got != 1 {
		t.Fatalf("rate_limit_backend_errors_total delta mismatch: want 1, got %v", got)
	}
}

func TestRateLimitBackendFailureReturnsServiceUnavailableAndSeparateRejectionReason(t *testing.T) {
	requestMetrics, _ := newTestHTTPRequestMetrics(t)

	server := "public"
	route := "getHealthz"
	method := http.MethodGet

	beforeErrors := testutil.ToFloat64(requestMetrics.httpRateLimitErrorsTotal.WithLabelValues(server, method, route))
	beforeRejected := testutil.ToFloat64(requestMetrics.httpRejectedRequestsTotal.WithLabelValues(server, method, route, RequestRejectionReasonRateLimitBackendFailure))

	handler := RequestMetrics(requestMetrics, RequestMetricsConfig{
		Server: server,
		RouteLabel: func(*http.Request) string {
			return route
		},
	})(RateLimit(RateLimitConfig{
		Store: ratelimit.StoreFunc(func(context.Context, string, ratelimit.Policy) (ratelimit.Decision, error) {
			return ratelimit.Decision{}, errors.New("valkey unavailable")
		}),
		Server:         server,
		RouteLabel:     func(*http.Request) string { return route },
		BackendTimeout: 50 * time.Millisecond,
		FailOpen:       false,
		DefaultPolicy:  ratelimit.Policy{RequestsPerSecond: 1, Burst: 2},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))

	req := httptest.NewRequest(method, "/healthz", nil)
	req.RemoteAddr = "192.0.2.10:43123"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
	if got := rr.Header().Get("Retry-After"); got != "" {
		t.Fatalf("Retry-After mismatch: want empty, got %q", got)
	}
	if got := rr.Header().Get("RateLimit"); got != "" {
		t.Fatalf("RateLimit mismatch: want empty, got %q", got)
	}
	if got := rr.Header().Get("RateLimit-Policy"); got != "" {
		t.Fatalf("RateLimit-Policy mismatch: want empty, got %q", got)
	}

	afterErrors := testutil.ToFloat64(requestMetrics.httpRateLimitErrorsTotal.WithLabelValues(server, method, route))
	if got := afterErrors - beforeErrors; got != 1 {
		t.Fatalf("rate_limit_backend_errors_total delta mismatch: want 1, got %v", got)
	}

	afterRejected := testutil.ToFloat64(requestMetrics.httpRejectedRequestsTotal.WithLabelValues(server, method, route, RequestRejectionReasonRateLimitBackendFailure))
	if got := afterRejected - beforeRejected; got != 1 {
		t.Fatalf("rejected_requests_total delta mismatch: want 1, got %v", got)
	}
}

func TestRateLimitBypassesPreflightRequests(t *testing.T) {
	handler := RateLimit(RateLimitConfig{
		Store: ratelimit.StoreFunc(func(context.Context, string, ratelimit.Policy) (ratelimit.Decision, error) {
			return ratelimit.Decision{Allowed: false}, nil
		}),
		DefaultPolicy: ratelimit.Policy{RequestsPerSecond: 1, Burst: 1},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/healthz", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusNoContent, rr.Code)
	}
}

func TestRateLimitStartupFallbackSuppressesPerRequestWarnings(t *testing.T) {
	previousLogger := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	var logs bytes.Buffer
	logger.New(logger.Config{
		Format:      logger.FormatJSON,
		Level:       slog.LevelWarn,
		Environment: "test",
		Writer:      &logs,
	})

	handler := RateLimit(RateLimitConfig{
		Store: ratelimit.StoreFunc(func(context.Context, string, ratelimit.Policy) (ratelimit.Decision, error) {
			return ratelimit.Decision{}, fmt.Errorf("%w: valkey unavailable", ratelimit.ErrStartupBackendUnavailable)
		}),
		Server:        "public",
		RouteLabel:    func(*http.Request) string { return "getHealthz" },
		FailOpen:      true,
		DefaultPolicy: ratelimit.Policy{RequestsPerSecond: 1, Burst: 1},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.RemoteAddr = "192.0.2.10:43123"
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Fatalf("status mismatch: want %d, got %d", http.StatusNoContent, rr.Code)
		}
	}

	if strings.Contains(logs.String(), "rate limit check failed") {
		t.Fatalf("expected startup fallback to suppress per-request warnings, got logs %q", logs.String())
	}
}

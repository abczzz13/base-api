package server

import (
	"context"
	"strings"
	"testing"

	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/ratelimit"
)

func TestSetupRateLimiterDisabledReturnsNilStore(t *testing.T) {
	store, cleanup, err := setupRateLimiter(context.Background(), config.RateLimitConfig{})
	if err != nil {
		t.Fatalf("setupRateLimiter returned error: %v", err)
	}
	if store != nil {
		t.Fatalf("expected nil store, got %T", store)
	}
	cleanup()
}

func TestSetupRateLimiterFailOpenReturnsUnavailableStoreWhenBackendInitFails(t *testing.T) {
	store, cleanup, err := setupRateLimiter(context.Background(), config.RateLimitConfig{
		Enabled:  true,
		FailOpen: true,
		Valkey: ratelimit.ValkeyConfig{
			Mode:  ratelimit.ValkeyModeStandalone,
			Addrs: []string{reserveTCPAddress(t)},
		},
	})
	if err != nil {
		t.Fatalf("setupRateLimiter returned error: %v", err)
	}
	t.Cleanup(cleanup)
	if store == nil {
		t.Fatal("expected non-nil fallback store")
	}

	_, allowErr := store.Allow(context.Background(), "public:getHealthz:192.0.2.10", ratelimit.Policy{RequestsPerSecond: 1, Burst: 1})
	if allowErr == nil {
		t.Fatal("expected fallback store to surface backend error")
	}
	if !strings.Contains(allowErr.Error(), "configure rate limit Valkey client") {
		t.Fatalf("unexpected fallback error: %v", allowErr)
	}
}

func TestSetupRateLimiterFailClosedReturnsErrorWhenBackendInitFails(t *testing.T) {
	store, cleanup, err := setupRateLimiter(context.Background(), config.RateLimitConfig{
		Enabled:  true,
		FailOpen: false,
		Valkey: ratelimit.ValkeyConfig{
			Mode:  ratelimit.ValkeyModeStandalone,
			Addrs: []string{reserveTCPAddress(t)},
		},
	})
	cleanup()
	if store != nil {
		t.Fatalf("expected nil store, got %T", store)
	}
	if err == nil {
		t.Fatal("expected setupRateLimiter error")
	}
	if !strings.Contains(err.Error(), "configure rate limit Valkey client") {
		t.Fatalf("unexpected error: %v", err)
	}
}

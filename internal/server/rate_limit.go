package server

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/ratelimit"
)

func setupRateLimiter(ctx context.Context, cfg config.RateLimitConfig) (ratelimit.Store, func(), error) {
	if !cfg.IsEnabled() {
		return nil, func() {}, nil
	}

	rateLimitClient, err := ratelimit.NewValkeyClient(cfg.Valkey)
	if err != nil {
		return handleRateLimiterSetupError(ctx, cfg, fmt.Errorf("configure rate limit Valkey client: %w", err))
	}

	store, err := ratelimit.NewValkeyStore(ratelimit.ValkeyStoreConfig{
		Client:    rateLimitClient,
		KeyPrefix: cfg.Valkey.KeyPrefix,
	})
	if err != nil {
		rateLimitClient.Close()
		return handleRateLimiterSetupError(ctx, cfg, fmt.Errorf("configure rate limiter: %w", err))
	}

	return store, func() {
		rateLimitClient.Close()
	}, nil
}

func handleRateLimiterSetupError(ctx context.Context, cfg config.RateLimitConfig, err error) (ratelimit.Store, func(), error) {
	if !cfg.FailOpen {
		return nil, func() {}, err
	}

	slog.WarnContext(ctx, "rate limiter backend unavailable; starting in fail-open mode",
		slog.Any("error", err),
		slog.String("mode", string(cfg.Valkey.NormalizedMode())),
		slog.Int("address_count", len(cfg.Valkey.Addrs)),
	)

	return newUnavailableRateLimiter(err), func() {}, nil
}

func newUnavailableRateLimiter(err error) ratelimit.Store {
	wrappedErr := fmt.Errorf("%w: %w", ratelimit.ErrStartupBackendUnavailable, err)

	return ratelimit.StoreFunc(func(context.Context, string, ratelimit.Policy) (ratelimit.Decision, error) {
		return ratelimit.Decision{}, wrappedErr
	})
}

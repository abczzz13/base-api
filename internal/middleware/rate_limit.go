package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/abczzz13/base-api/internal/apierrors"
	"github.com/abczzz13/base-api/internal/ratelimit"
)

const (
	RequestRejectionReasonRateLimit               = "rate_limit"
	RequestRejectionReasonRateLimitBackendFailure = "rate_limit_backend_failure"
)

type RateLimitConfig struct {
	ClientIPResolver  *ClientIPResolver
	Store             ratelimit.Store
	Server            string
	RouteLabel        func(*http.Request) string
	BackendTimeout    time.Duration
	FailOpen          bool
	DefaultPolicy     ratelimit.Policy
	RouteOverrides    map[string]ratelimit.RouteOverride
	TrustedProxyCIDRs []netip.Prefix
}

func RateLimit(cfg RateLimitConfig) func(http.Handler) http.Handler {
	if cfg.Store == nil {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	server := requestMetricsServerLabel(cfg.Server)
	routeLabel := cfg.RouteLabel
	if routeLabel == nil {
		routeLabel = func(*http.Request) string {
			return RequestMetricsRouteUnmatched
		}
	}

	clientIPResolver := cfg.ClientIPResolver
	if clientIPResolver == nil {
		clientIPResolver = NewClientIPResolver("rate limit", cfg.TrustedProxyCIDRs)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r == nil || strings.EqualFold(r.Method, http.MethodOptions) {
				next.ServeHTTP(w, r)
				return
			}

			route := requestMetricsRouteLabel(routeLabel(r))
			policy, enabled := ratelimit.ResolvePolicy(cfg.DefaultPolicy, cfg.RouteOverrides, route)
			if !enabled {
				next.ServeHTTP(w, r)
				return
			}

			r, clientIP := clientIPResolver.ResolvePreferred(r)
			if clientIP == "" {
				next.ServeHTTP(w, r)
				return
			}

			allowCtx, cancel := rateLimitContext(r.Context(), cfg.BackendTimeout)
			decision, err := cfg.Store.Allow(allowCtx, rateLimitKey(server, route, clientIP), policy)
			cancel()
			if err != nil {
				observeRateLimitBackendError(r)
				if !errors.Is(err, ratelimit.ErrStartupBackendUnavailable) {
					slog.WarnContext(r.Context(), "rate limit check failed",
						slog.String("server", server),
						slog.String("route", route),
						slog.String("client_ip", clientIP),
						slog.Any("error", err),
					)
				}
				if cfg.FailOpen {
					next.ServeHTTP(w, r)
					return
				}

				observeRejectedRequest(r, RequestRejectionReasonRateLimitBackendFailure)
				apierrors.New(http.StatusServiceUnavailable, "rate_limit_unavailable", "rate limit backend unavailable").WithContext(r.Context()).WritePublicServiceUnavailable(w)
				return
			}

			if decision.Allowed {
				next.ServeHTTP(w, r)
				return
			}

			observeRejectedRequest(r, RequestRejectionReasonRateLimit)
			headers := ratelimit.HeaderValues(policy, decision)
			apierrors.New(http.StatusTooManyRequests, "too_many_requests", "rate limit exceeded").WithContext(r.Context()).WritePublicTooManyRequests(w, apierrors.TooManyRequestsHeaders{
				RetryAfter:      headers.RetryAfter,
				RateLimit:       headers.RateLimit,
				RateLimitPolicy: headers.RateLimitPolicy,
			})
		})
	}
}

func rateLimitContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}

	return context.WithTimeout(ctx, timeout)
}

func rateLimitKey(server, route, clientIP string) string {
	return server + ":" + route + ":" + clientIP
}

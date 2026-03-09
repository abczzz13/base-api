package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"
)

var ErrStartupBackendUnavailable = errors.New("rate limiter backend unavailable during startup")

const headerPolicyName = "default"

type Policy struct {
	RequestsPerSecond float64
	Burst             int
}

func (p Policy) Validate() error {
	if p.RequestsPerSecond <= 0 {
		return fmt.Errorf("requestsPerSecond must be greater than 0")
	}
	if p.Burst <= 0 {
		return fmt.Errorf("burst must be greater than 0")
	}

	return nil
}

type RouteOverride struct {
	Disabled          bool     `json:"disabled,omitempty"`
	RequestsPerSecond *float64 `json:"requestsPerSecond,omitempty"`
	Burst             *int     `json:"burst,omitempty"`
}

func (o RouteOverride) Validate() error {
	if o.Disabled {
		if o.RequestsPerSecond != nil || o.Burst != nil {
			return fmt.Errorf("disabled overrides cannot set requestsPerSecond or burst")
		}

		return nil
	}

	if o.RequestsPerSecond == nil && o.Burst == nil {
		return fmt.Errorf("override must set disabled, requestsPerSecond, or burst")
	}

	if o.RequestsPerSecond != nil && *o.RequestsPerSecond <= 0 {
		return fmt.Errorf("requestsPerSecond must be greater than 0")
	}
	if o.Burst != nil && *o.Burst <= 0 {
		return fmt.Errorf("burst must be greater than 0")
	}

	return nil
}

func ResolvePolicy(defaultPolicy Policy, overrides map[string]RouteOverride, route string) (Policy, bool) {
	policy := defaultPolicy

	override, ok := overrides[route]
	if !ok {
		return policy, true
	}

	if override.Disabled {
		return Policy{}, false
	}
	if override.RequestsPerSecond != nil {
		policy.RequestsPerSecond = *override.RequestsPerSecond
	}
	if override.Burst != nil {
		policy.Burst = *override.Burst
	}

	return policy, true
}

type Decision struct {
	Allowed    bool
	Remaining  int
	RetryAfter time.Duration
}

type ResponseHeaders struct {
	RetryAfter      string
	RateLimit       string
	RateLimitPolicy string
}

func HeaderValues(policy Policy, decision Decision) ResponseHeaders {
	retryAfterSeconds := delaySeconds(decision.RetryAfter)

	return ResponseHeaders{
		RetryAfter:      strconv.Itoa(retryAfterSeconds),
		RateLimit:       fmt.Sprintf("%q;r=%d;t=%d", headerPolicyName, max(0, decision.Remaining), retryAfterSeconds),
		RateLimitPolicy: fmt.Sprintf("%q;q=%d;w=%d", headerPolicyName, max(0, policy.Burst), policyWindowSeconds(policy)),
	}
}

func delaySeconds(delay time.Duration) int {
	if delay <= 0 {
		return 1
	}

	return max(1, int(math.Ceil(delay.Seconds())))
}

func policyWindowSeconds(policy Policy) int {
	if err := policy.Validate(); err != nil {
		return 1
	}

	seconds := int(math.Ceil(float64(policy.Burst) / policy.RequestsPerSecond))
	if seconds < 1 {
		seconds = 1
	}

	return seconds
}

type Store interface {
	Allow(context.Context, string, Policy) (Decision, error)
}

type StoreFunc func(context.Context, string, Policy) (Decision, error)

func (f StoreFunc) Allow(ctx context.Context, key string, policy Policy) (Decision, error) {
	if f == nil {
		return Decision{Allowed: true}, nil
	}

	return f(ctx, key, policy)
}

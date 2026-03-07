package ratelimit

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestPolicyValidate(t *testing.T) {
	tests := []struct {
		name    string
		policy  Policy
		wantErr string
	}{
		{
			name:   "valid",
			policy: Policy{RequestsPerSecond: 2.5, Burst: 4},
		},
		{
			name:    "non-positive requests per second",
			policy:  Policy{RequestsPerSecond: 0, Burst: 4},
			wantErr: "requestsPerSecond must be greater than 0",
		},
		{
			name:    "non-positive burst",
			policy:  Policy{RequestsPerSecond: 1, Burst: 0},
			wantErr: "burst must be greater than 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate returned error: %v", err)
				}
				return
			}

			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("Validate error mismatch: want %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestRouteOverrideValidate(t *testing.T) {
	tests := []struct {
		name     string
		override RouteOverride
		wantErr  string
	}{
		{
			name:     "disabled",
			override: RouteOverride{Disabled: true},
		},
		{
			name:     "partial override",
			override: RouteOverride{RequestsPerSecond: float64Ptr(3.5)},
		},
		{
			name:     "full override",
			override: RouteOverride{RequestsPerSecond: float64Ptr(3.5), Burst: intPtr(7)},
		},
		{
			name:     "disabled with extra fields",
			override: RouteOverride{Disabled: true, Burst: intPtr(1)},
			wantErr:  "disabled overrides cannot set requestsPerSecond or burst",
		},
		{
			name:     "empty override",
			override: RouteOverride{},
			wantErr:  "override must set disabled, requestsPerSecond, or burst",
		},
		{
			name:     "non-positive requests per second",
			override: RouteOverride{RequestsPerSecond: float64Ptr(0)},
			wantErr:  "requestsPerSecond must be greater than 0",
		},
		{
			name:     "non-positive burst",
			override: RouteOverride{Burst: intPtr(0)},
			wantErr:  "burst must be greater than 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.override.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate returned error: %v", err)
				}
				return
			}

			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("Validate error mismatch: want %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestResolvePolicy(t *testing.T) {
	defaultPolicy := Policy{RequestsPerSecond: 5, Burst: 10}

	tests := []struct {
		name      string
		overrides map[string]RouteOverride
		route     string
		want      Policy
		wantOK    bool
	}{
		{
			name:   "uses default policy when no override exists",
			route:  "getHealthz",
			want:   defaultPolicy,
			wantOK: true,
		},
		{
			name: "disables route when override is disabled",
			overrides: map[string]RouteOverride{
				"getHealthz": {Disabled: true},
			},
			route:  "getHealthz",
			want:   Policy{},
			wantOK: false,
		},
		{
			name: "overrides only provided fields",
			overrides: map[string]RouteOverride{
				"getHealthz": {RequestsPerSecond: float64Ptr(2.5)},
			},
			route:  "getHealthz",
			want:   Policy{RequestsPerSecond: 2.5, Burst: 10},
			wantOK: true,
		},
		{
			name: "overrides both fields",
			overrides: map[string]RouteOverride{
				"getHealthz": {RequestsPerSecond: float64Ptr(2.5), Burst: intPtr(4)},
			},
			route:  "getHealthz",
			want:   Policy{RequestsPerSecond: 2.5, Burst: 4},
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotOK := ResolvePolicy(defaultPolicy, tt.overrides, tt.route)
			if gotOK != tt.wantOK {
				t.Fatalf("ResolvePolicy enabled mismatch: want %t, got %t", tt.wantOK, gotOK)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("ResolvePolicy policy mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestHeaderValues(t *testing.T) {
	tests := []struct {
		name     string
		policy   Policy
		decision Decision
		want     ResponseHeaders
	}{
		{
			name:     "formats retry and quota headers",
			policy:   Policy{RequestsPerSecond: 2.5, Burst: 4},
			decision: Decision{Allowed: false, Remaining: 0, RetryAfter: 1200 * time.Millisecond},
			want: ResponseHeaders{
				RetryAfter:      "2",
				RateLimit:       `"default";r=0;t=2`,
				RateLimitPolicy: `"default";q=4;w=2`,
			},
		},
		{
			name:     "clamps invalid values to a minimal delay",
			policy:   Policy{},
			decision: Decision{Allowed: false, Remaining: -1, RetryAfter: 0},
			want: ResponseHeaders{
				RetryAfter:      "1",
				RateLimit:       `"default";r=0;t=1`,
				RateLimitPolicy: `"default";q=0;w=1`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HeaderValues(tt.policy, tt.decision)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("HeaderValues mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func float64Ptr(value float64) *float64 {
	return &value
}

func intPtr(value int) *int {
	return &value
}

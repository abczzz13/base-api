package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	valkey "github.com/valkey-io/valkey-go"
)

func TestNewValkeyStoreRequiresClient(t *testing.T) {
	_, err := NewValkeyStore(ValkeyStoreConfig{})
	if err == nil {
		t.Fatal("expected nil client error")
	}
	if got := err.Error(); got != "valkey client is required" {
		t.Fatalf("error mismatch: want %q, got %q", "valkey client is required", got)
	}
}

func TestValkeyStoreAllowBuildsBucketKeyAndArgs(t *testing.T) {
	const key = "public:getHealthz:192.0.2.10"
	now := time.UnixMilli(1700000000123)
	store := &ValkeyStore{
		keyPrefix: "custom-prefix",
		now:       func() time.Time { return now },
		eval: func(ctx context.Context, client valkey.Client, script *valkey.Lua, bucketKey string, args []string) (Decision, error) {
			_ = ctx
			_ = client
			_ = script

			hash := sha256.Sum256([]byte(key))
			wantBucketKey := "custom-prefix:" + hex.EncodeToString(hash[:])
			if bucketKey != wantBucketKey {
				t.Fatalf("bucketKey mismatch: want %q, got %q", wantBucketKey, bucketKey)
			}

			wantArgs := []string{"1700000000123", "2.5", "4"}
			if got, want := strings.Join(args, ","), strings.Join(wantArgs, ","); got != want {
				t.Fatalf("args mismatch: want %q, got %q", want, got)
			}

			return Decision{Allowed: false, Remaining: 0, RetryAfter: 1500 * time.Millisecond}, nil
		},
	}

	decision, err := store.Allow(context.Background(), key, Policy{RequestsPerSecond: 2.5, Burst: 4})
	if err != nil {
		t.Fatalf("Allow returned error: %v", err)
	}
	if !cmpDecision(decision, Decision{Allowed: false, Remaining: 0, RetryAfter: 1500 * time.Millisecond}) {
		t.Fatalf("decision mismatch: got %#v", decision)
	}
}

func TestValkeyStoreAllowRejectsInvalidPolicy(t *testing.T) {
	store := &ValkeyStore{}

	_, err := store.Allow(context.Background(), "public:getHealthz:192.0.2.10", Policy{})
	if err == nil {
		t.Fatal("expected policy validation error")
	}
	if !strings.Contains(err.Error(), "validate policy") {
		t.Fatalf("error mismatch: got %v", err)
	}
}

func TestIdleTTL(t *testing.T) {
	tests := []struct {
		name   string
		policy Policy
		want   time.Duration
	}{
		{
			name:   "uses twice the time to refill the bucket",
			policy: Policy{RequestsPerSecond: 2, Burst: 6},
			want:   6 * time.Second,
		},
		{
			name:   "floors to minimum ttl",
			policy: Policy{RequestsPerSecond: 100, Burst: 1},
			want:   time.Second,
		},
		{
			name:   "invalid policy returns minimum ttl",
			policy: Policy{},
			want:   time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IdleTTL(tt.policy); got != tt.want {
				t.Fatalf("IdleTTL mismatch: want %s, got %s", tt.want, got)
			}
		})
	}
}

func cmpDecision(a, b Decision) bool {
	return a.Allowed == b.Allowed && a.Remaining == b.Remaining && a.RetryAfter == b.RetryAfter
}

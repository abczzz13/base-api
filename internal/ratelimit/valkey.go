package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	valkey "github.com/valkey-io/valkey-go"
)

const (
	DefaultKeyPrefix = "base-api:ratelimit"
	minKeyTTL        = time.Second
)

//nolint:gosec // Lua script contents are not credentials.
const tokenBucketScript = `
local key = KEYS[1]
local now_ms = tonumber(ARGV[1])
local refill_per_second = tonumber(ARGV[2])
local capacity = tonumber(ARGV[3])

local bucket = redis.call('HMGET', key, 'tokens', 'updated_at_ms')
local tokens = tonumber(bucket[1])
local updated_at_ms = tonumber(bucket[2])

if tokens == nil then
  tokens = capacity
end
if updated_at_ms == nil or updated_at_ms > now_ms then
  updated_at_ms = now_ms
end

local elapsed_ms = now_ms - updated_at_ms
local refilled = (elapsed_ms / 1000.0) * refill_per_second
tokens = math.min(capacity, tokens + refilled)

local allowed = 0
local retry_after_ms = 0
if tokens >= 1 then
  allowed = 1
  tokens = tokens - 1
else
  local shortfall = 1 - tokens
  retry_after_ms = math.ceil((shortfall / refill_per_second) * 1000)
end

local ttl_ms = math.max(math.ceil((capacity / refill_per_second) * 2000), 1000)

redis.call('HSET', key, 'tokens', tokens, 'updated_at_ms', now_ms)
redis.call('PEXPIRE', key, ttl_ms)

return {allowed, math.floor(tokens), retry_after_ms}
`

type ValkeyStoreConfig struct {
	Client    valkey.Client
	KeyPrefix string
	Now       func() time.Time
}

type ValkeyStore struct {
	client    valkey.Client
	script    *valkey.Lua
	keyPrefix string
	now       func() time.Time
	eval      tokenBucketEvalFunc
}

type tokenBucketEvalFunc func(context.Context, valkey.Client, *valkey.Lua, string, []string) (Decision, error)

func NewValkeyStore(cfg ValkeyStoreConfig) (*ValkeyStore, error) {
	if cfg.Client == nil {
		return nil, errors.New("valkey client is required")
	}

	keyPrefix := strings.TrimSpace(cfg.KeyPrefix)
	if keyPrefix == "" {
		keyPrefix = DefaultKeyPrefix
	}

	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	return &ValkeyStore{
		client:    cfg.Client,
		script:    valkey.NewLuaScript(tokenBucketScript),
		keyPrefix: keyPrefix,
		now:       now,
		eval:      execTokenBucketScript,
	}, nil
}

func (s *ValkeyStore) Allow(ctx context.Context, key string, policy Policy) (Decision, error) {
	if s == nil {
		return Decision{Allowed: true}, nil
	}
	if err := policy.Validate(); err != nil {
		return Decision{}, fmt.Errorf("validate policy: %w", err)
	}

	bucketKey := s.bucketKey(key)
	args := []string{
		strconv.FormatInt(s.now().UnixMilli(), 10),
		strconv.FormatFloat(policy.RequestsPerSecond, 'f', -1, 64),
		strconv.Itoa(policy.Burst),
	}

	decision, err := s.eval(ctx, s.client, s.script, bucketKey, args)
	if err != nil {
		return Decision{}, err
	}

	return decision, nil
}

func execTokenBucketScript(ctx context.Context, client valkey.Client, script *valkey.Lua, bucketKey string, args []string) (Decision, error) {
	result, err := script.Exec(ctx, client, []string{bucketKey}, args).ToArray()
	if err != nil {
		return Decision{}, fmt.Errorf("execute token bucket script: %w", err)
	}
	if len(result) != 3 {
		return Decision{}, fmt.Errorf("unexpected token bucket response length %d", len(result))
	}

	allowedValue, err := result[0].ToInt64()
	if err != nil {
		return Decision{}, fmt.Errorf("parse allowed response: %w", err)
	}
	remainingValue, err := result[1].ToInt64()
	if err != nil {
		return Decision{}, fmt.Errorf("parse remaining response: %w", err)
	}
	retryAfterMilliseconds, err := result[2].ToInt64()
	if err != nil {
		return Decision{}, fmt.Errorf("parse retry-after response: %w", err)
	}

	return Decision{
		Allowed:    allowedValue == 1,
		Remaining:  max(0, int(remainingValue)),
		RetryAfter: time.Duration(max(int64(0), retryAfterMilliseconds)) * time.Millisecond,
	}, nil
}

func (s *ValkeyStore) bucketKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return s.keyPrefix + ":" + hex.EncodeToString(hash[:])
}

func IdleTTL(policy Policy) time.Duration {
	if err := policy.Validate(); err != nil {
		return minKeyTTL
	}

	secondsToFull := float64(policy.Burst) / policy.RequestsPerSecond
	ttl := time.Duration(math.Ceil(secondsToFull * 2 * float64(time.Second)))

	return max(ttl, minKeyTTL)
}

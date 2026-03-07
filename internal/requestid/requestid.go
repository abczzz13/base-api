package requestid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	HeaderName = "X-Request-Id"
	maxLength  = 128
)

type contextKey struct{}

// New returns a validated request ID, preserving a trusted inbound value when
// possible and generating a new one otherwise.
func New(value string) string {
	if normalized, ok := Normalize(value); ok {
		return normalized
	}

	return Generate()
}

// Normalize trims and validates a request ID value.
func Normalize(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len(trimmed) > maxLength {
		return "", false
	}

	for _, r := range trimmed {
		if !allowedRune(r) {
			return "", false
		}
	}

	return trimmed, true
}

// Generate creates a new request ID.
func Generate() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return hex.EncodeToString(bytes[:])
	}

	return fmt.Sprintf("fallback-%d", time.Now().UTC().UnixNano())
}

// WithContext stores requestID in ctx.
func WithContext(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	if normalized, ok := Normalize(requestID); ok {
		return context.WithValue(ctx, contextKey{}, normalized)
	}

	return ctx
}

// FromContext returns the request ID stored in ctx, if present.
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	requestID, _ := ctx.Value(contextKey{}).(string)
	if normalized, ok := Normalize(requestID); ok {
		return normalized
	}

	return ""
}

func allowedRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	}

	switch r {
	case '-', '.', '_', '~', '/', '+', '=', ':', '@':
		return true
	default:
		return false
	}
}

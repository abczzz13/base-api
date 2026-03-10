package asyncaudit

import (
	"context"
	"encoding/json"
	"fmt"
)

// MarshalHeaders encodes HTTP headers to JSON for database persistence.
func MarshalHeaders(headers map[string][]string) ([]byte, error) {
	if headers == nil {
		return []byte("{}"), nil
	}

	encoded, err := json.Marshal(headers)
	if err != nil {
		return nil, fmt.Errorf("marshal audit headers: %w", err)
	}

	return encoded, nil
}

// JSONColumn returns nil for empty byte slices, preserving database NULL semantics.
func JSONColumn(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}

	return value
}

// NormalizeSizeBytes clamps negative byte counts to zero.
func NormalizeSizeBytes(value int64) int64 {
	return max(value, 0)
}

// ContextOrBackground returns ctx if non-nil, otherwise context.Background().
func ContextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}

	return ctx
}

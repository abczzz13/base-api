package notes

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrInvalidCursor = errors.New("invalid cursor")

// Cursor identifies the last item returned in a note page.
type Cursor struct {
	CreatedAt time.Time `json:"createdAt"`
	ID        uuid.UUID `json:"id"`
}

func encodeCursor(cursor Cursor) (string, error) {
	if cursor.CreatedAt.IsZero() || cursor.ID == uuid.Nil {
		return "", fmt.Errorf("%w: missing cursor fields", ErrInvalidCursor)
	}

	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("marshal cursor: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeCursor(raw string) (Cursor, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Cursor{}, fmt.Errorf("%w: empty cursor", ErrInvalidCursor)
	}

	payload, err := base64.RawURLEncoding.DecodeString(trimmed)
	if err != nil {
		return Cursor{}, fmt.Errorf("%w: decode cursor: %w", ErrInvalidCursor, err)
	}

	var cursor Cursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return Cursor{}, fmt.Errorf("%w: unmarshal cursor: %w", ErrInvalidCursor, err)
	}
	if cursor.CreatedAt.IsZero() || cursor.ID == uuid.Nil {
		return Cursor{}, fmt.Errorf("%w: invalid cursor payload", ErrInvalidCursor)
	}

	return cursor, nil
}

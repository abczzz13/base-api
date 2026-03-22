package notes

import (
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
)

func TestCursorRoundTrip(t *testing.T) {
	want := Cursor{
		CreatedAt: time.Date(2026, time.March, 21, 12, 34, 56, 0, time.UTC),
		ID:        uuid.MustParse("0195b1d3-e65d-779b-a3e9-6f3a25ed42c1"),
	}

	encoded, err := encodeCursor(want)
	if err != nil {
		t.Fatalf("encodeCursor returned error: %v", err)
	}

	got, err := decodeCursor(encoded)
	if err != nil {
		t.Fatalf("decodeCursor returned error: %v", err)
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("cursor mismatch (-want +got):\n%s", diff)
	}
}

func TestDecodeCursorRejectsInvalidValue(t *testing.T) {
	_, err := decodeCursor("not-a-valid-cursor")
	if !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("expected ErrInvalidCursor, got %v", err)
	}
}

package requestid

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
)

func TestGenerateReturnsUUIDv7(t *testing.T) {
	generated := Generate()

	parsed, err := uuid.Parse(generated)
	if err != nil {
		t.Fatalf("Parse(%q) returned error: %v", generated, err)
	}
	if diff := cmp.Diff(uuid.Version(7), parsed.Version()); diff != "" {
		t.Fatalf("UUID version mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(uuid.RFC4122, parsed.Variant()); diff != "" {
		t.Fatalf("UUID variant mismatch (-want +got):\n%s", diff)
	}
}

func TestNewPreservesValidInboundValueOrGeneratesUUIDv7(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		want         string
		wantPreserve bool
	}{
		{name: "preserve valid value", input: "client-req-123", want: "client-req-123", wantPreserve: true},
		{name: "missing value"},
		{name: "invalid value", input: "bad value with spaces"},
		{name: "too long value", input: strings.Repeat("a", maxLength+1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := New(tt.input)

			if tt.wantPreserve {
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Fatalf("request ID mismatch (-want +got):\n%s", diff)
				}
				return
			}

			parsed, err := uuid.Parse(got)
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", got, err)
			}
			if diff := cmp.Diff(uuid.Version(7), parsed.Version()); diff != "" {
				t.Fatalf("UUID version mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

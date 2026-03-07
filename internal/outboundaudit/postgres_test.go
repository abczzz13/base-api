package outboundaudit

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestNormalizeStatusCode(t *testing.T) {
	tests := []struct {
		name  string
		value int
		want  int32
	}{
		{
			name:  "zero is preserved for transport errors",
			value: 0,
			want:  0,
		},
		{
			name:  "below range is normalized to zero",
			value: 99,
			want:  0,
		},
		{
			name:  "minimum valid code is preserved",
			value: 100,
			want:  100,
		},
		{
			name:  "known status code is preserved",
			value: 201,
			want:  201,
		},
		{
			name:  "highest valid non-standard status code is preserved",
			value: 599,
			want:  599,
		},
		{
			name:  "above range is capped",
			value: 700,
			want:  599,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, normalizeStatusCode(tt.value)); diff != "" {
				t.Fatalf("normalizeStatusCode mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

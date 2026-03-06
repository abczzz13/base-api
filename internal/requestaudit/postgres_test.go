package requestaudit

import (
	"net/http"
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
			name:  "below range defaults to internal server error",
			value: 99,
			want:  http.StatusInternalServerError,
		},
		{
			name:  "minimum valid code is preserved",
			value: http.StatusContinue,
			want:  http.StatusContinue,
		},
		{
			name:  "known status code is preserved",
			value: http.StatusCreated,
			want:  http.StatusCreated,
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

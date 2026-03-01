package server

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abczzz13/base-api/internal/config"

	"github.com/abczzz13/base-api/internal/oas"
)

func TestBaseServiceGetHealthz(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		want *oas.HealthResponse
	}{
		{
			name: "returns public safe health response",
			cfg:  config.Config{Environment: "production"},
			want: &oas.HealthResponse{Status: "OK"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newBaseService(tt.cfg)
			got, err := svc.GetHealthz(context.Background())
			if err != nil {
				t.Fatalf("GetHealthz returned error: %v", err)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("GetHealthz mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBaseServiceNewError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want *oas.DefaultErrorStatusCode
	}{
		{
			name: "maps unexpected error to internal response",
			err:  errors.New("boom"),
			want: &oas.DefaultErrorStatusCode{
				StatusCode: 500,
				Response: oas.ErrorResponse{
					Code:    "internal_error",
					Message: "internal server error",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newBaseService(config.Config{})
			got := svc.NewError(context.Background(), tt.err)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("NewError mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

package publicapi_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/publicapi"

	"github.com/abczzz13/base-api/internal/publicoas"
)

func TestBaseServiceGetHealthz(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		want *publicoas.HealthResponseHeaders
	}{
		{
			name: "returns public safe health response",
			cfg:  config.Config{Environment: "production"},
			want: &publicoas.HealthResponseHeaders{Response: publicoas.HealthResponse{Status: "OK"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := publicapi.NewService(tt.cfg)
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
		want *publicoas.DefaultErrorStatusCodeWithHeaders
	}{
		{
			name: "maps unexpected error to internal response",
			err:  errors.New("boom"),
			want: &publicoas.DefaultErrorStatusCodeWithHeaders{
				StatusCode: 500,
				Response: publicoas.ErrorResponse{
					Code:    "internal_error",
					Message: "internal server error",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := publicapi.NewService(config.Config{})
			got := svc.NewError(context.Background(), tt.err)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("NewError mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

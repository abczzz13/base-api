package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/abczzz13/base-api/internal/infraoas"
)

func TestInfraServiceGetLivez(t *testing.T) {
	tests := []struct {
		name string
		want *infraoas.ProbeResponse
	}{
		{
			name: "returns alive response",
			want: &infraoas.ProbeResponse{Status: "OK"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newInfraService(Config{})
			got, err := svc.GetLivez(context.Background())
			if err != nil {
				t.Fatalf("GetLivez returned error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("GetLivez mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestInfraServiceGetReadyz(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		checkers []ReadinessChecker
		wantResp *infraoas.ProbeResponse
		wantErr  *infraoas.DefaultErrorStatusCode
	}{
		{
			name:     "succeeds without readiness checkers",
			cfg:      Config{ReadyzTimeout: time.Second},
			checkers: nil,
			wantResp: &infraoas.ProbeResponse{Status: "OK"},
		},
		{
			name: "succeeds when checker passes",
			cfg:  Config{ReadyzTimeout: time.Second},
			checkers: []ReadinessChecker{
				ReadinessCheckerFunc(func(context.Context) error { return nil }),
			},
			wantResp: &infraoas.ProbeResponse{Status: "OK"},
		},
		{
			name: "returns not ready when checker fails",
			cfg:  Config{ReadyzTimeout: time.Second},
			checkers: []ReadinessChecker{
				ReadinessCheckerFunc(func(context.Context) error { return errors.New("dependency down") }),
			},
			wantErr: &infraoas.DefaultErrorStatusCode{
				StatusCode: 503,
				Response: infraoas.ErrorResponse{
					Code:    "not_ready",
					Message: "service is not ready",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newInfraService(tt.cfg, tt.checkers...)
			gotResp, err := svc.GetReadyz(context.Background())

			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("GetReadyz returned unexpected error: %v", err)
				}
				if diff := cmp.Diff(tt.wantResp, gotResp); diff != "" {
					t.Fatalf("GetReadyz response mismatch (-want +got):\n%s", diff)
				}
				return
			}

			if gotResp != nil {
				t.Fatalf("GetReadyz response mismatch (-want +got):\n%s", cmp.Diff((*infraoas.ProbeResponse)(nil), gotResp))
			}

			var gotErr *infraoas.DefaultErrorStatusCode
			if !errors.As(err, &gotErr) {
				t.Fatalf("GetReadyz error type mismatch: got %T (%v)", err, err)
			}
			if diff := cmp.Diff(tt.wantErr, gotErr); diff != "" {
				t.Fatalf("GetReadyz error mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestInfraServiceGetReadyzTimeoutContext(t *testing.T) {
	tests := []struct {
		name          string
		readyzTimeout time.Duration
		wantDeadline  bool
	}{
		{
			name:          "sets deadline when timeout configured",
			readyzTimeout: 250 * time.Millisecond,
			wantDeadline:  true,
		},
		{
			name:          "does not set deadline when timeout disabled",
			readyzTimeout: 0,
			wantDeadline:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hadDeadline := false
			checker := ReadinessCheckerFunc(func(ctx context.Context) error {
				_, hadDeadline = ctx.Deadline()
				return nil
			})

			svc := newInfraService(Config{ReadyzTimeout: tt.readyzTimeout}, checker)
			_, err := svc.GetReadyz(context.Background())
			if err != nil {
				t.Fatalf("GetReadyz returned error: %v", err)
			}

			if diff := cmp.Diff(tt.wantDeadline, hadDeadline); diff != "" {
				t.Fatalf("readiness deadline mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestInfraServiceGetHealthz(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want struct {
			Status      string
			Environment string
		}
	}{
		{
			name: "returns expected static health fields",
			cfg:  Config{Environment: "local"},
			want: struct {
				Status      string
				Environment string
			}{
				Status:      "OK",
				Environment: "local",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newInfraService(tt.cfg)
			got, err := svc.GetHealthz(context.Background())
			if err != nil {
				t.Fatalf("GetHealthz returned error: %v", err)
			}

			if diff := cmp.Diff(tt.want.Status, got.Status); diff != "" {
				t.Fatalf("GetHealthz status mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.want.Environment, got.Environment); diff != "" {
				t.Fatalf("GetHealthz environment mismatch (-want +got):\n%s", diff)
			}

			if got.Version == "" {
				t.Fatalf("GetHealthz version should not be empty")
			}
			if _, err := time.Parse(time.RFC3339, got.Timestamp); err != nil {
				t.Fatalf("GetHealthz timestamp is not RFC3339: %v", err)
			}
		})
	}
}

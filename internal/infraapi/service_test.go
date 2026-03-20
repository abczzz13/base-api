package infraapi_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/abczzz13/base-api/internal/apierrors"
	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/infraapi"
)

func TestInfraServiceGetLivez(t *testing.T) {
	svc := infraapi.NewService(config.Config{})

	got, err := svc.GetLivez(context.Background())
	if err != nil {
		t.Fatalf("GetLivez returned error: %v", err)
	}

	want := infraapi.ProbeResponse{Status: "OK"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("GetLivez mismatch (-want +got):\n%s", diff)
	}
}

func TestInfraServiceGetReadyz(t *testing.T) {
	tests := []struct {
		name     string
		cfg      config.Config
		checkers []infraapi.ReadinessChecker
		wantResp infraapi.ProbeResponse
		wantErr  apierrors.Error
	}{
		{
			name:     "succeeds without readiness checkers",
			cfg:      config.Config{ReadyzTimeout: time.Second},
			checkers: nil,
			wantResp: infraapi.ProbeResponse{Status: "OK"},
		},
		{
			name: "succeeds when checker passes",
			cfg:  config.Config{ReadyzTimeout: time.Second},
			checkers: []infraapi.ReadinessChecker{
				infraapi.ReadinessCheckerFunc(func(context.Context) error { return nil }),
			},
			wantResp: infraapi.ProbeResponse{Status: "OK"},
		},
		{
			name: "returns not ready when checker fails",
			cfg:  config.Config{ReadyzTimeout: time.Second},
			checkers: []infraapi.ReadinessChecker{
				infraapi.ReadinessCheckerFunc(func(context.Context) error { return errors.New("dependency down") }),
			},
			wantErr: apierrors.New(503, "not_ready", "service is not ready"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := infraapi.NewService(tt.cfg, tt.checkers...)
			gotResp, err := svc.GetReadyz(context.Background())

			if tt.wantErr == (apierrors.Error{}) {
				if err != nil {
					t.Fatalf("GetReadyz returned unexpected error: %v", err)
				}
				if diff := cmp.Diff(tt.wantResp, gotResp); diff != "" {
					t.Fatalf("GetReadyz response mismatch (-want +got):\n%s", diff)
				}
				return
			}

			if diff := cmp.Diff(infraapi.ProbeResponse{}, gotResp); diff != "" {
				t.Fatalf("GetReadyz response mismatch (-want +got):\n%s", diff)
			}

			var gotErr apierrors.Error
			if !errors.As(err, &gotErr) {
				t.Fatalf("GetReadyz error type mismatch: got %T (%v)", err, err)
			}
			if diff := cmp.Diff(tt.wantErr, gotErr); diff != "" {
				t.Fatalf("GetReadyz error mismatch (-want +got):\n%s", diff)
			}
			if tt.name == "returns not ready when checker fails" && strings.Contains(gotErr.Message, "dependency down") {
				t.Fatalf("GetReadyz leaked checker details in response message: %q", gotErr.Message)
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
			checker := infraapi.ReadinessCheckerFunc(func(ctx context.Context) error {
				_, hadDeadline = ctx.Deadline()
				return nil
			})

			svc := infraapi.NewService(config.Config{ReadyzTimeout: tt.readyzTimeout}, checker)
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

func TestInfraServiceGetReadyzUsesSingleTimeoutBudget(t *testing.T) {
	readyzTimeout := 40 * time.Millisecond

	checker2Called := false
	var checker2Err error

	checker1 := infraapi.ReadinessCheckerFunc(func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})
	checker2 := infraapi.ReadinessCheckerFunc(func(ctx context.Context) error {
		checker2Called = true
		checker2Err = ctx.Err()
		return checker2Err
	})

	svc := infraapi.NewService(config.Config{ReadyzTimeout: readyzTimeout}, checker1, checker2)
	resp, err := svc.GetReadyz(context.Background())
	if diff := cmp.Diff(infraapi.ProbeResponse{}, resp); diff != "" {
		t.Fatalf("GetReadyz response mismatch (-want +got):\n%s", diff)
	}

	var gotErr apierrors.Error
	if !errors.As(err, &gotErr) {
		t.Fatalf("GetReadyz error type mismatch: got %T (%v)", err, err)
	}
	if gotErr.StatusCode != 503 {
		t.Fatalf("status code mismatch: want %d, got %d", 503, gotErr.StatusCode)
	}
	if gotErr.Code != "not_ready" {
		t.Fatalf("error code mismatch: want %q, got %q", "not_ready", gotErr.Code)
	}
	if !checker2Called {
		t.Fatal("second readiness checker was not called")
	}
	if !errors.Is(checker2Err, context.DeadlineExceeded) {
		t.Fatalf("expected second checker context deadline exceeded, got %v", checker2Err)
	}
}

func TestReadinessCheckerLogName(t *testing.T) {
	tests := []struct {
		name    string
		checker infraapi.ReadinessChecker
		index   int
		want    string
	}{
		{
			name: "uses explicit checker name",
			checker: infraapi.WithReadinessCheckerName("database", infraapi.ReadinessCheckerFunc(func(context.Context) error {
				return nil
			})),
			index: 3,
			want:  "database",
		},
		{
			name: "falls back to index when checker is unnamed",
			checker: infraapi.ReadinessCheckerFunc(func(context.Context) error {
				return nil
			}),
			index: 2,
			want:  "checker_2",
		},
		{
			name: "falls back to index when checker name is empty",
			checker: infraapi.WithReadinessCheckerName("   ", infraapi.ReadinessCheckerFunc(func(context.Context) error {
				return nil
			})),
			index: 1,
			want:  "checker_1",
		},
		{
			name:    "falls back to index when checker is nil",
			checker: nil,
			index:   0,
			want:    "checker_0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := infraapi.ReadinessCheckerLogName(tt.checker, tt.index); got != tt.want {
				t.Fatalf("checker log name mismatch: want %q, got %q", tt.want, got)
			}
		})
	}
}

func TestInfraServiceGetHealthz(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		want struct {
			Status      string
			Environment string
		}
	}{
		{
			name: "returns expected static health fields",
			cfg:  config.Config{Environment: "local"},
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
			svc := infraapi.NewService(tt.cfg)
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
			if !strings.HasSuffix(got.Timestamp, "Z") {
				t.Fatalf("GetHealthz timestamp should be UTC, got %q", got.Timestamp)
			}
		})
	}
}

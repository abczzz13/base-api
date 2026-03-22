package infraapi_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/infraapi"
	"github.com/abczzz13/base-api/internal/infraoas"
	"github.com/abczzz13/base-api/internal/requestid"
)

func TestInfraOASHandlerIncludesRequestIDHeader(t *testing.T) {
	handler, err := infraapi.NewOASHandler(infraapi.NewService(config.Config{}))
	if err != nil {
		t.Fatalf("NewOASHandler returned error: %v", err)
	}

	ctx := requestid.WithContext(context.Background(), "req-123")
	got, err := handler.GetLivez(ctx)
	if err != nil {
		t.Fatalf("GetLivez returned error: %v", err)
	}

	want := &infraoas.ProbeResponseHeaders{
		XRequestID: infraoas.NewOptString("req-123"),
		Response:   infraoas.ProbeResponse{Status: "OK"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("GetLivez mismatch (-want +got):\n%s", diff)
	}
}

func TestNewOASHandlerNilServiceReturnsError(t *testing.T) {
	_, err := infraapi.NewOASHandler(nil)
	if err == nil {
		t.Fatal("expected error from NewOASHandler(nil)")
	}
}

func TestInfraOASHandlerMapsGeneratedDefaultErrors(t *testing.T) {
	handler, err := infraapi.NewOASHandler(infraapi.NewService(config.Config{}, infraapi.ReadinessCheckerFunc(func(context.Context) error {
		return errors.New("dependency down")
	})))
	if err != nil {
		t.Fatalf("NewOASHandler returned error: %v", err)
	}

	_, err = handler.GetReadyz(context.Background())
	var gotErr *infraoas.DefaultErrorStatusCodeWithHeaders
	if !errors.As(err, &gotErr) {
		t.Fatalf("GetReadyz error type mismatch: got %T (%v)", err, err)
	}

	want := &infraoas.DefaultErrorStatusCodeWithHeaders{
		StatusCode: 503,
		Response: infraoas.ErrorResponse{
			Code:    "not_ready",
			Message: "service is not ready",
		},
	}
	if diff := cmp.Diff(want, gotErr); diff != "" {
		t.Fatalf("GetReadyz error mismatch (-want +got):\n%s", diff)
	}
}

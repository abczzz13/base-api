package publicapi_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abczzz13/base-api/internal/publicapi"
	"github.com/abczzz13/base-api/internal/publicoas"
	"github.com/abczzz13/base-api/internal/requestid"
)

func TestNewOASHandlerNilServiceReturnsHandler(t *testing.T) {
	handler := publicapi.NewOASHandler(nil)
	if handler == nil {
		t.Fatal("expected non-nil handler from NewOASHandler(nil)")
	}
}

func TestOASHandlerGetHealthzIncludesRequestIDHeader(t *testing.T) {
	handler := publicapi.NewOASHandler(publicapi.NewService())

	ctx := requestid.WithContext(context.Background(), "req-123")
	got, err := handler.GetHealthz(ctx)
	if err != nil {
		t.Fatalf("GetHealthz returned error: %v", err)
	}

	gotResp, ok := got.(*publicoas.HealthResponseHeaders)
	if !ok {
		t.Fatalf("GetHealthz response type mismatch: got %T", got)
	}

	want := &publicoas.HealthResponseHeaders{
		XRequestID: publicoas.NewOptString("req-123"),
		Response:   publicoas.HealthResponse{Status: "OK"},
	}
	if diff := cmp.Diff(want, gotResp); diff != "" {
		t.Fatalf("GetHealthz mismatch (-want +got):\n%s", diff)
	}
}

package publicapi_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abczzz13/base-api/internal/clients/weather"
	"github.com/abczzz13/base-api/internal/notes"
	"github.com/abczzz13/base-api/internal/publicapi"
	"github.com/abczzz13/base-api/internal/publicoas"
	"github.com/abczzz13/base-api/internal/requestid"
)

func TestOASHandlerGetHealthzIncludesRequestIDHeader(t *testing.T) {
	notesService, err := notes.NewService(notes.RepositoryFuncs{}, weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
		return weather.CurrentWeather{}, nil
	}))
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	handler, err := publicapi.NewOASHandler(publicapi.NewService(), notesService)
	if err != nil {
		t.Fatalf("NewOASHandler returned error: %v", err)
	}

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

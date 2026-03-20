package publicapi_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abczzz13/base-api/internal/publicapi"
)

func TestBaseServiceGetHealthz(t *testing.T) {
	svc := publicapi.NewService()

	got, err := svc.GetHealthz(context.Background())
	if err != nil {
		t.Fatalf("GetHealthz returned error: %v", err)
	}

	want := publicapi.HealthResponse{Status: "OK"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("GetHealthz mismatch (-want +got):\n%s", diff)
	}
}

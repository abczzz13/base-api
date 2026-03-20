package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	ogenmiddleware "github.com/ogen-go/ogen/middleware"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestOTELOperationAttributesAddsAttributesToSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
	})

	ctx, span := provider.Tracer("middleware-test").Start(context.Background(), "request")

	mw := OTELOperationAttributes()
	_, err := mw(ogenmiddleware.Request{
		Context:          ctx,
		OperationName:    "GetHealthz",
		OperationSummary: "Public health endpoint",
		OperationID:      "getHealthz",
		Raw:              httptest.NewRequest(http.MethodGet, "/healthz", nil),
	}, func(req ogenmiddleware.Request) (ogenmiddleware.Response, error) {
		return ogenmiddleware.Response{}, nil
	})
	if err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}

	span.End()

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected exactly one ended span, got %d", len(spans))
	}

	if got := spans[0].Name(); got != "GET GetHealthz" {
		t.Fatalf("span name mismatch: want %q, got %q", "GET GetHealthz", got)
	}

	got := map[string]string{}
	for _, attr := range spans[0].Attributes() {
		if attr.Value.Type() != attribute.STRING {
			continue
		}
		got[string(attr.Key)] = attr.Value.AsString()
	}

	want := map[string]string{
		"api.operation.name":    "GetHealthz",
		"api.operation.summary": "Public health endpoint",
	}

	for key, wantValue := range want {
		if diff := cmp.Diff(wantValue, got[key]); diff != "" {
			t.Fatalf("span attribute %q mismatch (-want +got):\n%s", key, diff)
		}
	}
}

func TestOTELOperationAttributesHandlesMissingContext(t *testing.T) {
	mw := OTELOperationAttributes()

	called := false
	_, err := mw(ogenmiddleware.Request{}, func(req ogenmiddleware.Request) (ogenmiddleware.Response, error) {
		called = true
		return ogenmiddleware.Response{}, nil
	})
	if err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}
	if !called {
		t.Fatal("expected middleware to call next")
	}
}

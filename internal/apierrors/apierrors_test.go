package apierrors

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	ht "github.com/ogen-go/ogen/http"
	"github.com/ogen-go/ogen/ogenerrors"

	"github.com/abczzz13/base-api/internal/infraoas"
	"github.com/abczzz13/base-api/internal/publicoas"
)

func TestWriteErrorProducesOASCompatiblePayload(t *testing.T) {
	rr := httptest.NewRecorder()

	WriteError(rr, "forbidden", "cross-origin request denied", http.StatusForbidden)

	if diff := cmp.Diff(http.StatusForbidden, rr.Code); diff != "" {
		t.Fatalf("status mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(contentTypeJSON, rr.Header().Get("Content-Type")); diff != "" {
		t.Fatalf("content type mismatch (-want +got):\n%s", diff)
	}

	var got publicoas.ErrorResponse
	if err := got.UnmarshalJSON(rr.Body.Bytes()); err != nil {
		t.Fatalf("decode body into publicoas.ErrorResponse: %v", err)
	}

	want := publicoas.ErrorResponse{
		Code:    "forbidden",
		Message: "cross-origin request denied",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("response mismatch (-want +got):\n%s", diff)
	}
}

func TestWriteMatchesPublicGeneratedErrorEncoding(t *testing.T) {
	apiErr := New(http.StatusForbidden, "forbidden", "cross-origin request denied")

	actual := httptest.NewRecorder()
	apiErr.Write(actual)

	server, err := publicoas.NewServer(publicErrorHandler{err: apiErr.OASDefault()})
	if err != nil {
		t.Fatalf("create public oas server: %v", err)
	}

	expected := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	server.ServeHTTP(expected, req)

	if diff := cmp.Diff(expected.Code, actual.Code); diff != "" {
		t.Fatalf("status mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(expected.Header().Get("Content-Type"), actual.Header().Get("Content-Type")); diff != "" {
		t.Fatalf("content type mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(expected.Body.String(), actual.Body.String()); diff != "" {
		t.Fatalf("body mismatch (-want +got):\n%s", diff)
	}
}

func TestWriteMatchesInfraGeneratedErrorEncoding(t *testing.T) {
	apiErr := New(http.StatusServiceUnavailable, "not_ready", "service is not ready")

	actual := httptest.NewRecorder()
	apiErr.Write(actual)

	server, err := infraoas.NewServer(infraErrorHandler{err: apiErr.InfraOASDefault()})
	if err != nil {
		t.Fatalf("create infra oas server: %v", err)
	}

	expected := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	server.ServeHTTP(expected, req)

	if diff := cmp.Diff(expected.Code, actual.Code); diff != "" {
		t.Fatalf("status mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(expected.Header().Get("Content-Type"), actual.Header().Get("Content-Type")); diff != "" {
		t.Fatalf("content type mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(expected.Body.String(), actual.Body.String()); diff != "" {
		t.Fatalf("body mismatch (-want +got):\n%s", diff)
	}
}

func TestErrorConversionsStayAlignedBetweenSpecs(t *testing.T) {
	apiErr := New(http.StatusUnauthorized, "unauthorized", "unauthorized").WithRequestID("req-123")

	publicErr := apiErr.OASDefault()
	infraErr := apiErr.InfraOASDefault()

	if diff := cmp.Diff(publicErr.StatusCode, infraErr.StatusCode); diff != "" {
		t.Fatalf("status mismatch (-want +got):\n%s", diff)
	}

	publicJSON, err := publicErr.Response.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal public error response: %v", err)
	}

	infraJSON, err := infraErr.Response.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal infra error response: %v", err)
	}

	if diff := cmp.Diff(string(publicJSON), string(infraJSON)); diff != "" {
		t.Fatalf("json mismatch (-want +got):\n%s", diff)
	}
}

func TestFromOgenError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want Error
	}{
		{
			name: "decode request maps to bad request",
			err: &ogenerrors.DecodeRequestError{
				OperationContext: ogenerrors.OperationContext{Name: "getHealthz", ID: "getHealthz"},
				Err:              errors.New("invalid input"),
			},
			want: New(http.StatusBadRequest, "bad_request", "bad request"),
		},
		{
			name: "not implemented maps to not implemented",
			err:  ht.ErrNotImplemented,
			want: New(http.StatusNotImplemented, "not_implemented", "not implemented"),
		},
		{
			name: "unknown error maps to internal error",
			err:  errors.New("boom"),
			want: New(http.StatusInternalServerError, "internal_error", "internal server error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, FromOgenError(tt.err)); diff != "" {
				t.Fatalf("FromOgenError mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

type publicErrorHandler struct {
	err *publicoas.DefaultErrorStatusCode
}

func (h publicErrorHandler) GetHealthz(context.Context) (*publicoas.HealthResponse, error) {
	return nil, h.err
}

func (h publicErrorHandler) NewError(context.Context, error) *publicoas.DefaultErrorStatusCode {
	return h.err
}

type infraErrorHandler struct {
	err *infraoas.DefaultErrorStatusCode
}

func (h infraErrorHandler) GetHealthz(context.Context) (*infraoas.HealthResponse, error) {
	return nil, h.err
}

func (h infraErrorHandler) GetLivez(context.Context) (*infraoas.ProbeResponse, error) {
	return nil, h.err
}

func (h infraErrorHandler) GetReadyz(context.Context) (*infraoas.ProbeResponse, error) {
	return nil, h.err
}

func (h infraErrorHandler) NewError(context.Context, error) *infraoas.DefaultErrorStatusCode {
	return h.err
}

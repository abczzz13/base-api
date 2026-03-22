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
	"github.com/abczzz13/base-api/internal/requestid"
	"github.com/abczzz13/base-api/internal/weatheroas"
)

func TestWriteErrorProducesOASCompatiblePayload(t *testing.T) {
	rr := httptest.NewRecorder()

	WriteError(context.Background(), rr, "forbidden", "cross-origin request denied", http.StatusForbidden)

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

func TestWriteErrorIncludesRequestIDFromContext(t *testing.T) {
	ctx := requestid.WithContext(context.Background(), "req-123")
	rr := httptest.NewRecorder()

	WriteError(ctx, rr, "forbidden", "cross-origin request denied", http.StatusForbidden)

	if diff := cmp.Diff("req-123", rr.Header().Get(requestid.HeaderName)); diff != "" {
		t.Fatalf("request ID header mismatch (-want +got):\n%s", diff)
	}

	var got publicoas.ErrorResponse
	if err := got.UnmarshalJSON(rr.Body.Bytes()); err != nil {
		t.Fatalf("decode body into publicoas.ErrorResponse: %v", err)
	}

	if !got.RequestId.IsSet() {
		t.Fatal("expected requestId to be set")
	}
	if diff := cmp.Diff("req-123", got.RequestId.Value); diff != "" {
		t.Fatalf("requestId mismatch (-want +got):\n%s", diff)
	}
}

func TestWriteMatchesPublicGeneratedErrorEncoding(t *testing.T) {
	apiErr := New(http.StatusForbidden, "forbidden", "cross-origin request denied").WithRequestID("req-123")

	actual := httptest.NewRecorder()
	apiErr.Write(actual)

	server, err := publicoas.NewServer(publicErrorHandler{err: publicDefaultError(apiErr)})
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
	if diff := cmp.Diff(expected.Header().Get(requestid.HeaderName), actual.Header().Get(requestid.HeaderName)); diff != "" {
		t.Fatalf("request ID header mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(expected.Body.String(), actual.Body.String()); diff != "" {
		t.Fatalf("body mismatch (-want +got):\n%s", diff)
	}
}

func TestWritePublicTooManyRequestsMatchesGeneratedEncoding(t *testing.T) {
	apiErr := New(http.StatusTooManyRequests, "too_many_requests", "rate limit exceeded").WithRequestID("req-123")
	headers := TooManyRequestsHeaders{
		RetryAfter:      "2",
		RateLimit:       `"default";r=0;t=2`,
		RateLimitPolicy: `"default";q=2;w=2`,
	}

	actual := httptest.NewRecorder()
	apiErr.WritePublicTooManyRequests(actual, headers)

	expected := httptest.NewRecorder()
	server, err := publicoas.NewServer(publicExplicitErrorHandler{healthz: publicTooManyRequestsError(apiErr, headers)})
	if err != nil {
		t.Fatalf("create public oas server: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	server.ServeHTTP(expected, req)

	if diff := cmp.Diff(expected.Code, actual.Code); diff != "" {
		t.Fatalf("status mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(expected.Header().Get("Content-Type"), actual.Header().Get("Content-Type")); diff != "" {
		t.Fatalf("content type mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(expected.Header().Get(requestid.HeaderName), actual.Header().Get(requestid.HeaderName)); diff != "" {
		t.Fatalf("request ID header mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(expected.Header().Get("Retry-After"), actual.Header().Get("Retry-After")); diff != "" {
		t.Fatalf("Retry-After mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(expected.Header().Get("RateLimit"), actual.Header().Get("RateLimit")); diff != "" {
		t.Fatalf("RateLimit mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(expected.Header().Get("RateLimit-Policy"), actual.Header().Get("RateLimit-Policy")); diff != "" {
		t.Fatalf("RateLimit-Policy mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(expected.Body.String(), actual.Body.String()); diff != "" {
		t.Fatalf("body mismatch (-want +got):\n%s", diff)
	}
}

func TestWritePublicServiceUnavailableMatchesGeneratedEncoding(t *testing.T) {
	apiErr := New(http.StatusServiceUnavailable, "rate_limit_unavailable", "rate limit backend unavailable").WithRequestID("req-123")

	actual := httptest.NewRecorder()
	apiErr.WritePublicServiceUnavailable(actual)

	expected := httptest.NewRecorder()
	server, err := publicoas.NewServer(publicExplicitErrorHandler{healthz: publicServiceUnavailableError(apiErr)})
	if err != nil {
		t.Fatalf("create public oas server: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	server.ServeHTTP(expected, req)

	if diff := cmp.Diff(expected.Code, actual.Code); diff != "" {
		t.Fatalf("status mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(expected.Header().Get("Content-Type"), actual.Header().Get("Content-Type")); diff != "" {
		t.Fatalf("content type mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(expected.Header().Get(requestid.HeaderName), actual.Header().Get(requestid.HeaderName)); diff != "" {
		t.Fatalf("request ID header mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(expected.Body.String(), actual.Body.String()); diff != "" {
		t.Fatalf("body mismatch (-want +got):\n%s", diff)
	}
}

func TestWriteMatchesInfraGeneratedErrorEncoding(t *testing.T) {
	apiErr := New(http.StatusServiceUnavailable, "not_ready", "service is not ready").WithRequestID("req-123")

	actual := httptest.NewRecorder()
	apiErr.Write(actual)

	server, err := infraoas.NewServer(infraErrorHandler{err: infraDefaultError(apiErr)})
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
	if diff := cmp.Diff(expected.Header().Get(requestid.HeaderName), actual.Header().Get(requestid.HeaderName)); diff != "" {
		t.Fatalf("request ID header mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(expected.Body.String(), actual.Body.String()); diff != "" {
		t.Fatalf("body mismatch (-want +got):\n%s", diff)
	}
}

func TestErrorConversionsStayAlignedAcrossSpecs(t *testing.T) {
	apiErr := New(http.StatusUnauthorized, "unauthorized", "unauthorized").WithRequestID("req-123")

	publicErr := publicDefaultError(apiErr)
	infraErr := infraDefaultError(apiErr)
	weatherErr := weatherDefaultError(apiErr)

	for name, got := range map[string]int{
		"public":  publicErr.StatusCode,
		"infra":   infraErr.StatusCode,
		"weather": weatherErr.StatusCode,
	} {
		if diff := cmp.Diff(apiErr.StatusCode, got); diff != "" {
			t.Fatalf("%s status mismatch (-want +got):\n%s", name, diff)
		}
	}

	publicJSON, err := publicErr.Response.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal public error response: %v", err)
	}

	infraJSON, err := infraErr.Response.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal infra error response: %v", err)
	}

	weatherJSON, err := weatherErr.Response.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal weather error response: %v", err)
	}

	if diff := cmp.Diff(string(publicJSON), string(infraJSON)); diff != "" {
		t.Fatalf("public/infra json mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(string(publicJSON), string(weatherJSON)); diff != "" {
		t.Fatalf("public/weather json mismatch (-want +got):\n%s", diff)
	}

	for name, value := range map[string]string{
		"public":  publicErr.XRequestID.Value,
		"infra":   infraErr.XRequestID.Value,
		"weather": weatherErr.XRequestID.Value,
	} {
		if diff := cmp.Diff(apiErr.RequestID, value); diff != "" {
			t.Fatalf("%s request ID mismatch (-want +got):\n%s", name, diff)
		}
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
	err *publicoas.DefaultErrorStatusCodeWithHeaders
}

type publicExplicitErrorHandler struct {
	healthz publicoas.GetHealthzRes
}

func (h publicExplicitErrorHandler) GetHealthz(context.Context) (publicoas.GetHealthzRes, error) {
	return h.healthz, nil
}

func (h publicExplicitErrorHandler) CreateNote(context.Context, *publicoas.CreateNoteRequest) (publicoas.CreateNoteRes, error) {
	return nil, errors.New("unexpected create note call")
}

func (h publicExplicitErrorHandler) DeleteNote(context.Context, publicoas.DeleteNoteParams) (publicoas.DeleteNoteRes, error) {
	return nil, errors.New("unexpected delete note call")
}

func (h publicExplicitErrorHandler) GetNote(context.Context, publicoas.GetNoteParams) (publicoas.GetNoteRes, error) {
	return nil, errors.New("unexpected get note call")
}

func (h publicExplicitErrorHandler) ListNotes(context.Context, publicoas.ListNotesParams) (publicoas.ListNotesRes, error) {
	return nil, errors.New("unexpected list notes call")
}

func (h publicExplicitErrorHandler) UpdateNote(context.Context, *publicoas.UpdateNoteRequest, publicoas.UpdateNoteParams) (publicoas.UpdateNoteRes, error) {
	return nil, errors.New("unexpected update note call")
}

func (h publicExplicitErrorHandler) NewError(context.Context, error) *publicoas.DefaultErrorStatusCodeWithHeaders {
	return publicDefaultError(New(http.StatusInternalServerError, "internal_error", "internal server error"))
}

func (h publicErrorHandler) GetHealthz(context.Context) (publicoas.GetHealthzRes, error) {
	return nil, h.err
}

func (h publicErrorHandler) CreateNote(context.Context, *publicoas.CreateNoteRequest) (publicoas.CreateNoteRes, error) {
	return nil, h.err
}

func (h publicErrorHandler) DeleteNote(context.Context, publicoas.DeleteNoteParams) (publicoas.DeleteNoteRes, error) {
	return nil, h.err
}

func (h publicErrorHandler) GetNote(context.Context, publicoas.GetNoteParams) (publicoas.GetNoteRes, error) {
	return nil, h.err
}

func (h publicErrorHandler) ListNotes(context.Context, publicoas.ListNotesParams) (publicoas.ListNotesRes, error) {
	return nil, h.err
}

func (h publicErrorHandler) UpdateNote(context.Context, *publicoas.UpdateNoteRequest, publicoas.UpdateNoteParams) (publicoas.UpdateNoteRes, error) {
	return nil, h.err
}

func (h publicErrorHandler) NewError(context.Context, error) *publicoas.DefaultErrorStatusCodeWithHeaders {
	return h.err
}

type infraErrorHandler struct {
	err *infraoas.DefaultErrorStatusCodeWithHeaders
}

func (h infraErrorHandler) GetHealthz(context.Context) (*infraoas.HealthResponseHeaders, error) {
	return nil, h.err
}

func (h infraErrorHandler) GetLivez(context.Context) (*infraoas.ProbeResponseHeaders, error) {
	return nil, h.err
}

func (h infraErrorHandler) GetReadyz(context.Context) (*infraoas.ProbeResponseHeaders, error) {
	return nil, h.err
}

func (h infraErrorHandler) NewError(context.Context, error) *infraoas.DefaultErrorStatusCodeWithHeaders {
	return h.err
}

func publicDefaultError(apiErr Error) *publicoas.DefaultErrorStatusCodeWithHeaders {
	apiErr = apiErr.normalize()

	response := &publicoas.DefaultErrorStatusCodeWithHeaders{
		StatusCode: apiErr.StatusCode,
		Response: publicoas.ErrorResponse{
			Code:    apiErr.Code,
			Message: apiErr.Message,
		},
	}
	if apiErr.RequestID != "" {
		response.XRequestID = publicoas.NewOptString(apiErr.RequestID)
		response.Response.RequestId = publicoas.NewOptString(apiErr.RequestID)
	}

	return response
}

func publicTooManyRequestsError(apiErr Error, headers TooManyRequestsHeaders) *publicoas.TooManyRequestsErrorHeaders {
	apiErr = apiErr.normalize()

	response := &publicoas.TooManyRequestsErrorHeaders{
		Response: publicoas.ErrorResponse{
			Code:    apiErr.Code,
			Message: apiErr.Message,
		},
	}
	if apiErr.RequestID != "" {
		response.XRequestID = publicoas.NewOptString(apiErr.RequestID)
		response.Response.RequestId = publicoas.NewOptString(apiErr.RequestID)
	}
	if headers.RetryAfter != "" {
		response.RetryAfter = publicoas.NewOptString(headers.RetryAfter)
	}
	if headers.RateLimit != "" {
		response.Ratelimit = publicoas.NewOptString(headers.RateLimit)
	}
	if headers.RateLimitPolicy != "" {
		response.RatelimitPolicy = publicoas.NewOptString(headers.RateLimitPolicy)
	}

	return response
}

func publicServiceUnavailableError(apiErr Error) *publicoas.ServiceUnavailableErrorHeaders {
	apiErr = apiErr.normalize()

	response := &publicoas.ServiceUnavailableErrorHeaders{
		Response: publicoas.ErrorResponse{
			Code:    apiErr.Code,
			Message: apiErr.Message,
		},
	}
	if apiErr.RequestID != "" {
		response.XRequestID = publicoas.NewOptString(apiErr.RequestID)
		response.Response.RequestId = publicoas.NewOptString(apiErr.RequestID)
	}

	return response
}

func infraDefaultError(apiErr Error) *infraoas.DefaultErrorStatusCodeWithHeaders {
	apiErr = apiErr.normalize()

	response := &infraoas.DefaultErrorStatusCodeWithHeaders{
		StatusCode: apiErr.StatusCode,
		Response: infraoas.ErrorResponse{
			Code:    apiErr.Code,
			Message: apiErr.Message,
		},
	}
	if apiErr.RequestID != "" {
		response.XRequestID = infraoas.NewOptString(apiErr.RequestID)
		response.Response.RequestId = infraoas.NewOptString(apiErr.RequestID)
	}

	return response
}

func weatherDefaultError(apiErr Error) *weatheroas.DefaultErrorStatusCodeWithHeaders {
	apiErr = apiErr.normalize()

	response := &weatheroas.DefaultErrorStatusCodeWithHeaders{
		StatusCode: apiErr.StatusCode,
		Response: weatheroas.ErrorResponse{
			Code:    apiErr.Code,
			Message: apiErr.Message,
		},
	}
	if apiErr.RequestID != "" {
		response.XRequestID = weatheroas.NewOptString(apiErr.RequestID)
		response.Response.RequestId = weatheroas.NewOptString(apiErr.RequestID)
	}

	return response
}

// Package apierrors provides shared error response utilities that stay aligned with OpenAPI-generated error schemas.
package apierrors

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ogen-go/ogen/ogenerrors"

	"github.com/abczzz13/base-api/internal/requestid"
)

const contentTypeJSON = "application/json; charset=utf-8"

// Error defines a canonical API error used across middleware and handlers.
type Error struct {
	StatusCode int
	Code       string
	Message    string
	RequestID  string
}

func (e Error) Error() string {
	e = e.normalize()
	return e.Message
}

// TooManyRequestsHeaders carries rate-limit response headers for 429 responses.
type TooManyRequestsHeaders struct {
	RetryAfter      string
	RateLimit       string
	RateLimitPolicy string
}

// New constructs a canonical API error.
func New(statusCode int, code, message string) Error {
	return Error{
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
	}
}

// WithRequestID returns a copy with request ID set.
func (e Error) WithRequestID(requestID string) Error {
	e.RequestID = requestID
	return e
}

// WithContext returns a copy with request metadata populated from ctx.
func (e Error) WithContext(ctx context.Context) Error {
	if e.RequestID == "" {
		e.RequestID = requestid.FromContext(ctx)
	}

	return e
}

// Write writes a spec-compliant JSON error response that matches generated OpenAPI responses.
func (e Error) Write(w http.ResponseWriter) {
	e = e.normalize()

	body, err := json.Marshal(e.responseBody())
	if err != nil {
		fallback := New(http.StatusInternalServerError, "internal_error", "internal server error").WithRequestID(e.RequestID)
		body, _ = json.Marshal(fallback.responseBody())
		e = fallback
	}

	w.Header().Set("Content-Type", contentTypeJSON)
	if e.RequestID != "" {
		w.Header().Set(requestid.HeaderName, e.RequestID)
	}
	w.WriteHeader(e.StatusCode)
	_, _ = w.Write(body)
}

// WriteContext writes a spec-compliant error response enriched from ctx.
func (e Error) WriteContext(ctx context.Context, w http.ResponseWriter) {
	e.WithContext(ctx).Write(w)
}

// WritePublicTooManyRequests writes the explicit public API 429 response.
func (e Error) WritePublicTooManyRequests(w http.ResponseWriter, headers TooManyRequestsHeaders) {
	e = e.normalize()
	if err := writePublicErrorResponse(w, http.StatusTooManyRequests, e.responseBody(), publicErrorHeaders{
		RequestID:       e.RequestID,
		RetryAfter:      headers.RetryAfter,
		RateLimit:       headers.RateLimit,
		RateLimitPolicy: headers.RateLimitPolicy,
	}); err != nil {
		e.Write(w)
	}
}

// WritePublicServiceUnavailable writes the explicit public API 503 response.
func (e Error) WritePublicServiceUnavailable(w http.ResponseWriter) {
	e = e.normalize()
	if err := writePublicErrorResponse(w, http.StatusServiceUnavailable, e.responseBody(), publicErrorHeaders{
		RequestID: e.RequestID,
	}); err != nil {
		e.Write(w)
	}
}

// WriteError writes a spec-compliant error response to the ResponseWriter.
func WriteError(ctx context.Context, w http.ResponseWriter, code, message string, statusCode int) {
	New(statusCode, code, message).WriteContext(ctx, w)
}

// ResolveError extracts a canonical API error from err (defaulting to 500) and enriches it from ctx.
func ResolveError(ctx context.Context, err error) Error {
	apiErr := New(http.StatusInternalServerError, "internal_error", "internal server error")
	var typedErr Error
	if errors.As(err, &typedErr) {
		apiErr = typedErr
	}

	return apiErr.WithContext(ctx)
}

// FromOgenError maps ogen framework errors to canonical API errors.
func FromOgenError(err error) Error {
	statusCode := ogenerrors.ErrorCode(err)
	code, message := statusCodeToCodeMessage(statusCode)

	return New(statusCode, code, message)
}

func (e Error) normalize() Error {
	if e.StatusCode == 0 {
		e.StatusCode = http.StatusInternalServerError
	}

	defaultCode, defaultMessage := statusCodeToCodeMessage(e.StatusCode)
	if e.Code == "" {
		e.Code = defaultCode
	}
	if e.Message == "" {
		e.Message = defaultMessage
	}

	return e
}

type responseBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
}

func (e Error) responseBody() responseBody {
	return responseBody{
		Code:      e.Code,
		Message:   e.Message,
		RequestID: e.RequestID,
	}
}

type publicErrorHeaders struct {
	RequestID       string
	RetryAfter      string
	RateLimit       string
	RateLimitPolicy string
}

func writePublicErrorResponse(w http.ResponseWriter, statusCode int, response responseBody, headers publicErrorHeaders) error {
	body, err := json.Marshal(response)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", contentTypeJSON)
	if headers.RequestID != "" {
		w.Header().Set(requestid.HeaderName, headers.RequestID)
	}
	if headers.RetryAfter != "" {
		w.Header().Set("Retry-After", headers.RetryAfter)
	}
	if headers.RateLimit != "" {
		w.Header().Set("RateLimit", headers.RateLimit)
	}
	if headers.RateLimitPolicy != "" {
		w.Header().Set("RateLimit-Policy", headers.RateLimitPolicy)
	}
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)

	return nil
}

func statusCodeToCodeMessage(statusCode int) (code, message string) {
	switch statusCode {
	case http.StatusBadRequest:
		return "bad_request", "bad request"
	case http.StatusUnauthorized:
		return "unauthorized", "unauthorized"
	case http.StatusForbidden:
		return "forbidden", "forbidden"
	case http.StatusNotFound:
		return "not_found", "not found"
	case http.StatusMethodNotAllowed:
		return "method_not_allowed", "method not allowed"
	case http.StatusNotAcceptable:
		return "not_acceptable", "not acceptable"
	case http.StatusRequestTimeout:
		return "request_timeout", "request timeout"
	case http.StatusConflict:
		return "conflict", "conflict"
	case http.StatusGone:
		return "gone", "gone"
	case http.StatusRequestEntityTooLarge:
		return "payload_too_large", "payload too large"
	case http.StatusUnsupportedMediaType:
		return "unsupported_media_type", "unsupported media type"
	case http.StatusUnprocessableEntity:
		return "unprocessable_entity", "unprocessable entity"
	case http.StatusTooManyRequests:
		return "too_many_requests", "too many requests"
	case http.StatusNotImplemented:
		return "not_implemented", "not implemented"
	case http.StatusBadGateway:
		return "bad_gateway", "bad gateway"
	case http.StatusServiceUnavailable:
		return "service_unavailable", "service unavailable"
	case http.StatusGatewayTimeout:
		return "gateway_timeout", "gateway timeout"
	}

	if statusCode >= http.StatusInternalServerError {
		return "internal_error", "internal server error"
	}

	if statusCode >= http.StatusBadRequest {
		return "request_error", "request failed"
	}

	return "internal_error", "internal server error"
}

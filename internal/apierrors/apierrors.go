// Package apierrors provides shared error response utilities that stay aligned with OpenAPI-generated error schemas.
package apierrors

import (
	"net/http"

	"github.com/ogen-go/ogen/ogenerrors"

	"github.com/abczzz13/base-api/internal/infraoas"
	"github.com/abczzz13/base-api/internal/oas"
)

const contentTypeJSON = "application/json; charset=utf-8"

// Error defines a canonical API error used across middleware and handlers.
type Error struct {
	StatusCode int
	Code       string
	Message    string
	RequestID  string
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

// OASDefault converts Error into the public API generated default error wrapper.
func (e Error) OASDefault() *oas.DefaultErrorStatusCode {
	e = e.normalize()

	return &oas.DefaultErrorStatusCode{
		StatusCode: e.StatusCode,
		Response:   e.oasResponse(),
	}
}

// InfraOASDefault converts Error into the infra API generated default error wrapper.
func (e Error) InfraOASDefault() *infraoas.DefaultErrorStatusCode {
	e = e.normalize()

	return &infraoas.DefaultErrorStatusCode{
		StatusCode: e.StatusCode,
		Response:   e.infraOASResponse(),
	}
}

// Write writes a spec-compliant JSON error response that matches generated OpenAPI responses.
func (e Error) Write(w http.ResponseWriter) {
	e = e.normalize()

	response := e.oasResponse()
	body, err := response.MarshalJSON()
	if err != nil {
		fallback := New(http.StatusInternalServerError, "internal_error", "internal server error")
		fallbackResponse := fallback.oasResponse()
		body, _ = fallbackResponse.MarshalJSON()
		e = fallback
	}

	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(e.StatusCode)
	_, _ = w.Write(body)
}

// WriteError writes a spec-compliant error response to the ResponseWriter.
func WriteError(w http.ResponseWriter, code, message string, statusCode int) {
	New(statusCode, code, message).Write(w)
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

func (e Error) oasResponse() oas.ErrorResponse {
	response := oas.ErrorResponse{
		Code:    e.Code,
		Message: e.Message,
	}

	if e.RequestID != "" {
		response.RequestId = oas.NewOptString(e.RequestID)
	}

	return response
}

func (e Error) infraOASResponse() infraoas.ErrorResponse {
	response := infraoas.ErrorResponse{
		Code:    e.Code,
		Message: e.Message,
	}

	if e.RequestID != "" {
		response.RequestId = infraoas.NewOptString(e.RequestID)
	}

	return response
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

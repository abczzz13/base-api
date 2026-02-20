package server

import (
	"github.com/abczzz13/base-api/internal/infraoas"
	"github.com/abczzz13/base-api/internal/oas"
)

func newDefaultError(statusCode int, code, message string) *oas.DefaultErrorStatusCode {
	return &oas.DefaultErrorStatusCode{
		StatusCode: statusCode,
		Response: oas.ErrorResponse{
			Code:    code,
			Message: message,
		},
	}
}

func newInfraDefaultError(statusCode int, code, message string) *infraoas.DefaultErrorStatusCode {
	return &infraoas.DefaultErrorStatusCode{
		StatusCode: statusCode,
		Response: infraoas.ErrorResponse{
			Code:    code,
			Message: message,
		},
	}
}

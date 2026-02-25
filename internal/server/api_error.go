package server

import (
	"github.com/abczzz13/base-api/internal/apierrors"
	"github.com/abczzz13/base-api/internal/infraoas"
	"github.com/abczzz13/base-api/internal/oas"
)

func newDefaultError(statusCode int, code, message string) *oas.DefaultErrorStatusCode {
	return apierrors.New(statusCode, code, message).OASDefault()
}

func newInfraDefaultError(statusCode int, code, message string) *infraoas.DefaultErrorStatusCode {
	return apierrors.New(statusCode, code, message).InfraOASDefault()
}

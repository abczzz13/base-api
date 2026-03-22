package weatherapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/abczzz13/base-api/internal/apierrors"
	"github.com/abczzz13/base-api/internal/middleware"
	"github.com/abczzz13/base-api/internal/requestid"
	"github.com/abczzz13/base-api/internal/weatheroas"
)

type oasHandler struct {
	service *Service
}

var _ weatheroas.Handler = (*oasHandler)(nil)

// NewOASHandler adapts the handwritten weather service to the generated weather transport.
func NewOASHandler(service *Service) (weatheroas.Handler, error) {
	if service == nil {
		return nil, errors.New("service is required")
	}

	return &oasHandler{service: service}, nil
}

func (h *oasHandler) GetCurrentWeather(ctx context.Context, params weatheroas.GetCurrentWeatherParams) (weatheroas.GetCurrentWeatherRes, error) {
	result, err := h.service.GetCurrentWeather(ctx, params.Location)
	if err != nil {
		return nil, weatherDefaultError(ctx, err)
	}

	wrapped := &weatheroas.CurrentWeatherResponseHeaders{
		Response: weatheroas.CurrentWeatherResponse{
			Provider:     result.Provider,
			Location:     result.Location,
			Condition:    result.Condition,
			TemperatureC: result.TemperatureC,
			ObservedAt:   result.ObservedAt,
		},
	}
	setWeatherRequestIDHeader(wrapped, ctx)

	return wrapped, nil
}

func (h *oasHandler) NewError(ctx context.Context, err error) *weatheroas.DefaultErrorStatusCodeWithHeaders {
	return weatherDefaultError(ctx, err)
}

// RouteLabeler returns a route labeler that resolves weather API operation names.
func RouteLabeler(api *weatheroas.Server) func(*http.Request) string {
	return middleware.OperationLabeler(middleware.OperationFinderFunc(func(method string, u *url.URL) (string, bool) {
		if route, ok := api.FindPath(method, u); ok {
			return route.Name(), true
		}
		return "", false
	}))
}

func weatherDefaultError(ctx context.Context, err error) *weatheroas.DefaultErrorStatusCodeWithHeaders {
	apiErr := apierrors.ResolveError(ctx, err)

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

func setWeatherRequestIDHeader(response *weatheroas.CurrentWeatherResponseHeaders, ctx context.Context) {
	if requestID := requestid.FromContext(ctx); requestID != "" {
		response.SetXRequestID(weatheroas.NewOptString(requestID))
	}
}

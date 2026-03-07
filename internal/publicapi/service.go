package publicapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/abczzz13/base-api/internal/apierrors"
	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/publicoas"
	"github.com/abczzz13/base-api/internal/requestid"
	"github.com/abczzz13/base-api/internal/weather"
)

var _ publicoas.Handler = (*Service)(nil)

// Service implements the public OAS-generated handler interface.
type Service struct {
	cfg           config.Config
	weatherClient weather.Client
}

// NewService creates a new public API service.
func NewService(cfg config.Config, weatherClient weather.Client) *Service {
	return &Service{cfg: cfg, weatherClient: weatherClient}
}

func (s *Service) GetHealthz(ctx context.Context) (publicoas.GetHealthzRes, error) {
	response := &publicoas.HealthResponseHeaders{
		Response: publicoas.HealthResponse{
			Status: "OK",
		},
	}
	setRequestIDHeader(response, ctx)

	return response, nil
}

func (s *Service) GetCurrentWeather(ctx context.Context, params publicoas.GetCurrentWeatherParams) (publicoas.GetCurrentWeatherRes, error) {
	if s.weatherClient == nil {
		return nil, apierrors.New(http.StatusServiceUnavailable, "weather_unavailable", "weather integration is not configured").WithContext(ctx).OASDefault()
	}

	currentWeather, err := s.weatherClient.GetCurrent(ctx, params.Location)
	if err != nil {
		return nil, weatherErrorResponse(ctx, err)
	}

	response := &publicoas.CurrentWeatherResponseHeaders{
		Response: publicoas.CurrentWeatherResponse{
			Provider:     currentWeather.Provider,
			Location:     currentWeather.Location,
			Condition:    currentWeather.Condition,
			TemperatureC: currentWeather.TemperatureC,
			ObservedAt:   currentWeather.ObservedAt,
		},
	}
	setRequestIDHeader(response, ctx)

	return response, nil
}

func (s *Service) NewError(ctx context.Context, err error) *publicoas.DefaultErrorStatusCodeWithHeaders {
	_ = err

	return apierrors.New(http.StatusInternalServerError, "internal_error", "internal server error").WithContext(ctx).OASDefault()
}

func weatherErrorResponse(ctx context.Context, err error) *publicoas.DefaultErrorStatusCodeWithHeaders {
	var notFoundErr *weather.NotFoundError
	if errors.As(err, &notFoundErr) {
		return apierrors.New(http.StatusNotFound, "weather_location_not_found", "weather location not found").WithContext(ctx).OASDefault()
	}

	var upstreamErr *weather.UpstreamError
	if errors.As(err, &upstreamErr) {
		return apierrors.New(http.StatusBadGateway, "weather_upstream_error", "weather provider returned an invalid response").WithContext(ctx).OASDefault()
	}

	var decodeErr *weather.DecodeError
	if errors.As(err, &decodeErr) {
		return apierrors.New(http.StatusBadGateway, "weather_upstream_error", "weather provider returned an invalid response").WithContext(ctx).OASDefault()
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return apierrors.New(http.StatusGatewayTimeout, "weather_timeout", "weather provider request timed out").WithContext(ctx).OASDefault()
	}

	return apierrors.New(http.StatusBadGateway, "weather_request_failed", "weather provider request failed").WithContext(ctx).OASDefault()
}

func setRequestIDHeader[T interface{ SetXRequestID(publicoas.OptString) }](response T, ctx context.Context) {
	if requestID := requestid.FromContext(ctx); requestID != "" {
		response.SetXRequestID(publicoas.NewOptString(requestID))
	}
}

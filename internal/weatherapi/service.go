package weatherapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/abczzz13/base-api/internal/apierrors"
	"github.com/abczzz13/base-api/internal/clients/weather"
)

// Service contains handwritten weather API behavior, independent of generated transport types.
type Service struct {
	weatherClient weather.Client
}

// NewService creates a new public weather service.
func NewService(weatherClient weather.Client) *Service {
	return &Service{weatherClient: weatherClient}
}

func (s *Service) GetCurrentWeather(ctx context.Context, location string) (weather.CurrentWeather, error) {
	currentWeather, err := s.weatherClient.GetCurrent(ctx, location)
	if err != nil {
		return weather.CurrentWeather{}, weatherError(err)
	}

	return currentWeather, nil
}

func weatherError(err error) error {
	var notFoundErr *weather.NotFoundError
	if errors.As(err, &notFoundErr) {
		return apierrors.New(http.StatusNotFound, "weather_location_not_found", "weather location not found")
	}

	var upstreamErr *weather.UpstreamError
	var decodeErr *weather.DecodeError
	if errors.As(err, &upstreamErr) || errors.As(err, &decodeErr) {
		return apierrors.New(http.StatusBadGateway, "weather_upstream_error", "weather provider returned an invalid response")
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return apierrors.New(http.StatusGatewayTimeout, "weather_timeout", "weather provider request timed out")
	}

	return apierrors.New(http.StatusBadGateway, "weather_request_failed", "weather provider request failed")
}

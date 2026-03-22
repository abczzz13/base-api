package weatherapi_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/abczzz13/base-api/internal/clients/weather"
	"github.com/abczzz13/base-api/internal/requestid"
	"github.com/abczzz13/base-api/internal/weatherapi"
	"github.com/abczzz13/base-api/internal/weatheroas"
)

func TestOASHandlerGetCurrentWeatherMapsResponsesAndErrors(t *testing.T) {
	observedAt := time.Date(2026, time.March, 7, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		weatherClient weather.Client
		ctx           context.Context
		wantResp      *weatheroas.CurrentWeatherResponseHeaders
		wantErr       *weatheroas.DefaultErrorStatusCodeWithHeaders
	}{
		{
			name: "maps success response",
			ctx:  requestid.WithContext(context.Background(), "req-123"),
			weatherClient: weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
				return weather.CurrentWeather{
					Provider:     "open-meteo",
					Location:     "Amsterdam",
					Condition:    "Cloudy",
					TemperatureC: 12.5,
					ObservedAt:   observedAt,
				}, nil
			}),
			wantResp: &weatheroas.CurrentWeatherResponseHeaders{
				XRequestID: weatheroas.NewOptString("req-123"),
				Response: weatheroas.CurrentWeatherResponse{
					Provider:     "open-meteo",
					Location:     "Amsterdam",
					Condition:    "Cloudy",
					TemperatureC: 12.5,
					ObservedAt:   observedAt,
				},
			},
		},
		{
			name: "maps service error to generated default error",
			weatherClient: weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
				return weather.CurrentWeather{}, errors.New("boom")
			}),
			wantErr: &weatheroas.DefaultErrorStatusCodeWithHeaders{
				StatusCode: 502,
				Response: weatheroas.ErrorResponse{
					Code:    "weather_request_failed",
					Message: "weather provider request failed",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := weatherapi.NewOASHandler(weatherapi.NewService(tt.weatherClient))
			if err != nil {
				t.Fatalf("NewOASHandler returned error: %v", err)
			}
			got, err := handler.GetCurrentWeather(tt.ctx, weatheroas.GetCurrentWeatherParams{Location: "Amsterdam"})

			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("GetCurrentWeather returned unexpected error: %v", err)
				}
				gotResp, ok := got.(*weatheroas.CurrentWeatherResponseHeaders)
				if !ok {
					t.Fatalf("GetCurrentWeather response type mismatch: got %T", got)
				}
				if diff := cmp.Diff(tt.wantResp, gotResp); diff != "" {
					t.Fatalf("GetCurrentWeather response mismatch (-want +got):\n%s", diff)
				}
				return
			}

			if got != nil {
				t.Fatalf("GetCurrentWeather response mismatch (-want +got):\n%s", cmp.Diff((*weatheroas.CurrentWeatherResponseHeaders)(nil), got))
			}

			var gotErr *weatheroas.DefaultErrorStatusCodeWithHeaders
			if !errors.As(err, &gotErr) {
				t.Fatalf("GetCurrentWeather error type mismatch: got %T (%v)", err, err)
			}
			if diff := cmp.Diff(tt.wantErr, gotErr); diff != "" {
				t.Fatalf("GetCurrentWeather error mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

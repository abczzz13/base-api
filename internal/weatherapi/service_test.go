package weatherapi_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/abczzz13/base-api/internal/apierrors"
	"github.com/abczzz13/base-api/internal/clients/weather"
	"github.com/abczzz13/base-api/internal/weatherapi"
)

func TestServiceGetCurrentWeather(t *testing.T) {
	observedAt := time.Date(2026, time.March, 7, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		weatherClient weather.Client
		location      string
		wantResp      weather.CurrentWeather
		wantErr       apierrors.Error
	}{
		{
			name:     "returns weather conditions when integration succeeds",
			location: "Amsterdam",
			weatherClient: weather.ClientFunc(func(ctx context.Context, location string) (weather.CurrentWeather, error) {
				if diff := cmp.Diff("Amsterdam", location); diff != "" {
					t.Fatalf("location mismatch (-want +got):\n%s", diff)
				}

				return weather.CurrentWeather{
					Provider:     "open-meteo",
					Location:     "Amsterdam",
					Condition:    "Cloudy",
					TemperatureC: 12.5,
					ObservedAt:   observedAt,
				}, nil
			}),
			wantResp: weather.CurrentWeather{
				Provider:     "open-meteo",
				Location:     "Amsterdam",
				Condition:    "Cloudy",
				TemperatureC: 12.5,
				ObservedAt:   observedAt,
			},
		},
		{
			name:     "maps upstream timeout to gateway timeout",
			location: "Amsterdam",
			weatherClient: weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
				return weather.CurrentWeather{}, context.DeadlineExceeded
			}),
			wantErr: apierrors.New(504, "weather_timeout", "weather provider request timed out"),
		},
		{
			name:     "maps upstream non-success status to bad gateway",
			location: "Amsterdam",
			weatherClient: weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
				return weather.CurrentWeather{}, &weather.UpstreamError{StatusCode: 503}
			}),
			wantErr: apierrors.New(502, "weather_upstream_error", "weather provider returned an invalid response"),
		},
		{
			name:     "maps malformed upstream payload to bad gateway",
			location: "Amsterdam",
			weatherClient: weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
				return weather.CurrentWeather{}, &weather.DecodeError{Err: errors.New("bad payload")}
			}),
			wantErr: apierrors.New(502, "weather_upstream_error", "weather provider returned an invalid response"),
		},
		{
			name:     "maps unknown location to not found",
			location: "Atlantis",
			weatherClient: weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
				return weather.CurrentWeather{}, &weather.NotFoundError{Location: "Atlantis"}
			}),
			wantErr: apierrors.New(404, "weather_location_not_found", "weather location not found"),
		},
		{
			name:     "maps generic transport failures to bad gateway",
			location: "Amsterdam",
			weatherClient: weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
				return weather.CurrentWeather{}, errors.New("dial failed")
			}),
			wantErr: apierrors.New(502, "weather_request_failed", "weather provider request failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := weatherapi.NewService(tt.weatherClient)
			got, err := svc.GetCurrentWeather(context.Background(), tt.location)

			if tt.wantErr == (apierrors.Error{}) {
				if err != nil {
					t.Fatalf("GetCurrentWeather returned unexpected error: %v", err)
				}
				if diff := cmp.Diff(tt.wantResp, got); diff != "" {
					t.Fatalf("GetCurrentWeather mismatch (-want +got):\n%s", diff)
				}
				return
			}

			if diff := cmp.Diff(weather.CurrentWeather{}, got); diff != "" {
				t.Fatalf("GetCurrentWeather response mismatch (-want +got):\n%s", diff)
			}

			var gotErr apierrors.Error
			if !errors.As(err, &gotErr) {
				t.Fatalf("GetCurrentWeather error type mismatch: got %T (%v)", err, err)
			}
			if diff := cmp.Diff(tt.wantErr, gotErr); diff != "" {
				t.Fatalf("GetCurrentWeather error mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

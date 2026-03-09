package publicapi_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/abczzz13/base-api/internal/clients/weather"
	"github.com/abczzz13/base-api/internal/publicapi"
	"github.com/abczzz13/base-api/internal/publicoas"
	"github.com/abczzz13/base-api/internal/requestid"
)

func TestBaseServiceGetHealthz(t *testing.T) {
	tests := []struct {
		name string
		want *publicoas.HealthResponseHeaders
	}{
		{
			name: "returns public safe health response",
			want: &publicoas.HealthResponseHeaders{Response: publicoas.HealthResponse{Status: "OK"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := publicapi.NewService(nil)
			got, err := svc.GetHealthz(context.Background())
			if err != nil {
				t.Fatalf("GetHealthz returned error: %v", err)
			}

			gotResp, ok := got.(*publicoas.HealthResponseHeaders)
			if !ok {
				t.Fatalf("GetHealthz response type mismatch: got %T", got)
			}

			if diff := cmp.Diff(tt.want, gotResp); diff != "" {
				t.Fatalf("GetHealthz mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBaseServiceGetCurrentWeather(t *testing.T) {
	observedAt := time.Date(2026, time.March, 7, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		weatherClient weather.Client
		ctx           context.Context
		params        publicoas.GetCurrentWeatherParams
		wantResp      *publicoas.CurrentWeatherResponseHeaders
		wantErr       *publicoas.DefaultErrorStatusCodeWithHeaders
	}{
		{
			name:   "returns weather conditions when integration succeeds",
			ctx:    requestid.WithContext(context.Background(), "req-123"),
			params: publicoas.GetCurrentWeatherParams{Location: "Amsterdam"},
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
			wantResp: &publicoas.CurrentWeatherResponseHeaders{
				XRequestID: publicoas.NewOptString("req-123"),
				Response: publicoas.CurrentWeatherResponse{
					Provider:     "open-meteo",
					Location:     "Amsterdam",
					Condition:    "Cloudy",
					TemperatureC: 12.5,
					ObservedAt:   observedAt,
				},
			},
		},
		{
			name:    "returns unavailable when weather client is missing",
			ctx:     context.Background(),
			params:  publicoas.GetCurrentWeatherParams{Location: "Amsterdam"},
			wantErr: errorResponse(http.StatusServiceUnavailable, "weather_unavailable", "weather integration is not configured"),
		},
		{
			name:   "maps upstream timeout to gateway timeout",
			ctx:    context.Background(),
			params: publicoas.GetCurrentWeatherParams{Location: "Amsterdam"},
			weatherClient: weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
				return weather.CurrentWeather{}, context.DeadlineExceeded
			}),
			wantErr: errorResponse(http.StatusGatewayTimeout, "weather_timeout", "weather provider request timed out"),
		},
		{
			name:   "maps malformed upstream payload to bad gateway",
			ctx:    context.Background(),
			params: publicoas.GetCurrentWeatherParams{Location: "Amsterdam"},
			weatherClient: weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
				return weather.CurrentWeather{}, &weather.DecodeError{Err: errors.New("bad payload")}
			}),
			wantErr: errorResponse(http.StatusBadGateway, "weather_upstream_error", "weather provider returned an invalid response"),
		},
		{
			name:   "maps unknown location to not found",
			ctx:    context.Background(),
			params: publicoas.GetCurrentWeatherParams{Location: "Atlantis"},
			weatherClient: weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
				return weather.CurrentWeather{}, &weather.NotFoundError{Location: "Atlantis"}
			}),
			wantErr: errorResponse(http.StatusNotFound, "weather_location_not_found", "weather location not found"),
		},
		{
			name:   "maps generic transport failures to bad gateway",
			ctx:    context.Background(),
			params: publicoas.GetCurrentWeatherParams{Location: "Amsterdam"},
			weatherClient: weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
				return weather.CurrentWeather{}, errors.New("dial failed")
			}),
			wantErr: errorResponse(http.StatusBadGateway, "weather_request_failed", "weather provider request failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := publicapi.NewService(tt.weatherClient)
			got, err := svc.GetCurrentWeather(tt.ctx, tt.params)

			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("GetCurrentWeather returned unexpected error: %v", err)
				}
				gotResp, ok := got.(*publicoas.CurrentWeatherResponseHeaders)
				if !ok {
					t.Fatalf("GetCurrentWeather response type mismatch: got %T", got)
				}
				if diff := cmp.Diff(tt.wantResp, gotResp); diff != "" {
					t.Fatalf("GetCurrentWeather response mismatch (-want +got):\n%s", diff)
				}
				return
			}

			if got != nil {
				t.Fatalf("GetCurrentWeather response mismatch (-want +got):\n%s", cmp.Diff((*publicoas.CurrentWeatherResponseHeaders)(nil), got))
			}

			var gotErr *publicoas.DefaultErrorStatusCodeWithHeaders
			if !errors.As(err, &gotErr) {
				t.Fatalf("GetCurrentWeather error type mismatch: got %T (%v)", err, err)
			}
			if diff := cmp.Diff(tt.wantErr, gotErr); diff != "" {
				t.Fatalf("GetCurrentWeather error mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBaseServiceNewError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want *publicoas.DefaultErrorStatusCodeWithHeaders
	}{
		{
			name: "maps unexpected error to internal response",
			err:  errors.New("boom"),
			want: &publicoas.DefaultErrorStatusCodeWithHeaders{
				StatusCode: 500,
				Response: publicoas.ErrorResponse{
					Code:    "internal_error",
					Message: "internal server error",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := publicapi.NewService(nil)
			got := svc.NewError(context.Background(), tt.err)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("NewError mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func errorResponse(statusCode int, code, message string) *publicoas.DefaultErrorStatusCodeWithHeaders {
	return &publicoas.DefaultErrorStatusCodeWithHeaders{
		StatusCode: statusCode,
		Response: publicoas.ErrorResponse{
			Code:    code,
			Message: message,
		},
	}
}

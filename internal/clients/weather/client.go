package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/abczzz13/base-api/internal/httpclient"
)

const (
	providerName        = "open-meteo"
	geocodingOperation  = "geocode_weather_location"
	forecastOperation   = "get_current_weather"
	geocodingPath       = "/v1/search"
	forecastPath        = "/v1/forecast"
	openMeteoAPIKeyName = "apikey" //nolint:gosec // Query parameter name, not a credential value.
)

// Client fetches current weather data from a configured provider.
type Client interface {
	GetCurrent(ctx context.Context, location string) (CurrentWeather, error)
}

// ClientFunc adapts a function to Client.
type ClientFunc func(context.Context, string) (CurrentWeather, error)

// GetCurrent implements Client.
func (f ClientFunc) GetCurrent(ctx context.Context, location string) (CurrentWeather, error) {
	if f == nil {
		return CurrentWeather{}, errors.New("weather client is required")
	}

	return f(ctx, location)
}

// CurrentWeather contains normalized weather conditions.
type CurrentWeather struct {
	Provider     string
	Location     string
	Condition    string
	TemperatureC float64
	ObservedAt   time.Time
}

// UpstreamError reports a non-success weather provider response.
type UpstreamError struct {
	StatusCode int
}

func (e *UpstreamError) Error() string {
	if e == nil {
		return "weather upstream error"
	}

	return fmt.Sprintf("weather provider returned status %d", e.StatusCode)
}

// DecodeError reports a malformed weather provider payload.
type DecodeError struct {
	Err error
}

func (e *DecodeError) Error() string {
	if e == nil || e.Err == nil {
		return "decode weather response"
	}

	return fmt.Sprintf("decode weather response: %v", e.Err)
}

// Unwrap returns the underlying decode error.
func (e *DecodeError) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Err
}

// NotFoundError reports when a requested location cannot be resolved.
type NotFoundError struct {
	Location string
}

func (e *NotFoundError) Error() string {
	if e == nil {
		return "weather location not found"
	}

	return fmt.Sprintf("weather location %q not found", e.Location)
}

type Service struct {
	geocodingClient *httpclient.Service
	forecastClient  *httpclient.Service
	apiKey          string
	timeout         time.Duration
}

// New creates an Open-Meteo weather client backed by httpclient.
func New(geocodingClient, forecastClient *httpclient.Service, apiKey string, timeout time.Duration) (*Service, error) {
	if geocodingClient == nil {
		return nil, errors.New("weather geocoding client is required")
	}
	if forecastClient == nil {
		return nil, errors.New("weather forecast client is required")
	}
	if timeout <= 0 {
		return nil, errors.New("weather timeout must be positive")
	}

	return &Service{
		geocodingClient: geocodingClient,
		forecastClient:  forecastClient,
		apiKey:          strings.TrimSpace(apiKey),
		timeout:         timeout,
	}, nil
}

type geocodingResponse struct {
	Results []struct {
		Name      *string  `json:"name"`
		Country   *string  `json:"country"`
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	} `json:"results"`
}

type forecastResponse struct {
	Current *struct {
		Time         *int64   `json:"time"`
		TemperatureC *float64 `json:"temperature_2m"`
		WeatherCode  *int     `json:"weather_code"`
	} `json:"current"`
}

// GetCurrent fetches current weather conditions for location.
func (s *Service) GetCurrent(ctx context.Context, location string) (CurrentWeather, error) {
	if s == nil || s.geocodingClient == nil || s.forecastClient == nil {
		return CurrentWeather{}, errors.New("weather client is required")
	}

	trimmedLocation := strings.TrimSpace(location)
	if trimmedLocation == "" {
		return CurrentWeather{}, errors.New("location is required")
	}

	ctx = contextOrBackground(ctx)
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	resolvedLocation, err := s.lookupLocation(ctx, trimmedLocation)
	if err != nil {
		return CurrentWeather{}, err
	}

	return s.lookupCurrentWeather(ctx, resolvedLocation)
}

type resolvedLocation struct {
	Name      string
	Country   string
	Latitude  float64
	Longitude float64
}

func (s *Service) lookupLocation(ctx context.Context, location string) (resolvedLocation, error) {
	req, err := s.geocodingClient.NewRequest(ctx, geocodingOperation, http.MethodGet, geocodingPath, nil)
	if err != nil {
		return resolvedLocation{}, fmt.Errorf("create weather geocoding request: %w", err)
	}

	query := req.URL.Query()
	query.Set("name", location)
	query.Set("count", "1")
	query.Set("language", "en")
	query.Set("format", "json")
	s.setAPIKey(query)
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Accept", "application/json")

	resp, err := s.geocodingClient.Do(req)
	if err != nil {
		return resolvedLocation{}, fmt.Errorf("request weather geocoding provider: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return resolvedLocation{}, &UpstreamError{StatusCode: resp.StatusCode}
	}

	var payload geocodingResponse
	if err := decodeAndDrainJSON(resp.Body, &payload); err != nil {
		return resolvedLocation{}, &DecodeError{Err: err}
	}
	if len(payload.Results) == 0 {
		return resolvedLocation{}, &NotFoundError{Location: location}
	}

	result := payload.Results[0]
	if result.Name == nil || strings.TrimSpace(*result.Name) == "" {
		return resolvedLocation{}, &DecodeError{Err: errors.New("results[0].name is required")}
	}
	if result.Latitude == nil {
		return resolvedLocation{}, &DecodeError{Err: errors.New("results[0].latitude is required")}
	}
	if result.Longitude == nil {
		return resolvedLocation{}, &DecodeError{Err: errors.New("results[0].longitude is required")}
	}

	country := ""
	if result.Country != nil {
		country = *result.Country
	}

	return resolvedLocation{
		Name:      *result.Name,
		Country:   country,
		Latitude:  *result.Latitude,
		Longitude: *result.Longitude,
	}, nil
}

func (s *Service) lookupCurrentWeather(ctx context.Context, location resolvedLocation) (CurrentWeather, error) {
	req, err := s.forecastClient.NewRequest(ctx, forecastOperation, http.MethodGet, forecastPath, nil)
	if err != nil {
		return CurrentWeather{}, fmt.Errorf("create weather forecast request: %w", err)
	}

	query := req.URL.Query()
	query.Set("latitude", strconv.FormatFloat(location.Latitude, 'f', -1, 64))
	query.Set("longitude", strconv.FormatFloat(location.Longitude, 'f', -1, 64))
	query.Set("current", "temperature_2m,weather_code")
	query.Set("timeformat", "unixtime")
	s.setAPIKey(query)
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Accept", "application/json")

	resp, err := s.forecastClient.Do(req)
	if err != nil {
		return CurrentWeather{}, fmt.Errorf("request weather forecast provider: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return CurrentWeather{}, &UpstreamError{StatusCode: resp.StatusCode}
	}

	var payload forecastResponse
	if err := decodeAndDrainJSON(resp.Body, &payload); err != nil {
		return CurrentWeather{}, &DecodeError{Err: err}
	}
	if payload.Current == nil {
		return CurrentWeather{}, &DecodeError{Err: errors.New("current is required")}
	}
	if payload.Current.Time == nil || *payload.Current.Time == 0 {
		return CurrentWeather{}, &DecodeError{Err: errors.New("current.time is required")}
	}
	if payload.Current.TemperatureC == nil {
		return CurrentWeather{}, &DecodeError{Err: errors.New("current.temperature_2m is required")}
	}
	if payload.Current.WeatherCode == nil {
		return CurrentWeather{}, &DecodeError{Err: errors.New("current.weather_code is required")}
	}

	return CurrentWeather{
		Provider:     providerName,
		Location:     displayLocation(location),
		Condition:    weatherCodeDescription(*payload.Current.WeatherCode),
		TemperatureC: *payload.Current.TemperatureC,
		ObservedAt:   time.Unix(*payload.Current.Time, 0).UTC(),
	}, nil
}

func (s *Service) setAPIKey(query mapSetter) {
	if strings.TrimSpace(s.apiKey) == "" {
		return
	}

	query.Set(openMeteoAPIKeyName, s.apiKey)
}

func decodeAndDrainJSON(body io.Reader, target any) error {
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(target); err != nil {
		return err
	}

	if _, err := io.Copy(io.Discard, body); err != nil {
		return fmt.Errorf("drain response body: %w", err)
	}

	return nil
}

type mapSetter interface {
	Set(key, value string)
}

func displayLocation(location resolvedLocation) string {
	if strings.TrimSpace(location.Country) == "" {
		return location.Name
	}

	return location.Name + ", " + location.Country
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}

	return ctx
}

func weatherCodeDescription(code int) string {
	switch code {
	case 0:
		return "Clear sky"
	case 1:
		return "Mainly clear"
	case 2:
		return "Partly cloudy"
	case 3:
		return "Overcast"
	case 45, 48:
		return "Fog"
	case 51, 53, 55:
		return "Drizzle"
	case 56, 57:
		return "Freezing drizzle"
	case 61, 63, 65:
		return "Rain"
	case 66, 67:
		return "Freezing rain"
	case 71, 73, 75, 77:
		return "Snow"
	case 80, 81, 82:
		return "Rain showers"
	case 85, 86:
		return "Snow showers"
	case 95:
		return "Thunderstorm"
	case 96, 99:
		return "Thunderstorm with hail"
	default:
		return fmt.Sprintf("Weather code %d", code)
	}
}

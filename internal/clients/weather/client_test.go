package weather

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/abczzz13/base-api/internal/httpclient"
	"github.com/abczzz13/base-api/internal/outboundaudit"
	"github.com/abczzz13/base-api/internal/requestid"
)

func TestServiceGetCurrent(t *testing.T) {
	forecastAuditRepo := &recordingAuditRepository{}
	forecastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diff := cmp.Diff(http.MethodGet, r.Method); diff != "" {
			t.Fatalf("method mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("52.3676", r.URL.Query().Get("latitude")); diff != "" {
			t.Fatalf("latitude mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("4.9041", r.URL.Query().Get("longitude")); diff != "" {
			t.Fatalf("longitude mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("temperature_2m,weather_code", r.URL.Query().Get("current")); diff != "" {
			t.Fatalf("current query mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("secret", r.URL.Query().Get(openMeteoAPIKeyName)); diff != "" {
			t.Fatalf("apikey query mismatch (-want +got):\n%s", diff)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"current":{"time":1772884800,"temperature_2m":12.5,"weather_code":3}}`))
	}))
	t.Cleanup(forecastServer.Close)

	geocodingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diff := cmp.Diff(http.MethodGet, r.Method); diff != "" {
			t.Fatalf("method mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("Amsterdam", r.URL.Query().Get("name")); diff != "" {
			t.Fatalf("name query mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("1", r.URL.Query().Get("count")); diff != "" {
			t.Fatalf("count query mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("en", r.URL.Query().Get("language")); diff != "" {
			t.Fatalf("language query mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("json", r.URL.Query().Get("format")); diff != "" {
			t.Fatalf("format query mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("secret", r.URL.Query().Get(openMeteoAPIKeyName)); diff != "" {
			t.Fatalf("apikey query mismatch (-want +got):\n%s", diff)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"name":"Amsterdam","country":"Netherlands","latitude":52.3676,"longitude":4.9041}]}`))
	}))
	t.Cleanup(geocodingServer.Close)

	geocodingHTTPClient, err := httpclient.New(httpclient.Config{
		Client:  providerName + "_geocoding",
		BaseURL: geocodingServer.URL,
	})
	if err != nil {
		t.Fatalf("create geocoding client: %v", err)
	}

	forecastHTTPClient, err := httpclient.New(httpclient.Config{
		Client:          providerName + "_forecast",
		BaseURL:         forecastServer.URL,
		AuditRepository: forecastAuditRepo,
	})
	if err != nil {
		t.Fatalf("create forecast client: %v", err)
	}

	client, err := New(geocodingHTTPClient, forecastHTTPClient, "secret", time.Second)
	if err != nil {
		t.Fatalf("create weather client: %v", err)
	}

	ctx := requestid.WithContext(context.Background(), "req-123")
	got, err := client.GetCurrent(ctx, "Amsterdam")
	if err != nil {
		t.Fatalf("GetCurrent returned error: %v", err)
	}

	want := CurrentWeather{
		Provider:     providerName,
		Location:     "Amsterdam, Netherlands",
		Condition:    "Overcast",
		TemperatureC: 12.5,
		ObservedAt:   time.Date(2026, time.March, 7, 12, 0, 0, 0, time.UTC),
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("current weather mismatch (-want +got):\n%s", diff)
	}

	if gotCount, wantCount := len(forecastAuditRepo.records), 1; gotCount != wantCount {
		t.Fatalf("audit record count mismatch: want %d, got %d", wantCount, gotCount)
	}

	record := forecastAuditRepo.records[0]
	if diff := cmp.Diff("req-123", record.RequestID); diff != "" {
		t.Fatalf("request ID mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff("apikey=%5BREDACTED%5D&current=temperature_2m%2Cweather_code&latitude=52.3676&longitude=4.9041&timeformat=unixtime", record.Query); diff != "" {
		t.Fatalf("forecast query mismatch (-want +got):\n%s", diff)
	}
	if record.ResponseBodyTruncated {
		t.Fatal("expected forecast response body to be fully captured")
	}
	if diff := cmp.Diff(
		map[string]any{"current": map[string]any{"time": float64(1772884800), "temperature_2m": 12.5, "weather_code": float64(3)}},
		decodeJSONBody(t, record.ResponseBody),
	); diff != "" {
		t.Fatalf("forecast response body mismatch (-want +got):\n%s", diff)
	}
}

func TestServiceGetCurrentReturnsNotFoundWhenLocationCannotBeResolved(t *testing.T) {
	geocodingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	t.Cleanup(geocodingServer.Close)

	geocodingHTTPClient, err := httpclient.New(httpclient.Config{Client: providerName + "_geocoding", BaseURL: geocodingServer.URL})
	if err != nil {
		t.Fatalf("create geocoding client: %v", err)
	}
	forecastHTTPClient, err := httpclient.New(httpclient.Config{Client: providerName + "_forecast", BaseURL: geocodingServer.URL})
	if err != nil {
		t.Fatalf("create forecast client: %v", err)
	}

	client, err := New(geocodingHTTPClient, forecastHTTPClient, "", time.Second)
	if err != nil {
		t.Fatalf("create weather client: %v", err)
	}

	_, err = client.GetCurrent(context.Background(), "Atlantis")
	var notFoundErr *NotFoundError
	if !errors.As(err, &notFoundErr) {
		t.Fatalf("expected NotFoundError, got %T (%v)", err, err)
	}
	if diff := cmp.Diff("Atlantis", notFoundErr.Location); diff != "" {
		t.Fatalf("location mismatch (-want +got):\n%s", diff)
	}
}

func TestServiceGetCurrentReturnsUpstreamError(t *testing.T) {
	geocodingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream down", http.StatusBadGateway)
	}))
	t.Cleanup(geocodingServer.Close)

	geocodingHTTPClient, err := httpclient.New(httpclient.Config{Client: providerName + "_geocoding", BaseURL: geocodingServer.URL})
	if err != nil {
		t.Fatalf("create geocoding client: %v", err)
	}
	forecastHTTPClient, err := httpclient.New(httpclient.Config{Client: providerName + "_forecast", BaseURL: geocodingServer.URL})
	if err != nil {
		t.Fatalf("create forecast client: %v", err)
	}

	client, err := New(geocodingHTTPClient, forecastHTTPClient, "", time.Second)
	if err != nil {
		t.Fatalf("create weather client: %v", err)
	}

	_, err = client.GetCurrent(context.Background(), "Amsterdam")
	var upstreamErr *UpstreamError
	if !errors.As(err, &upstreamErr) {
		t.Fatalf("expected UpstreamError, got %T (%v)", err, err)
	}
	if diff := cmp.Diff(http.StatusBadGateway, upstreamErr.StatusCode); diff != "" {
		t.Fatalf("status code mismatch (-want +got):\n%s", diff)
	}
}

func TestServiceGetCurrentReturnsDecodeErrorForMalformedGeocodingResponse(t *testing.T) {
	tests := []struct {
		name          string
		geocodingBody string
		wantContains  string
	}{
		{
			name:          "missing latitude",
			geocodingBody: `{"results":[{"name":"Amsterdam","country":"Netherlands","longitude":4.9041}]}`,
			wantContains:  "results[0].latitude is required",
		},
		{
			name:          "missing longitude",
			geocodingBody: `{"results":[{"name":"Amsterdam","country":"Netherlands","latitude":52.3676}]}`,
			wantContains:  "results[0].longitude is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var forecastRequests atomic.Int32
			forecastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				forecastRequests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"current":{"time":1772884800,"temperature_2m":12.5,"weather_code":3}}`))
			}))
			defer forecastServer.Close()

			geocodingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.geocodingBody))
			}))
			defer geocodingServer.Close()

			client := newTestService(t, geocodingServer.URL, forecastServer.URL, "", time.Second, nil)

			_, err := client.GetCurrent(context.Background(), "Amsterdam")
			var decodeErr *DecodeError
			if !errors.As(err, &decodeErr) {
				t.Fatalf("expected DecodeError, got %T (%v)", err, err)
			}
			if !strings.Contains(decodeErr.Error(), tt.wantContains) {
				t.Fatalf("expected decode error to mention %q, got %q", tt.wantContains, decodeErr.Error())
			}
			if diff := cmp.Diff(int32(0), forecastRequests.Load()); diff != "" {
				t.Fatalf("forecast request count mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestServiceGetCurrentReturnsDecodeErrorForMalformedForecastResponse(t *testing.T) {
	tests := []struct {
		name         string
		forecastBody string
		wantContains string
	}{
		{
			name:         "missing current object",
			forecastBody: `{}`,
			wantContains: "current is required",
		},
		{
			name:         "missing time",
			forecastBody: `{"current":{"temperature_2m":12.5,"weather_code":3}}`,
			wantContains: "current.time is required",
		},
		{
			name:         "missing temperature",
			forecastBody: `{"current":{"time":1772884800,"weather_code":3}}`,
			wantContains: "current.temperature_2m is required",
		},
		{
			name:         "missing weather code",
			forecastBody: `{"current":{"time":1772884800,"temperature_2m":12.5}}`,
			wantContains: "current.weather_code is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			forecastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.forecastBody))
			}))
			defer forecastServer.Close()

			geocodingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"results":[{"name":"Amsterdam","country":"Netherlands","latitude":52.3676,"longitude":4.9041}]}`))
			}))
			defer geocodingServer.Close()

			client := newTestService(t, geocodingServer.URL, forecastServer.URL, "", time.Second, nil)

			_, err := client.GetCurrent(context.Background(), "Amsterdam")
			var decodeErr *DecodeError
			if !errors.As(err, &decodeErr) {
				t.Fatalf("expected DecodeError, got %T (%v)", err, err)
			}
			if !strings.Contains(decodeErr.Error(), tt.wantContains) {
				t.Fatalf("expected decode error to mention %q, got %q", tt.wantContains, decodeErr.Error())
			}
		})
	}
}

func TestServiceGetCurrentUsesSingleTimeoutBudget(t *testing.T) {
	forecastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(60 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"current":{"time":1772884800,"temperature_2m":12.5,"weather_code":3}}`))
	}))
	defer forecastServer.Close()

	geocodingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(60 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"name":"Amsterdam","country":"Netherlands","latitude":52.3676,"longitude":4.9041}]}`))
	}))
	defer geocodingServer.Close()

	client := newTestService(t, geocodingServer.URL, forecastServer.URL, "", 70*time.Millisecond, nil)

	startedAt := time.Now()
	_, err := client.GetCurrent(context.Background(), "Amsterdam")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %T (%v)", err, err)
	}

	if elapsed := time.Since(startedAt); elapsed > 100*time.Millisecond {
		t.Fatalf("GetCurrent exceeded single timeout budget: elapsed %s", elapsed)
	}
}

func TestNewValidation(t *testing.T) {
	geocodingClient, err := httpclient.New(httpclient.Config{Client: "geo", BaseURL: "http://localhost"})
	if err != nil {
		t.Fatalf("create geocoding client: %v", err)
	}
	forecastClient, err := httpclient.New(httpclient.Config{Client: "fc", BaseURL: "http://localhost"})
	if err != nil {
		t.Fatalf("create forecast client: %v", err)
	}

	tests := []struct {
		name            string
		geocodingClient *httpclient.Service
		forecastClient  *httpclient.Service
		timeout         time.Duration
		wantErr         string
	}{
		{
			name:            "nil geocoding client",
			geocodingClient: nil,
			forecastClient:  forecastClient,
			timeout:         time.Second,
			wantErr:         "weather geocoding client is required",
		},
		{
			name:            "nil forecast client",
			geocodingClient: geocodingClient,
			forecastClient:  nil,
			timeout:         time.Second,
			wantErr:         "weather forecast client is required",
		},
		{
			name:            "zero timeout",
			geocodingClient: geocodingClient,
			forecastClient:  forecastClient,
			timeout:         0,
			wantErr:         "weather timeout must be positive",
		},
		{
			name:            "negative timeout",
			geocodingClient: geocodingClient,
			forecastClient:  forecastClient,
			timeout:         -time.Second,
			wantErr:         "weather timeout must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.geocodingClient, tt.forecastClient, "", tt.timeout)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if diff := cmp.Diff(tt.wantErr, err.Error()); diff != "" {
				t.Fatalf("error message mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestWeatherCodeDescription(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{0, "Clear sky"},
		{1, "Mainly clear"},
		{2, "Partly cloudy"},
		{3, "Overcast"},
		{45, "Fog"},
		{48, "Fog"},
		{51, "Drizzle"},
		{53, "Drizzle"},
		{55, "Drizzle"},
		{56, "Freezing drizzle"},
		{57, "Freezing drizzle"},
		{61, "Rain"},
		{63, "Rain"},
		{65, "Rain"},
		{66, "Freezing rain"},
		{67, "Freezing rain"},
		{71, "Snow"},
		{73, "Snow"},
		{75, "Snow"},
		{77, "Snow"},
		{80, "Rain showers"},
		{81, "Rain showers"},
		{82, "Rain showers"},
		{85, "Snow showers"},
		{86, "Snow showers"},
		{95, "Thunderstorm"},
		{96, "Thunderstorm with hail"},
		{99, "Thunderstorm with hail"},
		{999, "Weather code 999"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := weatherCodeDescription(tt.code)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("weatherCodeDescription(%d) mismatch (-want +got):\n%s", tt.code, diff)
			}
		})
	}
}

func TestServiceGetCurrentEmptyLocation(t *testing.T) {
	geocodingClient, err := httpclient.New(httpclient.Config{Client: "geo", BaseURL: "http://localhost"})
	if err != nil {
		t.Fatalf("create geocoding client: %v", err)
	}
	forecastClient, err := httpclient.New(httpclient.Config{Client: "fc", BaseURL: "http://localhost"})
	if err != nil {
		t.Fatalf("create forecast client: %v", err)
	}

	svc, err := New(geocodingClient, forecastClient, "", time.Second)
	if err != nil {
		t.Fatalf("create weather client: %v", err)
	}

	tests := []struct {
		name     string
		location string
	}{
		{"empty string", ""},
		{"spaces only", "  "},
		{"tab only", "\t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.GetCurrent(context.Background(), tt.location)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if diff := cmp.Diff("location is required", err.Error()); diff != "" {
				t.Fatalf("error message mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClientFuncNilReturnsError(t *testing.T) {
	var fn ClientFunc
	_, err := fn.GetCurrent(context.Background(), "Amsterdam")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if diff := cmp.Diff("weather client is required", err.Error()); diff != "" {
		t.Fatalf("error message mismatch (-want +got):\n%s", diff)
	}
}

func newTestService(
	t *testing.T,
	geocodingBaseURL string,
	forecastBaseURL string,
	apiKey string,
	timeout time.Duration,
	forecastAuditRepo outboundaudit.Repository,
) *Service {
	t.Helper()

	geocodingHTTPClient, err := httpclient.New(httpclient.Config{Client: providerName + "_geocoding", BaseURL: geocodingBaseURL})
	if err != nil {
		t.Fatalf("create geocoding client: %v", err)
	}

	forecastHTTPClient, err := httpclient.New(httpclient.Config{
		Client:          providerName + "_forecast",
		BaseURL:         forecastBaseURL,
		AuditRepository: forecastAuditRepo,
	})
	if err != nil {
		t.Fatalf("create forecast client: %v", err)
	}

	client, err := New(geocodingHTTPClient, forecastHTTPClient, apiKey, timeout)
	if err != nil {
		t.Fatalf("create weather client: %v", err)
	}

	return client
}

type recordingAuditRepository struct {
	records []outboundaudit.Record
}

func (repo *recordingAuditRepository) StoreOutboundAudit(_ context.Context, record outboundaudit.Record) error {
	repo.records = append(repo.records, record)
	return nil
}

func decodeJSONBody(t *testing.T, body []byte) map[string]any {
	t.Helper()

	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal JSON body: %v", err)
	}

	return decoded
}

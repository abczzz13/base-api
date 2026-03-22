package publicapi

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/abczzz13/base-api/internal/clients/weather"
	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/logger"
	"github.com/abczzz13/base-api/internal/middleware"
	"github.com/abczzz13/base-api/internal/notes"
	"github.com/abczzz13/base-api/internal/ratelimit"
	"github.com/abczzz13/base-api/internal/requestaudit"
)

func TestNewPublicHandlerCORSAndCSRFInteraction(t *testing.T) {
	t.Run("allows preflight for configured origin", func(t *testing.T) {
		handler, err := newPublicHandlerForTest(t, config.Config{
			Environment: "test",
			CORS: config.CORSConfig{
				AllowedOrigins: []string{"https://client.example"},
			},
			CSRF: config.CSRFConfig{Enabled: true},
		})
		if err != nil {
			t.Fatalf("NewHandler returned error: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/healthz", nil)
		req.Header.Set("Origin", "https://client.example")
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
		req.Header.Set("Access-Control-Request-Headers", "Content-Type")

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Fatalf("status mismatch: want %d, got %d", http.StatusNoContent, rr.Code)
		}

		for headerName, wantValue := range map[string]string{
			"Access-Control-Allow-Origin":  "https://client.example",
			"Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, PATCH, OPTIONS",
			"Access-Control-Allow-Headers": "Content-Type, Authorization, Accept",
		} {
			if got := rr.Header().Get(headerName); got != wantValue {
				t.Fatalf("header %q mismatch: want %q, got %q", headerName, wantValue, got)
			}
		}
	})

	t.Run("denies unsafe cross-origin request from untrusted origin", func(t *testing.T) {
		handler, err := newPublicHandlerForTest(t, config.Config{
			Environment: "test",
			CORS: config.CORSConfig{
				AllowedOrigins: []string{"https://trusted.example"},
			},
			CSRF: config.CSRFConfig{
				Enabled:        true,
				TrustedOrigins: []string{"https://trusted.example"},
			},
		})
		if err != nil {
			t.Fatalf("NewHandler returned error: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
		req.Header.Set("Origin", "https://evil.example")
		req.Header.Set("Sec-Fetch-Site", "cross-site")

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("status mismatch: want %d, got %d", http.StatusForbidden, rr.Code)
		}
		if got := rr.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
			t.Fatalf("content-type mismatch: want %q, got %q", "application/json; charset=utf-8", got)
		}
		if got := rr.Header().Get("X-Request-Id"); got == "" {
			t.Fatal("expected non-empty X-Request-Id response header")
		}
		if got := rr.Body.String(); !strings.Contains(got, `"code":"forbidden"`) || !strings.Contains(got, `"message":"cross-origin request denied"`) || !strings.Contains(got, `"requestId":"`) {
			t.Fatalf("body mismatch: got %q", got)
		}
	})

	t.Run("trusted cross-origin requests pass CSRF layer", func(t *testing.T) {
		baseCfg := config.Config{
			Environment: "test",
			CORS: config.CORSConfig{
				AllowedOrigins: []string{"https://trusted.example"},
			},
		}

		handlerWithoutCSRF, err := newPublicHandlerForTest(t, baseCfg)
		if err != nil {
			t.Fatalf("NewHandler without CSRF returned error: %v", err)
		}

		cfgWithCSRF := baseCfg
		cfgWithCSRF.CSRF = config.CSRFConfig{
			Enabled:        true,
			TrustedOrigins: []string{"https://trusted.example"},
		}
		handlerWithCSRF, err := newPublicHandlerForTest(t, cfgWithCSRF)
		if err != nil {
			t.Fatalf("NewHandler with CSRF returned error: %v", err)
		}

		newCrossSiteRequest := func() *http.Request {
			req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
			req.Header.Set("Origin", "https://trusted.example")
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			return req
		}

		rrWithoutCSRF := httptest.NewRecorder()
		handlerWithoutCSRF.ServeHTTP(rrWithoutCSRF, newCrossSiteRequest())

		if rrWithoutCSRF.Code == http.StatusForbidden {
			t.Fatalf("expected baseline request without CSRF to be non-forbidden")
		}

		rrWithCSRF := httptest.NewRecorder()
		handlerWithCSRF.ServeHTTP(rrWithCSRF, newCrossSiteRequest())

		if rrWithCSRF.Code == http.StatusForbidden {
			t.Fatalf("expected trusted origin request to pass CSRF layer")
		}
		if rrWithCSRF.Code != rrWithoutCSRF.Code {
			t.Fatalf("status mismatch with trusted CSRF origin: want %d, got %d", rrWithoutCSRF.Code, rrWithCSRF.Code)
		}
		if rrWithCSRF.Body.String() != rrWithoutCSRF.Body.String() {
			t.Fatalf("body mismatch with trusted CSRF origin: want %q, got %q", rrWithoutCSRF.Body.String(), rrWithCSRF.Body.String())
		}
	})
}

func TestNewPublicHandlerTracingIncludesTraceContextInLogs(t *testing.T) {
	previousLogger := slog.Default()
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})

	var logs bytes.Buffer
	logger.New(logger.Config{
		Format:      logger.FormatJSON,
		Level:       slog.LevelDebug,
		Environment: "test",
		Writer:      &logs,
	})

	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
	})
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	handler, err := newPublicHandlerForTest(t, config.Config{
		Environment: "test",
		OTEL: config.OTELConfig{
			TracingEnabled: true,
		},
	})
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusOK, rr.Code)
	}
	if got := rr.Header().Get(middleware.TraceIDResponseHeader); got == "" {
		t.Fatalf("expected non-empty %s response header", middleware.TraceIDResponseHeader)
	}

	entries := decodeJSONLogLines(t, logs.String())
	var completed map[string]any
	for _, entry := range entries {
		if msg, _ := entry["msg"].(string); msg == "request completed" {
			completed = entry
			break
		}
	}
	if completed == nil {
		t.Fatalf("expected request completed log entry, got %#v", entries)
	}

	traceID, ok := completed["trace_id"].(string)
	if !ok || traceID == "" {
		t.Fatalf("expected non-empty trace_id in request log, got %#v", completed["trace_id"])
	}
	spanID, ok := completed["span_id"].(string)
	if !ok || spanID == "" {
		t.Fatalf("expected non-empty span_id in request log, got %#v", completed["span_id"])
	}
}

func TestNewPublicHandlerTracingAddsOperationAttributesToSpans(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
	})
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	handler, err := newPublicHandlerForTest(t, config.Config{
		Environment: "test",
		OTEL: config.OTELConfig{
			TracingEnabled: true,
		},
	})
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusOK, rr.Code)
	}

	spans := recorder.Ended()
	if len(spans) == 0 {
		t.Fatal("expected at least one ended span")
	}

	matches := make([]sdktrace.ReadOnlySpan, 0, len(spans))
	for _, span := range spans {
		for _, attr := range span.Attributes() {
			if string(attr.Key) == "api.operation.name" && attr.Value.Type() == attribute.STRING && attr.Value.AsString() == "GetHealthz" {
				matches = append(matches, span)
				break
			}
		}
	}

	if len(matches) != 1 {
		names := make([]string, 0, len(spans))
		for _, span := range spans {
			names = append(names, span.Name())
		}
		t.Fatalf("expected exactly one span with api.operation.name=GetHealthz, got %d spans (names=%v)", len(matches), names)
	}

	targetSpan := matches[0]
	if got := targetSpan.Name(); got != "GET GetHealthz" {
		t.Fatalf("span name mismatch: want %q, got %q", "GET GetHealthz", got)
	}

	attrs := map[string]string{}
	for _, attr := range targetSpan.Attributes() {
		if attr.Value.Type() != attribute.STRING {
			continue
		}
		attrs[string(attr.Key)] = attr.Value.AsString()
	}

	for key, want := range map[string]string{
		"api.operation.name":    "GetHealthz",
		"api.operation.summary": "Public health endpoint",
	} {
		if got := attrs[key]; got != want {
			t.Fatalf("span attribute %q mismatch: want %q, got %q", key, want, got)
		}
	}
}

func TestNewPublicHandlerWeatherEndpointUsesInjectedClient(t *testing.T) {
	handler, err := newPublicHandlerForTestWithDependencies(t, config.Config{Environment: "test"}, Dependencies{
		WeatherClient: weather.ClientFunc(func(ctx context.Context, location string) (weather.CurrentWeather, error) {
			if diff := cmp.Diff("Amsterdam", location); diff != "" {
				t.Fatalf("location mismatch (-want +got):\n%s", diff)
			}

			return weather.CurrentWeather{
				Provider:     "open-meteo",
				Location:     "Amsterdam",
				Condition:    "Cloudy",
				TemperatureC: 12.5,
				ObservedAt:   time.Date(2026, time.March, 7, 12, 0, 0, 0, time.UTC),
			}, nil
		}),
	})
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/weather/current?location=Amsterdam", nil)
	req.Header.Set("X-Request-Id", "req-123")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusOK, rr.Code)
	}
	if got := rr.Header().Get("X-Request-Id"); got != "req-123" {
		t.Fatalf("X-Request-Id header mismatch: want %q, got %q", "req-123", got)
	}
	for _, want := range []string{
		`"provider":"open-meteo"`,
		`"location":"Amsterdam"`,
		`"condition":"Cloudy"`,
		`"temperatureC":12.5`,
		`"observedAt":"2026-03-07T12:00:00Z"`,
	} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Fatalf("response body %q does not contain %q", rr.Body.String(), want)
		}
	}
}

func TestNewPublicHandlerWeatherMetricsAndTracingUseWeatherOperationName(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})

	registry := prometheus.NewRegistry()
	requestMetrics, err := middleware.NewHTTPRequestMetrics(registry)
	if err != nil {
		t.Fatalf("create request metrics: %v", err)
	}

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
	})
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	handler, err := NewHandler(config.Config{
		Environment: "test",
		OTEL: config.OTELConfig{
			TracingEnabled: true,
		},
	}, Dependencies{
		RequestMetrics:         requestMetrics,
		RequestAuditRepository: requestaudit.NopRepository(),
		NotesRepository:        notes.RepositoryFuncs{},
		WeatherClient: weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
			return weather.CurrentWeather{
				Provider:     "open-meteo",
				Location:     "Amsterdam",
				Condition:    "Cloudy",
				TemperatureC: 12.5,
				ObservedAt:   time.Date(2026, time.March, 7, 12, 0, 0, 0, time.UTC),
			}, nil
		}),
	})
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/weather/current?location=Amsterdam", nil)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusOK, rr.Code)
	}

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather prometheus metrics: %v", err)
	}

	requestsFamily, ok := metricFamilyByName(families, "base_api_http_requests_total")
	if !ok {
		t.Fatal("base_api_http_requests_total metric family not found")
	}
	if !metricFamilyHasLabels(requestsFamily, map[string]string{
		"server":      "public",
		"method":      http.MethodGet,
		"route":       "GetCurrentWeather",
		"status_code": "200",
	}) {
		t.Fatal("expected weather request metric with route=GetCurrentWeather")
	}

	spans := recorder.Ended()
	matches := make([]sdktrace.ReadOnlySpan, 0, len(spans))
	for _, span := range spans {
		for _, attr := range span.Attributes() {
			if string(attr.Key) == "api.operation.name" && attr.Value.Type() == attribute.STRING && attr.Value.AsString() == "GetCurrentWeather" {
				matches = append(matches, span)
				break
			}
		}
	}

	if len(matches) != 1 {
		t.Fatalf("expected exactly one span with api.operation.name=GetCurrentWeather, got %d", len(matches))
	}
	if got := matches[0].Name(); got != "GET GetCurrentWeather" {
		t.Fatalf("span name mismatch: want %q, got %q", "GET GetCurrentWeather", got)
	}
}

func TestNewPublicHandlerWeatherRateLimitUsesSharedMiddlewareOnce(t *testing.T) {
	registry := prometheus.NewRegistry()
	requestMetrics, err := middleware.NewHTTPRequestMetrics(registry)
	if err != nil {
		t.Fatalf("create request metrics: %v", err)
	}

	rateLimitCalls := 0
	handler, err := NewHandler(config.Config{
		Environment: "test",
		RateLimit: config.RateLimitConfig{
			Enabled:       true,
			FailOpen:      true,
			Timeout:       50 * time.Millisecond,
			DefaultPolicy: ratelimit.Policy{RequestsPerSecond: 1, Burst: 1},
			RouteOverrides: map[string]ratelimit.RouteOverride{
				"GetCurrentWeather": {Burst: intPtr(1), RequestsPerSecond: float64Ptr(1)},
			},
		},
	}, Dependencies{
		RequestMetrics:         requestMetrics,
		RequestAuditRepository: requestaudit.NopRepository(),
		NotesRepository:        notes.RepositoryFuncs{},
		RateLimiter: ratelimit.StoreFunc(func(ctx context.Context, key string, policy ratelimit.Policy) (ratelimit.Decision, error) {
			rateLimitCalls++
			if got, want := key, "public:GetCurrentWeather:192.0.2.10"; got != want {
				t.Fatalf("key mismatch: want %q, got %q", want, got)
			}
			if diff := cmp.Diff(ratelimit.Policy{RequestsPerSecond: 1, Burst: 1}, policy); diff != "" {
				t.Fatalf("policy mismatch (-want +got):\n%s", diff)
			}
			return ratelimit.Decision{Allowed: true}, nil
		}),
		WeatherClient: weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
			return weather.CurrentWeather{
				Provider:     "open-meteo",
				Location:     "Amsterdam",
				Condition:    "Cloudy",
				TemperatureC: 12.5,
				ObservedAt:   time.Date(2026, time.March, 7, 12, 0, 0, 0, time.UTC),
			}, nil
		}),
	})
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/weather/current?location=Amsterdam", nil)
	req.RemoteAddr = "192.0.2.10:43123"
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusOK, rr.Code)
	}
	if got := rateLimitCalls; got != 1 {
		t.Fatalf("rate limit call count mismatch: want %d, got %d", 1, got)
	}
}

func TestNewPublicHandlerWeatherClientRequiresDependency(t *testing.T) {
	registry := prometheus.NewRegistry()
	requestMetrics, err := middleware.NewHTTPRequestMetrics(registry)
	if err != nil {
		t.Fatalf("create request metrics: %v", err)
	}

	_, err = NewHandler(config.Config{Environment: "test"}, Dependencies{
		RequestMetrics:         requestMetrics,
		RequestAuditRepository: requestaudit.NopRepository(),
		NotesRepository:        notes.RepositoryFuncs{},
	})
	if err == nil {
		t.Fatal("expected missing weather client dependency error")
	}
	if got := err.Error(); got != "weather client dependency is required" {
		t.Fatalf("error mismatch: want %q, got %q", "weather client dependency is required", got)
	}
}

func TestNewPublicHandlerRateLimitRequiresDependencyWhenEnabled(t *testing.T) {
	_, err := newPublicHandlerForTestWithDependencies(t, config.Config{
		Environment: "test",
		RateLimit: config.RateLimitConfig{
			Enabled: true,
			DefaultPolicy: ratelimit.Policy{
				RequestsPerSecond: 1,
				Burst:             1,
			},
		},
	}, Dependencies{})
	if err == nil {
		t.Fatal("expected missing rate limiter dependency error")
	}
	if got := err.Error(); got != "rate limiter dependency is required" {
		t.Fatalf("error mismatch: want %q, got %q", "rate limiter dependency is required", got)
	}
}

func TestNewPublicHandlerRateLimitRejectsRequests(t *testing.T) {
	handler, err := newPublicHandlerForTestWithDependencies(t, config.Config{
		Environment: "test",
		CORS: config.CORSConfig{
			AllowedOrigins: []string{"https://client.example"},
		},
		RateLimit: config.RateLimitConfig{
			Enabled:       true,
			FailOpen:      true,
			Timeout:       50 * time.Millisecond,
			DefaultPolicy: ratelimit.Policy{RequestsPerSecond: 1, Burst: 1},
			RouteOverrides: map[string]ratelimit.RouteOverride{
				"GetHealthz": {Burst: intPtr(1), RequestsPerSecond: float64Ptr(1)},
			},
		},
	}, Dependencies{
		RateLimiter: ratelimit.StoreFunc(func(ctx context.Context, key string, policy ratelimit.Policy) (ratelimit.Decision, error) {
			if diff := cmp.Diff(ratelimit.Policy{RequestsPerSecond: 1, Burst: 1}, policy); diff != "" {
				t.Fatalf("policy mismatch (-want +got):\n%s", diff)
			}
			return ratelimit.Decision{Allowed: false, RetryAfter: 1500 * time.Millisecond}, nil
		}),
	})
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "https://client.example")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusTooManyRequests, rr.Code)
	}
	if got := rr.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After mismatch: want %q, got %q", "2", got)
	}
	if got := rr.Header().Get("RateLimit"); got != `"default";r=0;t=2` {
		t.Fatalf("RateLimit mismatch: want %q, got %q", `"default";r=0;t=2`, got)
	}
	if got := rr.Header().Get("RateLimit-Policy"); got != `"default";q=1;w=1` {
		t.Fatalf("RateLimit-Policy mismatch: want %q, got %q", `"default";q=1;w=1`, got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://client.example" {
		t.Fatalf("Access-Control-Allow-Origin mismatch: want %q, got %q", "https://client.example", got)
	}
	if got := rr.Header().Get("X-Request-Id"); got == "" {
		t.Fatal("expected non-empty X-Request-Id response header")
	}
	if body := rr.Body.String(); !strings.Contains(body, `"code":"too_many_requests"`) || !strings.Contains(body, `"message":"rate limit exceeded"`) {
		t.Fatalf("body mismatch: got %q", body)
	}
}

func TestNewPublicHandlerRateLimitUsesSharedTrustedProxyCIDRsWhenAuditDisabled(t *testing.T) {
	handler, err := newPublicHandlerForTestWithDependencies(t, config.Config{
		Environment: "test",
		ClientIP: config.ClientIPConfig{
			TrustedProxyCIDRs: []netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")},
		},
		RequestAudit: config.RequestAuditConfig{
			Enabled: boolPtr(false),
		},
		RateLimit: config.RateLimitConfig{
			Enabled:       true,
			FailOpen:      true,
			Timeout:       50 * time.Millisecond,
			DefaultPolicy: ratelimit.Policy{RequestsPerSecond: 1, Burst: 1},
		},
	}, Dependencies{
		RateLimiter: ratelimit.StoreFunc(func(ctx context.Context, key string, policy ratelimit.Policy) (ratelimit.Decision, error) {
			if diff := cmp.Diff(ratelimit.Policy{RequestsPerSecond: 1, Burst: 1}, policy); diff != "" {
				t.Fatalf("policy mismatch (-want +got):\n%s", diff)
			}
			if got, want := key, "public:GetHealthz:8.8.8.8"; got != want {
				t.Fatalf("key mismatch: want %q, got %q", want, got)
			}
			return ratelimit.Decision{Allowed: true}, nil
		}),
	})
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.RemoteAddr = "203.0.113.10:43123"
	req.Header.Set("X-Forwarded-For", "8.8.8.8")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestNewPublicHandlerRateLimitFailsOpen(t *testing.T) {
	handler, err := newPublicHandlerForTestWithDependencies(t, config.Config{
		Environment: "test",
		RateLimit: config.RateLimitConfig{
			Enabled:       true,
			FailOpen:      true,
			Timeout:       50 * time.Millisecond,
			DefaultPolicy: ratelimit.Policy{RequestsPerSecond: 1, Burst: 1},
			RouteOverrides: map[string]ratelimit.RouteOverride{
				"GetHealthz": {Burst: intPtr(1), RequestsPerSecond: float64Ptr(1)},
			},
		},
	}, Dependencies{
		RateLimiter: ratelimit.StoreFunc(func(context.Context, string, ratelimit.Policy) (ratelimit.Decision, error) {
			return ratelimit.Decision{}, errors.New("valkey unavailable")
		}),
	})
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestNewPublicHandlerRateLimitBackendFailureReturnsServiceUnavailable(t *testing.T) {
	handler, err := newPublicHandlerForTestWithDependencies(t, config.Config{
		Environment: "test",
		RateLimit: config.RateLimitConfig{
			Enabled:       true,
			FailOpen:      false,
			Timeout:       50 * time.Millisecond,
			DefaultPolicy: ratelimit.Policy{RequestsPerSecond: 1, Burst: 1},
			RouteOverrides: map[string]ratelimit.RouteOverride{
				"GetHealthz": {Burst: intPtr(1), RequestsPerSecond: float64Ptr(1)},
			},
		},
	}, Dependencies{
		RateLimiter: ratelimit.StoreFunc(func(context.Context, string, ratelimit.Policy) (ratelimit.Decision, error) {
			return ratelimit.Decision{}, errors.New("valkey unavailable")
		}),
	})
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
	if got := rr.Header().Get("Retry-After"); got != "" {
		t.Fatalf("Retry-After mismatch: want empty, got %q", got)
	}
	if got := rr.Header().Get("RateLimit"); got != "" {
		t.Fatalf("RateLimit mismatch: want empty, got %q", got)
	}
	if got := rr.Header().Get("RateLimit-Policy"); got != "" {
		t.Fatalf("RateLimit-Policy mismatch: want empty, got %q", got)
	}
	if body := rr.Body.String(); !strings.Contains(body, `"code":"rate_limit_unavailable"`) || !strings.Contains(body, `"message":"rate limit backend unavailable"`) {
		t.Fatalf("body mismatch: got %q", body)
	}
}

func newPublicHandlerForTest(t *testing.T, cfg config.Config) (http.Handler, error) {
	return newPublicHandlerForTestWithDependencies(t, cfg, Dependencies{})
}

func newPublicHandlerForTestWithDependencies(t *testing.T, cfg config.Config, deps Dependencies) (http.Handler, error) {
	t.Helper()

	registry := prometheus.NewRegistry()
	requestMetrics, err := middleware.NewHTTPRequestMetrics(registry)
	if err != nil {
		t.Fatalf("create request metrics: %v", err)
	}

	deps.RequestMetrics = requestMetrics
	if deps.RequestAuditRepository == nil {
		deps.RequestAuditRepository = requestaudit.NopRepository()
	}
	if deps.NotesRepository == nil {
		deps.NotesRepository = notes.RepositoryFuncs{}
	}
	if deps.WeatherClient == nil {
		deps.WeatherClient = weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
			return weather.CurrentWeather{
				Provider:     "open-meteo",
				Location:     "Amsterdam",
				Condition:    "Cloudy",
				TemperatureC: 12.5,
				ObservedAt:   time.Date(2026, time.March, 7, 12, 0, 0, 0, time.UTC),
			}, nil
		})
	}

	return NewHandler(cfg, deps)
}

func metricFamilyByName(families []*dto.MetricFamily, name string) (*dto.MetricFamily, bool) {
	for _, family := range families {
		if family.GetName() == name {
			return family, true
		}
	}

	return nil, false
}

func metricFamilyHasLabels(family *dto.MetricFamily, labels map[string]string) bool {
	for _, metric := range family.GetMetric() {
		if metricHasLabels(metric, labels) {
			return true
		}
	}

	return false
}

func metricHasLabels(metric *dto.Metric, labels map[string]string) bool {
	for labelName, wantValue := range labels {
		if gotValue, ok := metricLabelValue(metric, labelName); !ok || gotValue != wantValue {
			return false
		}
	}

	return true
}

func metricLabelValue(metric *dto.Metric, labelName string) (string, bool) {
	for _, label := range metric.GetLabel() {
		if label.GetName() == labelName {
			return label.GetValue(), true
		}
	}

	return "", false
}

func float64Ptr(value float64) *float64 {
	return &value
}

func intPtr(value int) *int {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

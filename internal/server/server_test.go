package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/abczzz13/base-api/internal/logger"
	"github.com/abczzz13/base-api/internal/middleware"
)

func TestRunIgnoresStartupWriteErrors(t *testing.T) {
	assertRunHandlesWriterErrors(t, errWriter{err: errors.New("stdout unavailable")}, io.Discard)
}

func TestRunIgnoresShutdownWriteErrors(t *testing.T) {
	assertRunHandlesWriterErrors(t, io.Discard, errWriter{err: errors.New("stderr unavailable")})
}

func TestRunReturnsErrorWhenListenFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := Run(
		ctx,
		nil,
		getenvFromMap(map[string]string{
			"API_ADDR":        "invalid-address",
			"API_INFRA_ADDR":  reserveTCPAddress(t),
			"API_ENVIRONMENT": "test",
		}),
		strings.NewReader(""),
		io.Discard,
		io.Discard,
	)
	if err == nil {
		t.Fatalf("Run returned nil error for invalid listen address")
	}
	if !strings.Contains(err.Error(), "create public listener") {
		t.Fatalf("Run error does not identify public listener failure: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid-address") {
		t.Fatalf("Run error does not include invalid address context: %v", err)
	}
}

func TestRunClosesBoundListenersWhenLaterListenFails(t *testing.T) {
	publicAddr := reserveTCPAddress(t)

	occupiedInfraListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for occupied infra address: %v", err)
	}
	defer func() { _ = occupiedInfraListener.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = Run(
		ctx,
		nil,
		getenvFromMap(map[string]string{
			"API_ADDR":        publicAddr,
			"API_INFRA_ADDR":  occupiedInfraListener.Addr().String(),
			"API_ENVIRONMENT": "test",
		}),
		strings.NewReader(""),
		io.Discard,
		io.Discard,
	)
	if err == nil {
		t.Fatalf("Run returned nil error when infra listen should fail")
	}
	if !strings.Contains(err.Error(), "create infra listener") {
		t.Fatalf("Run error does not identify infra listener failure: %v", err)
	}

	releasedListener, err := net.Listen("tcp", publicAddr)
	if err != nil {
		t.Fatalf("public listener address was not released after startup failure: %v", err)
	}
	_ = releasedListener.Close()
}

func TestRunContinuesWhenTracingInitializationFails(t *testing.T) {
	publicAddr := reserveTCPAddress(t)
	infraAddr := reserveTCPAddress(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var stderr bytes.Buffer
	env := map[string]string{
		"API_ADDR":                           publicAddr,
		"API_INFRA_ADDR":                     infraAddr,
		"API_ENVIRONMENT":                    "test",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "http://127.0.0.1:4318/v1/traces",
		"OTEL_EXPORTER_OTLP_TRACES_PROTOCOL": "http/protobuf",
	}

	err := Run(ctx, nil, getenvFromMap(env), strings.NewReader(""), io.Discard, &stderr)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if !strings.Contains(stderr.String(), "OpenTelemetry tracing disabled") {
		t.Fatalf("expected tracing initialization warning in logs, got: %q", stderr.String())
	}
}

func TestNewHTTPServerUsesConfiguredTimeouts(t *testing.T) {
	cfg := Config{
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       11 * time.Second,
		WriteTimeout:      25 * time.Second,
		IdleTimeout:       45 * time.Second,
	}
	handler := http.NewServeMux()
	addr := "127.0.0.1:8080"

	srv := newHTTPServer(cfg, addr, handler)

	if srv.Addr != addr {
		t.Fatalf("Addr mismatch: want %q, got %q", addr, srv.Addr)
	}
	if srv.Handler != handler {
		t.Fatalf("Handler mismatch: got unexpected handler")
	}
	if srv.ReadHeaderTimeout != cfg.ReadHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout mismatch: want %s, got %s", cfg.ReadHeaderTimeout, srv.ReadHeaderTimeout)
	}
	if srv.ReadTimeout != cfg.ReadTimeout {
		t.Fatalf("ReadTimeout mismatch: want %s, got %s", cfg.ReadTimeout, srv.ReadTimeout)
	}
	if srv.WriteTimeout != cfg.WriteTimeout {
		t.Fatalf("WriteTimeout mismatch: want %s, got %s", cfg.WriteTimeout, srv.WriteTimeout)
	}
	if srv.IdleTimeout != cfg.IdleTimeout {
		t.Fatalf("IdleTimeout mismatch: want %s, got %s", cfg.IdleTimeout, srv.IdleTimeout)
	}
}

func TestNewInfraHandlerRoutesMetricsThroughPromHTTP(t *testing.T) {
	handler, err := newInfraHandlerForTest(t, Config{Environment: "test"})
	if err != nil {
		t.Fatalf("newInfraHandler returned error: %v", err)
	}

	tests := []struct {
		name                    string
		method                  string
		path                    string
		requestHeaders          map[string]string
		wantStatus              int
		wantHeaders             map[string]string
		wantContentTypeContains []string
		wantBodyContains        []string
	}{
		{
			name:       "metrics GET uses promhttp content negotiation",
			method:     http.MethodGet,
			path:       "/metrics",
			wantStatus: http.StatusOK,
			wantContentTypeContains: []string{
				"text/plain",
				"version=0.0.4",
			},
			wantBodyContains: []string{"# HELP", "# TYPE"},
		},
		{
			name:   "metrics GET supports openmetrics negotiation",
			method: http.MethodGet,
			path:   "/metrics",
			requestHeaders: map[string]string{
				"Accept": "application/openmetrics-text; version=1.0.0; charset=utf-8",
			},
			wantStatus: http.StatusOK,
			wantContentTypeContains: []string{
				"application/openmetrics-text",
			},
		},
		{
			name:       "metrics HEAD is routed through promhttp",
			method:     http.MethodHead,
			path:       "/metrics",
			wantStatus: http.StatusOK,
			wantContentTypeContains: []string{
				"text/plain",
				"version=0.0.4",
			},
		},
		{
			name:       "metrics POST is not exposed",
			method:     http.MethodPost,
			path:       "/metrics",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "metrics OPTIONS is not exposed",
			method:     http.MethodOptions,
			path:       "/metrics",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			for headerName, headerValue := range tt.requestHeaders {
				req.Header.Set(headerName, headerValue)
			}

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("status mismatch: want %d, got %d", tt.wantStatus, rr.Code)
			}

			for headerName, wantValue := range tt.wantHeaders {
				if got := rr.Header().Get(headerName); got != wantValue {
					t.Fatalf("header %q mismatch: want %q, got %q", headerName, wantValue, got)
				}
			}

			contentType := rr.Header().Get("Content-Type")
			for _, want := range tt.wantContentTypeContains {
				if !strings.Contains(contentType, want) {
					t.Fatalf("content type %q does not contain %q", contentType, want)
				}
			}

			body := rr.Body.String()
			for _, want := range tt.wantBodyContains {
				if !strings.Contains(body, want) {
					t.Fatalf("body does not contain %q", want)
				}
			}
		})
	}
}

func TestNewInfraHandlerWiresDocumentationRoutes(t *testing.T) {
	handler, err := newInfraHandlerForTest(t, Config{Environment: "test"})
	if err != nil {
		t.Fatalf("newInfraHandler returned error: %v", err)
	}

	tests := []struct {
		name             string
		method           string
		path             string
		wantStatus       int
		wantHeaders      map[string]string
		wantBodyContains []string
	}{
		{
			name:       "swagger endpoint is exposed through infra mux",
			method:     http.MethodGet,
			path:       "/swagger",
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Content-Security-Policy": "default-src 'self'; connect-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; font-src 'self' data:; worker-src 'self' blob:; frame-ancestors 'none'; base-uri 'none'; form-action 'none'",
			},
			wantBodyContains: []string{
				"\"urls.primaryName\": \"Public API\"",
			},
		},
		{
			name:       "docs endpoint redirects through infra mux",
			method:     http.MethodGet,
			path:       "/docs",
			wantStatus: http.StatusTemporaryRedirect,
			wantHeaders: map[string]string{
				"Location": "/swagger",
			},
		},
		{
			name:       "public spec is exposed through infra mux",
			method:     http.MethodGet,
			path:       "/openapi/public.yaml",
			wantStatus: http.StatusOK,
			wantBodyContains: []string{
				"title: Base API",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("status mismatch: want %d, got %d", tt.wantStatus, rr.Code)
			}

			for headerName, wantValue := range tt.wantHeaders {
				if got := rr.Header().Get(headerName); got != wantValue {
					t.Fatalf("header %q mismatch: want %q, got %q", headerName, wantValue, got)
				}
			}

			body := rr.Body.String()
			for _, want := range tt.wantBodyContains {
				if !strings.Contains(body, want) {
					t.Fatalf("body does not contain %q", want)
				}
			}
		})
	}
}

func TestNewInfraHandlerRecoversPanicsFromMetricsGatherer(t *testing.T) {
	runtimeDeps := newRuntimeDependenciesForTest(t)
	runtimeDeps.metricsGatherer = panicGatherer{}

	handler, err := newInfraHandler(Config{Environment: "test"}, runtimeDeps)
	if err != nil {
		t.Fatalf("newInfraHandler returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusInternalServerError, rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content-type mismatch: want %q, got %q", "application/json; charset=utf-8", got)
	}
	if got := rr.Body.String(); got != `{"code":"internal_error","message":"internal server error"}` {
		t.Fatalf("body mismatch: got %q", got)
	}
}

func TestNewPublicHandlerCORSAndCSRFInteraction(t *testing.T) {
	t.Run("allows preflight for configured origin", func(t *testing.T) {
		handler, err := newPublicHandlerForTest(t, Config{
			Environment: "test",
			CORS: CORSConfig{
				AllowedOrigins: []string{"https://client.example"},
			},
			CSRF: CSRFConfig{Enabled: true},
		})
		if err != nil {
			t.Fatalf("newPublicHandler returned error: %v", err)
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
		handler, err := newPublicHandlerForTest(t, Config{
			Environment: "test",
			CORS: CORSConfig{
				AllowedOrigins: []string{"https://trusted.example"},
			},
			CSRF: CSRFConfig{
				Enabled:        true,
				TrustedOrigins: []string{"https://trusted.example"},
			},
		})
		if err != nil {
			t.Fatalf("newPublicHandler returned error: %v", err)
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
		if got := rr.Body.String(); got != `{"code":"forbidden","message":"cross-origin request denied"}` {
			t.Fatalf("body mismatch: got %q", got)
		}
	})

	t.Run("trusted cross-origin requests pass CSRF layer", func(t *testing.T) {
		baseCfg := Config{
			Environment: "test",
			CORS: CORSConfig{
				AllowedOrigins: []string{"https://trusted.example"},
			},
		}

		handlerWithoutCSRF, err := newPublicHandlerForTest(t, baseCfg)
		if err != nil {
			t.Fatalf("newPublicHandler without CSRF returned error: %v", err)
		}

		cfgWithCSRF := baseCfg
		cfgWithCSRF.CSRF = CSRFConfig{
			Enabled:        true,
			TrustedOrigins: []string{"https://trusted.example"},
		}
		handlerWithCSRF, err := newPublicHandlerForTest(t, cfgWithCSRF)
		if err != nil {
			t.Fatalf("newPublicHandler with CSRF returned error: %v", err)
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

	handler, err := newPublicHandlerForTest(t, Config{
		Environment: "test",
		OTEL: OTELConfig{
			TracingEnabled: true,
		},
	})
	if err != nil {
		t.Fatalf("newPublicHandler returned error: %v", err)
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

	handler, err := newPublicHandlerForTest(t, Config{
		Environment: "test",
		OTEL: OTELConfig{
			TracingEnabled: true,
		},
	})
	if err != nil {
		t.Fatalf("newPublicHandler returned error: %v", err)
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
			if string(attr.Key) == "api.operation.id" && attr.Value.Type() == attribute.STRING && attr.Value.AsString() == "getHealthz" {
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
		t.Fatalf("expected exactly one span with api.operation.id=getHealthz, got %d spans (names=%v)", len(matches), names)
	}

	targetSpan := matches[0]
	if got := targetSpan.Name(); got != "GET getHealthz" {
		t.Fatalf("span name mismatch: want %q, got %q", "GET getHealthz", got)
	}

	attrs := map[string]string{}
	for _, attr := range targetSpan.Attributes() {
		if attr.Value.Type() != attribute.STRING {
			continue
		}
		attrs[string(attr.Key)] = attr.Value.AsString()
	}

	for key, want := range map[string]string{
		"api.operation.id":      "getHealthz",
		"api.operation.name":    "GetHealthz",
		"api.operation.summary": "Public health endpoint",
	} {
		if got := attrs[key]; got != want {
			t.Fatalf("span attribute %q mismatch: want %q, got %q", key, want, got)
		}
	}
}

func newRuntimeDependenciesForTest(t *testing.T) runtimeDependencies {
	t.Helper()

	deps, err := newRuntimeDependencies()
	if err != nil {
		t.Fatalf("newRuntimeDependencies returned error: %v", err)
	}

	return deps
}

func newPublicHandlerForTest(t *testing.T, cfg Config) (http.Handler, error) {
	t.Helper()

	return newPublicHandler(cfg, newRuntimeDependenciesForTest(t))
}

func newInfraHandlerForTest(t *testing.T, cfg Config) (http.Handler, error) {
	t.Helper()

	return newInfraHandler(cfg, newRuntimeDependenciesForTest(t))
}

func decodeJSONLogLines(t *testing.T, data string) []map[string]any {
	t.Helper()

	if strings.TrimSpace(data) == "" {
		return nil
	}

	decoder := json.NewDecoder(strings.NewReader(data))
	entries := make([]map[string]any, 0)
	for {
		entry := map[string]any{}
		if err := decoder.Decode(&entry); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			t.Fatalf("decode JSON log entry: %v", err)
		}

		entries = append(entries, entry)
	}

	return entries
}

type errWriter struct {
	err error
}

type panicGatherer struct{}

func (panicGatherer) Gather() ([]*dto.MetricFamily, error) {
	panic("metrics gather panic")
}

func (w errWriter) Write(p []byte) (int, error) {
	return 0, w.err
}

const maxRunStartupAttempts = 5

func assertRunHandlesWriterErrors(t *testing.T, stdout, stderr io.Writer) {
	t.Helper()

	var lastErr error
	for attempt := 1; attempt <= maxRunStartupAttempts; attempt++ {
		publicAddr := reserveTCPAddress(t)
		infraAddr := reserveTCPAddress(t)

		ctx, cancel := context.WithCancel(context.Background())
		env := map[string]string{
			"API_ADDR":        publicAddr,
			"API_INFRA_ADDR":  infraAddr,
			"API_ENVIRONMENT": "test",
		}

		runDone := make(chan struct{})
		var runErr error
		go func() {
			runErr = Run(ctx, nil, getenvFromMap(env), strings.NewReader(""), stdout, stderr)
			close(runDone)
		}()

		startupErr := waitForStatusOK("http://"+publicAddr+"/healthz", runDone, &runErr)
		if startupErr == nil {
			startupErr = waitForStatusOK("http://"+infraAddr+"/livez", runDone, &runErr)
		}

		if startupErr != nil {
			cancel()
			if !waitForRunDone(runDone, 3*time.Second) {
				t.Fatalf("Run did not return after failed startup attempt")
			}

			if isAddressInUseError(startupErr) && attempt < maxRunStartupAttempts {
				lastErr = startupErr
				continue
			}

			t.Fatalf("startup check failed: %v", startupErr)
		}

		cancel()
		if !waitForRunDone(runDone, 3*time.Second) {
			t.Fatalf("Run did not return after cancellation")
		}

		if runErr != nil {
			if isAddressInUseError(runErr) && attempt < maxRunStartupAttempts {
				lastErr = runErr
				continue
			}

			t.Fatalf("Run returned error: %v", runErr)
		}

		return
	}

	t.Fatalf("Run failed after %d startup attempts: %v", maxRunStartupAttempts, lastErr)
}

func reserveTCPAddress(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on ephemeral port: %v", err)
	}
	defer func() { _ = ln.Close() }()

	return ln.Addr().String()
}

func waitForStatusOK(url string, runDone <-chan struct{}, runErr *error) error {
	client := &http.Client{Timeout: 250 * time.Millisecond}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-runDone:
			if *runErr != nil {
				return fmt.Errorf("run exited before %s became ready: %w", url, *runErr)
			}
			return fmt.Errorf("run exited before %s became ready", url)
		default:
		}

		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		time.Sleep(10 * time.Millisecond)
	}

	select {
	case <-runDone:
		if *runErr != nil {
			return fmt.Errorf("run exited before %s became ready: %w", url, *runErr)
		}
		return fmt.Errorf("run exited before %s became ready", url)
	default:
	}

	return fmt.Errorf("timed out waiting for %s", url)
}

func waitForRunDone(runDone <-chan struct{}, timeout time.Duration) bool {
	select {
	case <-runDone:
		return true
	case <-time.After(timeout):
		return false
	}
}

func isAddressInUseError(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}

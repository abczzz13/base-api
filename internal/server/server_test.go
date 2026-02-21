package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"
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
	if !strings.Contains(err.Error(), "public server listen") {
		t.Fatalf("Run error does not identify public listener failure: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid-address") {
		t.Fatalf("Run error does not include invalid address context: %v", err)
	}
}

func TestNewInfraHandlerRoutesMetricsThroughPromHTTP(t *testing.T) {
	handler, err := newInfraHandler(Config{Environment: "test"})
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
	handler, err := newInfraHandler(Config{Environment: "test"})
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

type errWriter struct {
	err error
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

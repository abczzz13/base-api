package middleware

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLogger(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		statusCode int
		wantStatus int
	}{
		{
			name:       "logs GET request and passes through",
			method:     "GET",
			statusCode: http.StatusOK,
			wantStatus: http.StatusOK,
		},
		{
			name:       "logs POST request and passes through",
			method:     "POST",
			statusCode: http.StatusCreated,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "logs error status and passes through",
			method:     "GET",
			statusCode: http.StatusInternalServerError,
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			})

			req := httptest.NewRequest(tt.method, "/test", nil)
			rec := httptest.NewRecorder()

			RequestLogger()(handler).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rec.Code)
			}
		})
	}
}

func TestRequestLoggerDelegatesReadFrom(t *testing.T) {
	rec := &readFromRecorder{ResponseRecorder: httptest.NewRecorder()}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readerFrom, ok := w.(io.ReaderFrom)
		if !ok {
			t.Fatalf("expected response writer to implement io.ReaderFrom")
		}

		if _, err := readerFrom.ReadFrom(strings.NewReader("payload")); err != nil {
			t.Fatalf("ReadFrom returned error: %v", err)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	RequestLogger()(handler).ServeHTTP(rec, req)

	if !rec.readFromCalled {
		t.Fatalf("expected underlying ReadFrom to be called")
	}
	if got := rec.Body.String(); got != "payload" {
		t.Fatalf("expected body %q, got %q", "payload", got)
	}
}

func TestRequestLoggerDelegatesFlush(t *testing.T) {
	rec := httptest.NewRecorder()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatalf("expected response writer to implement http.Flusher")
		}
		flusher.Flush()
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	RequestLogger()(handler).ServeHTTP(rec, req)

	if !rec.Flushed {
		t.Fatalf("expected underlying flush to be called")
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
}

func TestRequestLoggerDelegatesHijack(t *testing.T) {
	rec := &hijackRecorder{ResponseRecorder: httptest.NewRecorder()}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Fatalf("expected response writer to implement http.Hijacker")
		}

		conn, _, err := hijacker.Hijack()
		if err != nil {
			t.Fatalf("Hijack returned error: %v", err)
		}
		_ = conn.Close()
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	RequestLogger()(handler).ServeHTTP(rec, req)

	if !rec.hijackCalled {
		t.Fatalf("expected underlying hijack to be called")
	}
}

func TestRequestLoggerPushReturnsNotSupportedWhenUnavailable(t *testing.T) {
	rec := httptest.NewRecorder()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pusher, ok := w.(http.Pusher)
		if !ok {
			t.Fatalf("expected response writer to implement http.Pusher")
		}

		err := pusher.Push("/asset.js", nil)
		if !errors.Is(err, http.ErrNotSupported) {
			t.Fatalf("expected http.ErrNotSupported, got %v", err)
		}

		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	RequestLogger()(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestRequestLoggerLogsRecoveredPanics(t *testing.T) {
	var logs bytes.Buffer
	setDefaultLoggerForRequestLoggerTest(t, &logs)

	handler := RequestLogger()(Recovery()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}

	entries := decodeJSONLines(t, logs.String())
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

	status, ok := completed["status"].(float64)
	if !ok {
		t.Fatalf("expected numeric status field in request completed log, got %#v", completed["status"])
	}
	if got := int(status); got != http.StatusInternalServerError {
		t.Fatalf("status field mismatch: want %d, got %d", http.StatusInternalServerError, got)
	}
}

func setDefaultLoggerForRequestLoggerTest(t *testing.T, writer io.Writer) {
	t.Helper()

	previous := slog.Default()
	logger := slog.New(slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	t.Cleanup(func() {
		slog.SetDefault(previous)
	})
}

func decodeJSONLines(t *testing.T, data string) []map[string]any {
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

type readFromRecorder struct {
	*httptest.ResponseRecorder
	readFromCalled bool
}

func (r *readFromRecorder) ReadFrom(src io.Reader) (int64, error) {
	r.readFromCalled = true
	return io.Copy(r.ResponseRecorder, src)
}

type hijackRecorder struct {
	*httptest.ResponseRecorder
	hijackCalled bool
}

func (r *hijackRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	r.hijackCalled = true
	serverConn, clientConn := net.Pipe()
	_ = clientConn.Close()

	readWriter := bufio.NewReadWriter(bufio.NewReader(serverConn), bufio.NewWriter(serverConn))
	return serverConn, readWriter, nil
}

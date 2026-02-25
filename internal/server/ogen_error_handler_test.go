package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	ht "github.com/ogen-go/ogen/http"
	"github.com/ogen-go/ogen/ogenerrors"

	"github.com/abczzz13/base-api/internal/oas"
)

func TestOgenErrorHandler(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   oas.ErrorResponse
	}{
		{
			name: "maps decode request errors to bad request",
			err: &ogenerrors.DecodeRequestError{
				OperationContext: ogenerrors.OperationContext{Name: "getHealthz", ID: "getHealthz"},
				Err:              errors.New("invalid input"),
			},
			wantStatus: http.StatusBadRequest,
			wantBody: oas.ErrorResponse{
				Code:    "bad_request",
				Message: "bad request",
			},
		},
		{
			name:       "maps not implemented error",
			err:        ht.ErrNotImplemented,
			wantStatus: http.StatusNotImplemented,
			wantBody: oas.ErrorResponse{
				Code:    "not_implemented",
				Message: "not implemented",
			},
		},
		{
			name:       "maps unknown errors to internal error",
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
			wantBody: oas.ErrorResponse{
				Code:    "internal_error",
				Message: "internal server error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			rr := httptest.NewRecorder()

			ogenErrorHandler(context.Background(), rr, req, tt.err)

			if diff := cmp.Diff(tt.wantStatus, rr.Code); diff != "" {
				t.Fatalf("status mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff("application/json; charset=utf-8", rr.Header().Get("Content-Type")); diff != "" {
				t.Fatalf("content type mismatch (-want +got):\n%s", diff)
			}

			var got oas.ErrorResponse
			if err := got.UnmarshalJSON(rr.Body.Bytes()); err != nil {
				t.Fatalf("decode response body: %v", err)
			}

			if diff := cmp.Diff(tt.wantBody, got); diff != "" {
				t.Fatalf("body mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestOgenErrorHandlerLogsServerErrors(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		wantStatus    int
		wantLog       bool
		wantErrorText string
	}{
		{
			name:          "logs internal server errors",
			err:           errors.New("boom"),
			wantStatus:    http.StatusInternalServerError,
			wantLog:       true,
			wantErrorText: "boom",
		},
		{
			name: "does not log client errors",
			err: &ogenerrors.DecodeRequestError{
				OperationContext: ogenerrors.OperationContext{Name: "getHealthz", ID: "getHealthz"},
				Err:              errors.New("invalid input"),
			},
			wantStatus: http.StatusBadRequest,
			wantLog:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			setDefaultLoggerForTest(t, &logs)

			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			rr := httptest.NewRecorder()

			ogenErrorHandler(context.Background(), rr, req, tt.err)

			if diff := cmp.Diff(tt.wantStatus, rr.Code); diff != "" {
				t.Fatalf("status mismatch (-want +got):\n%s", diff)
			}

			entries := decodeJSONLines(t, logs.String())
			var errorEntries []map[string]any
			for _, entry := range entries {
				if msg, _ := entry["msg"].(string); msg == "ogen error response" {
					errorEntries = append(errorEntries, entry)
				}
			}

			logged := len(errorEntries) > 0
			if diff := cmp.Diff(tt.wantLog, logged); diff != "" {
				t.Fatalf("log presence mismatch (-want +got):\n%s\nentries: %#v", diff, entries)
			}

			if !tt.wantLog {
				return
			}

			if diff := cmp.Diff(1, len(errorEntries)); diff != "" {
				t.Fatalf("expected one ogen error response log entry (-want +got):\n%s\nentries: %#v", diff, entries)
			}

			entry := errorEntries[0]

			status, ok := entry["status"].(float64)
			if !ok {
				t.Fatalf("expected numeric status field, got %#v", entry["status"])
			}
			if diff := cmp.Diff(tt.wantStatus, int(status)); diff != "" {
				t.Fatalf("status field mismatch (-want +got):\n%s", diff)
			}

			method, ok := entry["method"].(string)
			if !ok {
				t.Fatalf("expected string method field, got %#v", entry["method"])
			}
			if diff := cmp.Diff("GET", method); diff != "" {
				t.Fatalf("method field mismatch (-want +got):\n%s", diff)
			}

			path, ok := entry["path"].(string)
			if !ok {
				t.Fatalf("expected string path field, got %#v", entry["path"])
			}
			if diff := cmp.Diff("/healthz", path); diff != "" {
				t.Fatalf("path field mismatch (-want +got):\n%s", diff)
			}

			errorText, ok := entry["error"].(string)
			if !ok {
				t.Fatalf("expected string error field, got %#v", entry["error"])
			}
			if diff := cmp.Diff(tt.wantErrorText, errorText); diff != "" {
				t.Fatalf("error field mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func setDefaultLoggerForTest(t *testing.T, writer io.Writer) {
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

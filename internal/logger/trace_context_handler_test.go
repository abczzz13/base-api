package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestNewAddsTraceAndSpanIDsWhenContextContainsSpan(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	var logs bytes.Buffer
	New(Config{Format: FormatJSON, Writer: &logs})

	traceID, err := trace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("parse trace id: %v", err)
	}
	spanID, err := trace.SpanIDFromHex("0123456789abcdef")
	if err != nil {
		t.Fatalf("parse span id: %v", err)
	}

	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext)

	slog.InfoContext(ctx, "trace test")

	entry := decodeSingleJSONEntry(t, logs.String())
	if got := entry["trace_id"]; got != traceID.String() {
		t.Fatalf("trace_id mismatch: want %q, got %#v", traceID.String(), got)
	}
	if got := entry["span_id"]; got != spanID.String() {
		t.Fatalf("span_id mismatch: want %q, got %#v", spanID.String(), got)
	}
}

func TestNewOmitsTraceAndSpanIDsWithoutSpanContext(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	var logs bytes.Buffer
	New(Config{Format: FormatJSON, Writer: &logs})

	slog.InfoContext(context.Background(), "no trace")

	entry := decodeSingleJSONEntry(t, logs.String())
	if _, ok := entry["trace_id"]; ok {
		t.Fatalf("trace_id should be omitted when span context is missing")
	}
	if _, ok := entry["span_id"]; ok {
		t.Fatalf("span_id should be omitted when span context is missing")
	}
}

func decodeSingleJSONEntry(t *testing.T, data string) map[string]any {
	t.Helper()

	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		t.Fatal("expected log output, got empty string")
	}

	var entry map[string]any
	if err := json.Unmarshal([]byte(trimmed), &entry); err != nil {
		t.Fatalf("decode JSON log entry: %v", err)
	}

	return entry
}

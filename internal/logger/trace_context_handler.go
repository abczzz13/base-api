package logger

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

type traceContextHandler struct {
	next slog.Handler
}

func newTraceContextHandler(next slog.Handler) slog.Handler {
	return &traceContextHandler{next: next}
}

func (h *traceContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *traceContextHandler) Handle(ctx context.Context, record slog.Record) error {
	spanContext := trace.SpanContextFromContext(ctx)
	if spanContext.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", spanContext.TraceID().String()),
			slog.String("span_id", spanContext.SpanID().String()),
		)
	}

	return h.next.Handle(ctx, record)
}

func (h *traceContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceContextHandler{next: h.next.WithAttrs(attrs)}
}

func (h *traceContextHandler) WithGroup(name string) slog.Handler {
	return &traceContextHandler{next: h.next.WithGroup(name)}
}

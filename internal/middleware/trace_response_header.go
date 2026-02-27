package middleware

import (
	"net/http"

	"go.opentelemetry.io/otel/trace"
)

const TraceIDResponseHeader = "X-Trace-Id"

func TraceResponseHeader() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			spanContext := trace.SpanContextFromContext(r.Context())
			if spanContext.IsValid() {
				w.Header().Set(TraceIDResponseHeader, spanContext.TraceID().String())
			}

			next.ServeHTTP(w, r)
		})
	}
}

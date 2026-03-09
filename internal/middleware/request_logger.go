package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/abczzz13/base-api/internal/middleware/internal/responsewriter"
)

func RequestLogger() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ctx := r.Context()

			nextWriter, rw := responsewriter.EnsureObservedResponseWriter(w)

			slog.With(
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("remote_addr", r.RemoteAddr),
			).DebugContext(ctx, "request started")

			next.ServeHTTP(nextWriter, r)

			duration := time.Since(start)

			slog.With(
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rw.StatusCode),
				slog.Int64("duration_ms", duration.Milliseconds()),
			).InfoContext(ctx, "request completed")
		})
	}
}

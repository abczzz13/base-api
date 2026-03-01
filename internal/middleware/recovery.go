package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime"

	"github.com/abczzz13/base-api/internal/apierrors"
)

func Recovery() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rvr := recover(); rvr != nil {
					observeRecoveredPanic(r)

					stack := make([]byte, 4096)
					n := runtime.Stack(stack, false)
					stack = stack[:n]

					ctx := r.Context()
					slog.With(
						slog.String("error", fmt.Sprintf("%v", rvr)),
						slog.String("stack", string(stack)),
						slog.String("method", r.Method),
						slog.String("path", r.URL.Path),
					).ErrorContext(ctx, "panic recovered")

					apierrors.WriteError(w, "internal_error", "internal server error", http.StatusInternalServerError)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

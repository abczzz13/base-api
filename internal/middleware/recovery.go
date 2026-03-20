package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime"

	"github.com/abczzz13/base-api/internal/apierrors"
	"github.com/abczzz13/base-api/internal/requestid"
)

func Recovery() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			currentReq := r
			wrappedNext := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				currentReq = req
				next.ServeHTTP(w, req)
			})

			defer func() {
				if rvr := recover(); rvr != nil {
					observeRecoveredPanic(currentReq)

					stack := make([]byte, 4096)
					n := runtime.Stack(stack, false)
					stack = stack[:n]

					ctx := currentReq.Context()
					if requestid.FromContext(ctx) == "" {
						ctx = requestid.WithContext(ctx, w.Header().Get(requestid.HeaderName))
					}
					slog.ErrorContext(ctx,
						"panic recovered",
						slog.String("error", fmt.Sprintf("%v", rvr)),
						slog.String("stack", string(stack)),
						slog.String("method", currentReq.Method),
						slog.String("path", currentReq.URL.Path),
					)

					apierrors.WriteError(ctx, w, "internal_error", "internal server error", http.StatusInternalServerError)
				}
			}()

			wrappedNext.ServeHTTP(w, r)
		})
	}
}

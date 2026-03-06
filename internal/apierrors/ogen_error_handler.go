package apierrors

import (
	"context"
	"log/slog"
	"net/http"
)

// OgenErrorHandler maps ogen framework errors to API errors and logs 5xx responses.
func OgenErrorHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, err error) {
	if ctx == nil {
		ctx = context.Background()
	}

	apiErr := FromOgenError(err)
	if apiErr.StatusCode >= http.StatusInternalServerError {
		attrs := []slog.Attr{slog.Int("status", apiErr.StatusCode)}
		if r != nil {
			attrs = append(attrs, slog.String("method", r.Method))
			if r.URL != nil {
				attrs = append(attrs, slog.String("path", r.URL.Path))
			}
		}
		if err != nil {
			attrs = append(attrs, slog.String("error", err.Error()))
		}

		slog.LogAttrs(ctx, slog.LevelError, "ogen error response", attrs...)
	}

	apiErr.Write(w)
}

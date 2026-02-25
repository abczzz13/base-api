package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/abczzz13/base-api/internal/apierrors"
)

func ogenErrorHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, err error) {
	if ctx == nil {
		ctx = context.Background()
	}

	apiErr := apierrors.FromOgenError(err)
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

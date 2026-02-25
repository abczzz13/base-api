package middleware

import (
	"log/slog"
	"net/http"

	"github.com/abczzz13/base-api/internal/apierrors"
)

type CSRFConfig struct {
	TrustedOrigins []string
}

func CSRF(cfg CSRFConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		cop := http.NewCrossOriginProtection()

		for _, origin := range cfg.TrustedOrigins {
			if err := cop.AddTrustedOrigin(origin); err != nil {
				slog.Warn("invalid CSRF trusted origin ignored", "origin", origin, "error", err.Error())
			}
		}

		cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			origin := r.Header.Get("Origin")
			secFetchSite := r.Header.Get("Sec-Fetch-Site")

			slog.WarnContext(ctx, "csrf request denied",
				"method", r.Method,
				"path", r.URL.Path,
				"origin", origin,
				"sec_fetch_site", secFetchSite,
			)

			apierrors.WriteError(w, "forbidden", "cross-origin request denied", http.StatusForbidden)
		}))

		return cop.Handler(next)
	}
}

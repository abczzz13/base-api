package middleware

import (
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func Tracing(serverName string) func(http.Handler) http.Handler {
	return otelhttp.NewMiddleware(
		serverName,
		otelhttp.WithServerName(serverName),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			if r.Pattern != "" {
				if r.Method != "" {
					methodPrefix := r.Method + " "
					if strings.HasPrefix(r.Pattern, methodPrefix) {
						return r.Pattern
					}

					return methodPrefix + r.Pattern
				}

				return r.Pattern
			}

			if r.Method == "" {
				return "HTTP"
			}

			return r.Method
		}),
	)
}

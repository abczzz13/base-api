package publicapi

import (
	"net/http"
	"strings"

	"github.com/abczzz13/base-api/internal/middleware"
	"github.com/abczzz13/base-api/internal/publicoas"
)

func requestMetricsRouteLabeler(api *publicoas.Server) func(*http.Request) string {
	return func(r *http.Request) string {
		if api == nil || r == nil || r.URL == nil {
			return middleware.RequestMetricsRouteUnmatched
		}

		if route, ok := api.FindPath(r.Method, r.URL); ok {
			if operationID := strings.TrimSpace(route.OperationID()); operationID != "" {
				return operationID
			}
		}

		return middleware.RequestMetricsRouteUnmatched
	}
}

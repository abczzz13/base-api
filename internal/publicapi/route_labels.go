package publicapi

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/abczzz13/base-api/internal/middleware"
	"github.com/abczzz13/base-api/internal/publicoas"
)

func requestMetricsRouteLabeler(coreLabeler, weatherLabeler func(*http.Request) string) func(*http.Request) string {
	return func(r *http.Request) string {
		if r == nil || r.URL == nil {
			return middleware.RequestMetricsRouteUnmatched
		}

		for _, labeler := range []func(*http.Request) string{weatherLabeler, coreLabeler} {
			if labeler == nil {
				continue
			}

			if label := strings.TrimSpace(labeler(r)); label != "" && label != middleware.RequestMetricsRouteUnmatched {
				return label
			}
		}

		return middleware.RequestMetricsRouteUnmatched
	}
}

func coreRouteLabeler(api *publicoas.Server) func(*http.Request) string {
	return middleware.OperationLabeler(middleware.OperationFinderFunc(func(method string, u *url.URL) (string, bool) {
		if route, ok := api.FindPath(method, u); ok {
			return route.Name(), true
		}
		return "", false
	}))
}

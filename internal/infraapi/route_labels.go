package infraapi

import (
	"net/http"
	"net/url"
	"strings"

	geninfra "github.com/abczzz13/base-api/internal/infraoas"
	"github.com/abczzz13/base-api/internal/middleware"
)

const (
	routeMetrics        = "metrics"
	routeSwaggerUI      = "swagger_ui"
	routeSwaggerUIAsset = "swagger_ui_asset"
	routeDocsRedirect   = "docs_redirect"
	routeOpenAPISpec    = "openapi_spec"
)

func requestMetricsRouteLabeler(api *geninfra.Server) func(*http.Request) string {
	ogenLabeler := middleware.OperationLabeler(middleware.OperationFinderFunc(func(method string, u *url.URL) (string, bool) {
		if route, ok := api.FindPath(method, u); ok {
			return route.Name(), true
		}
		return "", false
	}))

	return func(r *http.Request) string {
		if r == nil || r.URL == nil {
			return middleware.RequestMetricsRouteUnmatched
		}

		path := r.URL.Path
		switch {
		case path == "/metrics":
			return routeMetrics
		case path == "/swagger":
			return routeSwaggerUI
		case strings.HasPrefix(path, "/swagger-ui/"):
			return routeSwaggerUIAsset
		case path == "/docs", path == "/docs/":
			return routeDocsRedirect
		case strings.HasPrefix(path, "/openapi/"):
			return routeOpenAPISpec
		}

		return ogenLabeler(r)
	}
}

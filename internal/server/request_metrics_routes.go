package server

import (
	"net/http"
	"strings"

	"github.com/abczzz13/base-api/internal/infraoas"
	"github.com/abczzz13/base-api/internal/middleware"
	"github.com/abczzz13/base-api/internal/oas"
)

const (
	infraRouteMetrics        = "metrics"
	infraRouteSwaggerUI      = "swagger_ui"
	infraRouteSwaggerUIAsset = "swagger_ui_asset"
	infraRouteDocsRedirect   = "docs_redirect"
	infraRouteOpenAPISpec    = "openapi_spec"
)

func publicRequestMetricsRouteLabeler(api *oas.Server) func(*http.Request) string {
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

func infraRequestMetricsRouteLabeler(api *infraoas.Server) func(*http.Request) string {
	return func(r *http.Request) string {
		if r == nil || r.URL == nil {
			return middleware.RequestMetricsRouteUnmatched
		}

		path := r.URL.Path
		switch {
		case path == "/metrics":
			return infraRouteMetrics
		case path == "/swagger":
			return infraRouteSwaggerUI
		case strings.HasPrefix(path, "/swagger-ui/"):
			return infraRouteSwaggerUIAsset
		case path == "/docs", path == "/docs/":
			return infraRouteDocsRedirect
		case strings.HasPrefix(path, "/openapi/"):
			return infraRouteOpenAPISpec
		}

		if api != nil {
			if route, ok := api.FindPath(r.Method, r.URL); ok {
				if operationID := strings.TrimSpace(route.OperationID()); operationID != "" {
					return operationID
				}
			}
		}

		return middleware.RequestMetricsRouteUnmatched
	}
}

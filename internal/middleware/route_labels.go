package middleware

import (
	"net/http"
	"net/url"
	"strings"
)

// OperationFinder looks up a stable operation label for a given HTTP method and URL.
type OperationFinder interface {
	FindOperation(method string, u *url.URL) (operation string, ok bool)
}

// OperationLabeler returns a route labeler that resolves operations via FindOperation.
func OperationLabeler(finder OperationFinder) func(*http.Request) string {
	return func(r *http.Request) string {
		if finder == nil || r == nil || r.URL == nil {
			return RequestMetricsRouteUnmatched
		}

		if operation, ok := finder.FindOperation(r.Method, r.URL); ok {
			if trimmed := strings.TrimSpace(operation); trimmed != "" {
				return trimmed
			}
		}

		return RequestMetricsRouteUnmatched
	}
}

// OperationFinderFunc adapts a function to the OperationFinder interface.
type OperationFinderFunc func(method string, u *url.URL) (string, bool)

// FindOperation implements OperationFinder.
func (f OperationFinderFunc) FindOperation(method string, u *url.URL) (string, bool) {
	return f(method, u)
}

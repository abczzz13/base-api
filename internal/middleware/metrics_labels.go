package middleware

import (
	"net/http"
	"strings"
)

const (
	requestMetricsNamespace = "base_api"
	requestMetricsSubsystem = "http"

	RequestMetricsRouteUnmatched = "unmatched"
	RequestRejectionReasonCSRF   = "csrf"

	requestMetricsServerUnknown = "unknown"
	requestMetricsMethodUnknown = "UNKNOWN"
	requestMetricsReasonUnknown = "unknown"
)

func defaultRouteLabel(fn func(*http.Request) string) func(*http.Request) string {
	if fn != nil {
		return fn
	}

	return func(*http.Request) string {
		return RequestMetricsRouteUnmatched
	}
}

func defaultServerLabel(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return requestMetricsServerUnknown
	}

	return trimmed
}

func normalizeMethod(method string) string {
	normalized := strings.ToUpper(strings.TrimSpace(method))
	if normalized == "" {
		return requestMetricsMethodUnknown
	}

	return normalized
}

func requestMetricsMethodLabel(r *http.Request) string {
	return requestMetricsMethodValue(r.Method)
}

func requestMetricsMethodValue(value string) string {
	normalized := normalizeMethod(value)

	switch normalized {
	case http.MethodConnect,
		http.MethodDelete,
		http.MethodGet,
		http.MethodHead,
		http.MethodOptions,
		http.MethodPatch,
		http.MethodPost,
		http.MethodPut,
		http.MethodTrace:
		return normalized
	default:
		return requestMetricsMethodUnknown
	}
}

func requestMetricsRouteLabel(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return RequestMetricsRouteUnmatched
	}

	return trimmed
}

func requestMetricsReasonLabel(reason string) string {
	trimmed := strings.ToLower(strings.TrimSpace(reason))
	if trimmed == "" {
		return requestMetricsReasonUnknown
	}

	return trimmed
}

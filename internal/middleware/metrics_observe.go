package middleware

import (
	"context"
	"net/http"
)

type requestMetricsLabels struct {
	server string
	method string
	route  string
}

type requestMetricsContext struct {
	metrics *HTTPRequestMetrics
	labels  requestMetricsLabels
}

type requestMetricsContextKey struct{}

func requestWithMetricsContext(r *http.Request, metrics *HTTPRequestMetrics, labels requestMetricsLabels) *http.Request {
	if r == nil {
		return r
	}

	ctx := context.WithValue(r.Context(), requestMetricsContextKey{}, requestMetricsContext{metrics: metrics, labels: labels})
	return r.WithContext(ctx)
}

func requestMetricsContextFromRequest(r *http.Request) requestMetricsContext {
	metricsCtx := requestMetricsContext{
		labels: requestMetricsLabels{
			server: requestMetricsServerUnknown,
			method: requestMetricsMethodLabel(r),
			route:  RequestMetricsRouteUnmatched,
		},
	}

	if r == nil {
		return metricsCtx
	}

	if fromContext, ok := r.Context().Value(requestMetricsContextKey{}).(requestMetricsContext); ok {
		metricsCtx.metrics = fromContext.metrics
		metricsCtx.labels.server = defaultServerLabel(fromContext.labels.server)
		metricsCtx.labels.method = requestMetricsMethodValue(fromContext.labels.method)
		metricsCtx.labels.route = requestMetricsRouteLabel(fromContext.labels.route)
	}

	return metricsCtx
}

func observeRecoveredPanic(r *http.Request) {
	metricsCtx := requestMetricsContextFromRequest(r)
	if metricsCtx.metrics == nil {
		return
	}

	metricsCtx.metrics.httpRecoveredPanicsTotal.WithLabelValues(metricsCtx.labels.server, metricsCtx.labels.method, metricsCtx.labels.route).Inc()
}

func observeRejectedRequest(r *http.Request, reason string) {
	metricsCtx := requestMetricsContextFromRequest(r)
	if metricsCtx.metrics == nil {
		return
	}

	reasonLabel := requestMetricsReasonLabel(reason)
	metricsCtx.metrics.httpRejectedRequestsTotal.WithLabelValues(metricsCtx.labels.server, metricsCtx.labels.method, metricsCtx.labels.route, reasonLabel).Inc()
}

func observeCORSPolicyDenial(r *http.Request) {
	metricsCtx := requestMetricsContextFromRequest(r)
	if metricsCtx.metrics == nil {
		return
	}

	metricsCtx.metrics.httpCORSPolicyDenialsTotal.WithLabelValues(metricsCtx.labels.server, metricsCtx.labels.method, metricsCtx.labels.route).Inc()
}

func observeRateLimitBackendError(r *http.Request) {
	metricsCtx := requestMetricsContextFromRequest(r)
	if metricsCtx.metrics == nil {
		return
	}

	metricsCtx.metrics.httpRateLimitErrorsTotal.WithLabelValues(metricsCtx.labels.server, metricsCtx.labels.method, metricsCtx.labels.route).Inc()
}

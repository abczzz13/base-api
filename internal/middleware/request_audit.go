package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
	"unicode"

	"go.opentelemetry.io/otel/trace"

	"github.com/abczzz13/base-api/internal/requestaudit"
	"github.com/abczzz13/base-api/internal/requestid"
)

const (
	requestAuditDefaultMaxBodyBytes = 64 * 1024
	requestAuditDefaultWriteTimeout = 250 * time.Millisecond
	requestAuditRedactedValue       = "[REDACTED]"
	requestAuditInvalidQueryValue   = "_redacted=invalid_query"
)

var (
	requestAuditInvalidJSONPlaceholder   = []byte(`{"_redacted":"invalid_json"}`)
	requestAuditTruncatedJSONPlaceholder = []byte(`{"_redacted":"truncated"}`)

	requestAuditSensitiveJSONKeys = map[string]struct{}{
		"password":      {},
		"passphrase":    {},
		"secret":        {},
		"clientsecret":  {},
		"token":         {},
		"accesstoken":   {},
		"refreshtoken":  {},
		"idtoken":       {},
		"apikey":        {},
		"privatekey":    {},
		"credential":    {},
		"credentials":   {},
		"session":       {},
		"sessionid":     {},
		"authorization": {},
		"cookie":        {},
		"setcookie":     {},
		"jwt":           {},
	}
)

type RequestAuditConfig struct {
	ClientIPResolver  *ClientIPResolver
	Store             requestaudit.Repository
	Server            string
	RouteLabel        func(*http.Request) string
	MaxBodyBytes      int
	WriteTimeout      time.Duration
	TrustedProxyCIDRs []netip.Prefix
}

func RequestAudit(cfg RequestAuditConfig) func(http.Handler) http.Handler {
	if cfg.Store == nil {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	server := requestMetricsServerLabel(cfg.Server)
	routeLabel := cfg.RouteLabel
	if routeLabel == nil {
		routeLabel = func(*http.Request) string {
			return RequestMetricsRouteUnmatched
		}
	}

	maxBodyBytes := cfg.MaxBodyBytes
	if maxBodyBytes <= 0 {
		maxBodyBytes = requestAuditDefaultMaxBodyBytes
	}

	writeTimeout := cfg.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = requestAuditDefaultWriteTimeout
	}

	clientIPResolver := cfg.ClientIPResolver
	if clientIPResolver == nil {
		clientIPResolver = NewClientIPResolver("request audit", cfg.TrustedProxyCIDRs)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r, _ = clientIPResolver.ResolveStrict(r)

			nextWriter, observedRW := ensureObservedResponseWriter(w)
			responseBodyCapture := newBodyCaptureResponseWriter(nextWriter, maxBodyBytes)

			var requestBodyCapture *requestAuditBodyCaptureReadCloser
			if r != nil && r.Body != nil {
				requestBodyCapture = newRequestAuditBodyCaptureReadCloser(r.Body, maxBodyBytes)
				r.Body = requestBodyCapture
			}

			startedAt := time.Now()
			next.ServeHTTP(responseBodyCapture, r)

			record := requestAuditRecordFromRequest(
				r,
				requestAuditRecordInputs{
					server:               server,
					routeLabel:           routeLabel,
					statusCode:           observedRW.statusCode,
					duration:             time.Since(startedAt),
					responseSizeBytes:    observedRW.bytesWritten,
					requestBodyCapture:   requestBodyCapture,
					responseBodyCapture:  responseBodyCapture,
					responseHeaderSource: responseBodyCapture.Header(),
				},
			)

			auditCtx, cancelAudit := requestAuditWriteContext(requestContextOrBackground(r), writeTimeout)
			defer cancelAudit()
			if err := cfg.Store.StoreRequestAudit(auditCtx, record); err != nil {
				slog.WarnContext(requestContextOrBackground(r), "request audit insert failed",
					slog.String("method", record.Method),
					slog.String("path", record.Path),
					slog.Int("status", record.StatusCode),
					slog.Any("error", err),
				)
			}
		})
	}
}

type requestAuditRecordInputs struct {
	server               string
	routeLabel           func(*http.Request) string
	statusCode           int
	duration             time.Duration
	responseSizeBytes    int64
	requestBodyCapture   *requestAuditBodyCaptureReadCloser
	responseBodyCapture  *bodyCaptureResponseWriter
	responseHeaderSource http.Header
}

func requestAuditRecordFromRequest(r *http.Request, inputs requestAuditRecordInputs) requestaudit.Record {
	record := requestaudit.Record{
		Server:                inputs.server,
		Route:                 requestMetricsRouteLabel(inputs.routeLabel(r)),
		Method:                requestAuditMethod(r),
		Path:                  requestAuditPath(r),
		Query:                 requestAuditQuery(r),
		Host:                  requestAuditHost(r),
		Scheme:                requestAuditScheme(r),
		Proto:                 requestAuditProto(r),
		StatusCode:            inputs.statusCode,
		Duration:              inputs.duration,
		RequestSizeBytes:      requestAuditRequestSizeBytes(r, inputs.requestBodyCapture),
		ResponseSizeBytes:     requestAuditNonNegativeSize(inputs.responseSizeBytes),
		RemoteAddr:            requestAuditRemoteAddr(r),
		ClientIP:              requestAuditClientIP(r),
		UserAgent:             requestAuditUserAgent(r),
		RequestHeaders:        requestAuditRedactHeaders(requestAuditHeaders(r)),
		ResponseHeaders:       requestAuditRedactHeaders(inputs.responseHeaderSource),
		RequestBody:           requestAuditRedactBody(requestAuditBodyBytes(inputs.requestBodyCapture), requestAuditBodyTruncated(inputs.requestBodyCapture)),
		ResponseBody:          requestAuditRedactBody(requestAuditBodyBytes(inputs.responseBodyCapture), requestAuditBodyTruncated(inputs.responseBodyCapture)),
		RequestBodyTruncated:  requestAuditBodyTruncated(inputs.requestBodyCapture),
		ResponseBodyTruncated: requestAuditBodyTruncated(inputs.responseBodyCapture),
		RequestID:             requestid.FromContext(requestContextOrBackground(r)),
	}

	if spanContext := trace.SpanContextFromContext(requestContextOrBackground(r)); spanContext.IsValid() {
		record.TraceID = spanContext.TraceID().String()
		record.SpanID = spanContext.SpanID().String()
	}

	return record
}

func requestAuditWriteContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	writeCtx := context.WithoutCancel(ctx)
	if timeout <= 0 {
		return writeCtx, func() {}
	}

	return context.WithTimeout(writeCtx, timeout)
}

func requestContextOrBackground(r *http.Request) context.Context {
	if r == nil {
		return context.Background()
	}

	if ctx := r.Context(); ctx != nil {
		return ctx
	}

	return context.Background()
}

func requestAuditMethod(r *http.Request) string {
	if r == nil {
		return requestMetricsMethodUnknown
	}

	method := strings.ToUpper(strings.TrimSpace(r.Method))
	if method == "" {
		return requestMetricsMethodUnknown
	}

	return method
}

func requestAuditPath(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}

	return r.URL.Path
}

func requestAuditQuery(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}

	return requestAuditRedactQuery(r.URL.RawQuery)
}

func requestAuditRedactQuery(rawQuery string) string {
	trimmed := strings.TrimSpace(rawQuery)
	if trimmed == "" {
		return ""
	}

	values, err := url.ParseQuery(trimmed)
	if err != nil {
		return requestAuditInvalidQueryValue
	}

	for key, entries := range values {
		if !requestAuditSensitiveJSONKey(key) {
			continue
		}

		values[key] = requestAuditRedactedValues(len(entries))
	}

	return values.Encode()
}

func requestAuditHost(r *http.Request) string {
	if r == nil {
		return ""
	}

	host := strings.TrimSpace(r.Host)
	if host != "" {
		return host
	}

	if r.URL != nil {
		return strings.TrimSpace(r.URL.Host)
	}

	return ""
}

func requestAuditScheme(r *http.Request) string {
	if r == nil {
		return ""
	}

	if r.URL != nil {
		if scheme := strings.ToLower(strings.TrimSpace(r.URL.Scheme)); scheme != "" {
			return scheme
		}
	}

	if forwardedProto := requestAuditFirstCSVValue(r.Header.Get("X-Forwarded-Proto")); forwardedProto != "" {
		return strings.ToLower(forwardedProto)
	}

	if r.TLS != nil {
		return "https"
	}

	return "http"
}

func requestAuditProto(r *http.Request) string {
	if r == nil {
		return ""
	}

	return strings.TrimSpace(r.Proto)
}

func requestAuditRemoteAddr(r *http.Request) string {
	if r == nil {
		return ""
	}

	return strings.TrimSpace(r.RemoteAddr)
}

func requestAuditClientIP(r *http.Request) string {
	if clientIP, ok := strictClientIPFromContext(requestContextOrBackground(r)); ok {
		return clientIP
	}

	return ""
}

func requestAuditUserAgent(r *http.Request) string {
	if r == nil {
		return ""
	}

	return strings.TrimSpace(r.UserAgent())
}

func requestAuditRequestSizeBytes(r *http.Request, capture *requestAuditBodyCaptureReadCloser) int64 {
	if r != nil && r.ContentLength >= 0 {
		return r.ContentLength
	}

	if capture != nil {
		return requestAuditNonNegativeSize(capture.TotalBytes())
	}

	return 0
}

func requestAuditNonNegativeSize(value int64) int64 {
	if value < 0 {
		return 0
	}

	return value
}

func requestAuditHeaders(r *http.Request) http.Header {
	if r == nil || r.Header == nil {
		return http.Header{}
	}

	return r.Header.Clone()
}

func requestAuditFirstCSVValue(value string) string {
	parts := strings.Split(value, ",")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			return trimmed
		}
	}

	return ""
}

func requestAuditRedactHeaders(headers http.Header) map[string][]string {
	if len(headers) == 0 {
		return map[string][]string{}
	}

	result := make(map[string][]string, len(headers))
	for key, values := range headers {
		if requestAuditSensitiveHeader(key) {
			result[key] = requestAuditRedactedValues(len(values))
			continue
		}

		result[key] = append([]string(nil), values...)
	}

	return result
}

func requestAuditRedactedValues(count int) []string {
	if count <= 0 {
		return []string{requestAuditRedactedValue}
	}

	values := make([]string, count)
	for i := range values {
		values[i] = requestAuditRedactedValue
	}

	return values
}

func requestAuditSensitiveHeader(headerName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(headerName))
	if normalized == "" {
		return false
	}

	switch normalized {
	case "authorization", "proxy-authorization", "cookie", "set-cookie":
		return true
	}

	if strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.HasSuffix(normalized, "-key") ||
		strings.HasSuffix(normalized, "_key") {
		return true
	}

	return false
}

func requestAuditRedactBody(body []byte, truncated bool) []byte {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil
	}

	if truncated {
		return append([]byte(nil), requestAuditTruncatedJSONPlaceholder...)
	}

	var value any
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return append([]byte(nil), requestAuditInvalidJSONPlaceholder...)
	}

	marshaled, err := json.Marshal(requestAuditRedactJSONValue(value))
	if err != nil {
		return append([]byte(nil), requestAuditInvalidJSONPlaceholder...)
	}

	return marshaled
}

func requestAuditRedactJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, nestedValue := range typed {
			if requestAuditSensitiveJSONKey(key) {
				result[key] = requestAuditRedactedValue
				continue
			}

			result[key] = requestAuditRedactJSONValue(nestedValue)
		}

		return result
	case []any:
		result := make([]any, len(typed))
		for i, nestedValue := range typed {
			result[i] = requestAuditRedactJSONValue(nestedValue)
		}

		return result
	default:
		return value
	}
}

func requestAuditSensitiveJSONKey(key string) bool {
	normalized := requestAuditNormalizeKey(key)
	if normalized == "" {
		return false
	}

	if _, ok := requestAuditSensitiveJSONKeys[normalized]; ok {
		return true
	}

	if strings.HasSuffix(normalized, "password") ||
		strings.HasSuffix(normalized, "passphrase") ||
		strings.HasSuffix(normalized, "secret") ||
		strings.HasSuffix(normalized, "token") ||
		strings.HasSuffix(normalized, "apikey") ||
		strings.HasSuffix(normalized, "privatekey") ||
		strings.HasSuffix(normalized, "credential") ||
		strings.HasSuffix(normalized, "credentials") ||
		strings.HasSuffix(normalized, "session") ||
		strings.HasSuffix(normalized, "sessionid") ||
		strings.HasSuffix(normalized, "jwt") {
		return true
	}

	return false
}

func requestAuditNormalizeKey(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			builder.WriteRune(unicode.ToLower(character))
		}
	}

	return builder.String()
}

type requestAuditBodyCapture interface {
	Bytes() []byte
	Truncated() bool
	TotalBytes() int64
}

func requestAuditBodyBytes(capture requestAuditBodyCapture) []byte {
	if capture == nil {
		return nil
	}

	return capture.Bytes()
}

func requestAuditBodyTruncated(capture requestAuditBodyCapture) bool {
	if capture == nil {
		return false
	}

	return capture.Truncated()
}

type requestAuditBodyCaptureReadCloser struct {
	io.ReadCloser
	maxBytes   int
	buffer     bytes.Buffer
	totalBytes int64
	truncated  bool
}

func newRequestAuditBodyCaptureReadCloser(body io.ReadCloser, maxBytes int) *requestAuditBodyCaptureReadCloser {
	return &requestAuditBodyCaptureReadCloser{
		ReadCloser: body,
		maxBytes:   maxBytes,
	}
}

func (capture *requestAuditBodyCaptureReadCloser) Read(p []byte) (int, error) {
	n, err := capture.ReadCloser.Read(p)
	if n > 0 {
		capture.capture(p[:n])
	}

	return n, err
}

func (capture *requestAuditBodyCaptureReadCloser) Bytes() []byte {
	return append([]byte(nil), capture.buffer.Bytes()...)
}

func (capture *requestAuditBodyCaptureReadCloser) Truncated() bool {
	return capture.truncated
}

func (capture *requestAuditBodyCaptureReadCloser) TotalBytes() int64 {
	return capture.totalBytes
}

func (capture *requestAuditBodyCaptureReadCloser) capture(chunk []byte) {
	if len(chunk) == 0 {
		return
	}

	capture.totalBytes += int64(len(chunk))

	if capture.maxBytes <= 0 {
		capture.truncated = true
		return
	}

	remaining := capture.maxBytes - capture.buffer.Len()
	if remaining <= 0 {
		capture.truncated = true
		return
	}

	if len(chunk) > remaining {
		_, _ = capture.buffer.Write(chunk[:remaining])
		capture.truncated = true
		return
	}

	_, _ = capture.buffer.Write(chunk)
}

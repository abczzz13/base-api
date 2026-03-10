package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/abczzz13/base-api/internal/httpcapture"
	"github.com/abczzz13/base-api/internal/outboundaudit"
	"github.com/abczzz13/base-api/internal/requestid"
)

const (
	defaultMaxBodyBytes   = 64 * 1024
	maxErrorMessageLength = 512
)

var absoluteURLPattern = regexp.MustCompile(`https?://[^\s"']+`)

// Config configures a reusable outbound HTTP service.
type Config struct {
	Client string
	// BaseURL must be an origin-only absolute URL, such as https://api.example.com.
	BaseURL      string
	Timeout      time.Duration
	MaxBodyBytes int
	Metrics      *Metrics
	// AuditRepository is optional; when nil, outbound audit logging is disabled.
	AuditRepository       outboundaudit.Repository
	HTTPClient            *http.Client
	Transport             http.RoundTripper
	PropagateTraceContext bool
}

// Service wraps an instrumented HTTP client for outbound integrations.
type Service struct {
	client     string
	baseURL    *url.URL
	httpClient *http.Client
}

// New creates a reusable outbound HTTP service.
func New(cfg Config) (*Service, error) {
	clientName := strings.TrimSpace(cfg.Client)
	if clientName == "" {
		return nil, errors.New("client name is required")
	}

	baseURL, err := parseBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}

	maxBodyBytes := cfg.MaxBodyBytes
	if maxBodyBytes <= 0 {
		maxBodyBytes = defaultMaxBodyBytes
	}

	httpClient := &http.Client{}
	if cfg.HTTPClient != nil {
		copyClient := *cfg.HTTPClient
		httpClient = &copyClient
	}
	if cfg.Timeout > 0 {
		httpClient.Timeout = cfg.Timeout
	}

	baseTransport := cfg.Transport
	if baseTransport == nil {
		baseTransport = httpClient.Transport
	}
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}

	transportOptions := make([]otelhttp.Option, 0, 1)
	if !cfg.PropagateTraceContext {
		transportOptions = append(transportOptions, otelhttp.WithPropagators(propagation.NewCompositeTextMapPropagator()))
	}

	httpClient.Transport = otelhttp.NewTransport(&instrumentedTransport{
		base:                  baseTransport,
		client:                clientName,
		metrics:               cfg.Metrics,
		auditRepository:       cfg.AuditRepository,
		maxBodyBytes:          maxBodyBytes,
		propagateTraceContext: cfg.PropagateTraceContext,
	}, transportOptions...)

	return &Service{
		client:     clientName,
		baseURL:    baseURL,
		httpClient: httpClient,
	}, nil
}

// HTTPClient returns the wrapped HTTP client.
func (s *Service) HTTPClient() *http.Client {
	if s == nil {
		return nil
	}

	return s.httpClient
}

// NewRequest creates a request relative to the service base URL.
func (s *Service) NewRequest(ctx context.Context, operation, method, requestPath string, body io.Reader) (*http.Request, error) {
	if s == nil {
		return nil, errors.New("service is required")
	}

	resolvedURL, err := s.resolveURL(requestPath)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(contextOrBackground(ctx), method, resolvedURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	return withRequestMetadata(req, requestMetadata{client: s.client, operation: operation}), nil
}

// NewJSONRequest creates a JSON request relative to the service base URL.
func (s *Service) NewJSONRequest(ctx context.Context, operation, method, requestPath string, body any) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := s.NewRequest(ctx, operation, method, requestPath, reader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

// Do sends a prepared request with outbound instrumentation.
func (s *Service) Do(req *http.Request) (*http.Response, error) {
	if s == nil {
		return nil, errors.New("service is required")
	}
	if req == nil {
		return nil, errors.New("request is required")
	}

	preparedReq, err := s.prepareRequest(req)
	if err != nil {
		return nil, err
	}

	// #nosec G704 -- preparedReq origin is restricted to the configured base URL.
	return s.httpClient.Do(preparedReq)
}

func parseBaseURL(value string) (*url.URL, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, errors.New("base URL is required")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	if !parsed.IsAbs() {
		return nil, errors.New("base URL must be absolute")
	}
	if parsed.User != nil {
		return nil, errors.New("base URL must not include user info")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("base URL must not include query or fragment")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return nil, errors.New("base URL must not include a path; pass request paths to NewRequest instead")
	}

	parsed.Path = ""
	parsed.RawPath = ""

	return parsed, nil
}

func (s *Service) prepareRequest(req *http.Request) (*http.Request, error) {
	preparedReq := req.Clone(contextOrBackground(req.Context()))

	if preparedReq.URL == nil {
		return nil, errors.New("request URL is required")
	}

	resolvedURL, err := s.resolveParsedURL(preparedReq.URL)
	if err != nil {
		return nil, err
	}
	preparedReq.URL = resolvedURL

	metadata := requestMetadataFromContext(preparedReq.Context())
	if strings.TrimSpace(metadata.client) == "" {
		metadata.client = s.client
	}
	preparedReq = withRequestMetadata(preparedReq, metadata)

	return preparedReq, nil
}

func (s *Service) resolveURL(requestPath string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(requestPath))
	if err != nil {
		return nil, fmt.Errorf("parse request URL: %w", err)
	}

	return s.resolveParsedURL(parsed)
}

func (s *Service) resolveParsedURL(parsed *url.URL) (*url.URL, error) {
	if parsed == nil {
		return nil, errors.New("request URL is required")
	}
	if parsed.User != nil {
		return nil, errors.New("request URL must not include user info")
	}
	if parsed.Scheme == "" && strings.TrimSpace(parsed.Host) != "" {
		return nil, errors.New("request URL must not be scheme-relative")
	}
	if parsed.IsAbs() {
		if !sameOrigin(s.baseURL, parsed) {
			return nil, errors.New("absolute request URL must match the service base URL origin")
		}

		copyURL := *parsed
		copyURL.User = nil
		return &copyURL, nil
	}
	if s.baseURL == nil {
		return nil, errors.New("relative request URL requires a base URL")
	}

	return s.baseURL.ResolveReference(parsed), nil
}

func sameOrigin(baseURL, requestURL *url.URL) bool {
	if baseURL == nil || requestURL == nil {
		return false
	}

	return strings.EqualFold(strings.TrimSpace(baseURL.Scheme), strings.TrimSpace(requestURL.Scheme)) &&
		strings.EqualFold(strings.TrimSpace(baseURL.Hostname()), strings.TrimSpace(requestURL.Hostname())) &&
		effectivePort(baseURL) == effectivePort(requestURL)
}

func effectivePort(value *url.URL) string {
	if value == nil {
		return ""
	}

	if port := strings.TrimSpace(value.Port()); port != "" {
		return port
	}

	switch strings.ToLower(strings.TrimSpace(value.Scheme)) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

type instrumentedTransport struct {
	base                  http.RoundTripper
	client                string
	metrics               *Metrics
	auditRepository       outboundaudit.Repository
	maxBodyBytes          int
	propagateTraceContext bool
}

func (t *instrumentedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("request is required")
	}

	metadata := requestMetadataFromContext(req.Context())
	labels := normalizeLabels(firstNonEmpty(metadata.client, t.client), metadata.operation, req.Method)
	if t.metrics != nil {
		t.metrics.observeInFlightInc(labels)
	}

	requestBody := req.Body
	req = req.Clone(contextOrBackground(req.Context()))
	req.Body = requestBody
	if !t.propagateTraceContext {
		stripTraceContextHeaders(req.Header)
	}

	var requestCapture *httpcapture.CapturingReadCloser
	if req.Body != nil {
		requestCapture = httpcapture.NewCapturingReadCloser(req.Body, t.maxBodyBytes)
		req.Body = requestCapture
	}

	if span := trace.SpanFromContext(req.Context()); span.IsRecording() {
		span.SetName(labels.method + " " + labels.operation)
		span.SetAttributes(
			attribute.String("http.client.name", labels.client),
			attribute.String("http.client.operation", labels.operation),
			attribute.String("url.scheme", requestScheme(req)),
			attribute.String("server.address", requestHost(req)),
			attribute.String("url.path", requestPath(req)),
		)
	}

	startedAt := time.Now()
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		t.finalize(req, labels, startedAt, nil, requestCapture, nil, err)
		return nil, err
	}
	if resp == nil {
		err = errors.New("round tripper returned nil response")
		t.finalize(req, labels, startedAt, nil, requestCapture, nil, err)
		return nil, err
	}

	if resp.Body == nil || resp.Body == http.NoBody {
		t.finalize(req, labels, startedAt, resp, requestCapture, nil, nil)
		return resp, nil
	}

	responseCapture := httpcapture.NewCapturingReadCloser(resp.Body, t.maxBodyBytes)
	resp.Body = &auditedResponseBody{
		reader: responseCapture,
		finalize: func(readErr error) {
			t.finalize(req, labels, startedAt, resp, requestCapture, responseCapture, readErr)
		},
	}

	return resp, nil
}

func (t *instrumentedTransport) finalize(
	req *http.Request,
	labels metricLabels,
	startedAt time.Time,
	resp *http.Response,
	requestCapture *httpcapture.CapturingReadCloser,
	responseCapture *httpcapture.CapturingReadCloser,
	err error,
) {
	duration := time.Since(startedAt)
	statusCode := responseStatusCode(resp)
	requestSize := requestSizeBytes(req, requestCapture)
	responseSize := responseSizeBytes(resp, responseCapture)

	if t.metrics != nil {
		t.metrics.observeCompleted(labels, statusCode, duration, requestSize, responseSize, err)
		t.metrics.observeInFlightDec(labels)
	}

	if t.auditRepository == nil {
		return
	}

	record := outboundaudit.Record{
		Client:                labels.client,
		Operation:             labels.operation,
		Method:                labels.method,
		Path:                  requestPath(req),
		Query:                 requestQuery(req),
		Host:                  requestHost(req),
		Scheme:                requestScheme(req),
		StatusCode:            statusCode,
		Duration:              duration,
		RequestSizeBytes:      requestSize,
		ResponseSizeBytes:     responseSize,
		RequestHeaders:        redactHeaders(requestHeaders(req)),
		ResponseHeaders:       redactHeaders(responseHeaders(resp)),
		RequestBody:           redactBody(capturedBytes(requestCapture), captureTruncated(requestContentLength(req), requestCapture), requestContentType(req)),
		ResponseBody:          redactBody(capturedBytes(responseCapture), captureTruncated(responseContentLength(resp), responseCapture), responseContentType(resp)),
		RequestBodyTruncated:  captureTruncated(requestContentLength(req), requestCapture),
		ResponseBodyTruncated: captureTruncated(responseContentLength(resp), responseCapture),
		ErrorKind:             errorKind(err),
		ErrorMessage:          errorMessage(err),
		RequestID:             requestid.FromContext(contextOrBackground(req.Context())),
	}

	if spanContext := requestSpanContext(req); spanContext.IsValid() {
		record.TraceID = spanContext.TraceID().String()
		record.SpanID = spanContext.SpanID().String()
	}

	auditCtx := context.WithoutCancel(contextOrBackground(req.Context()))
	if auditErr := t.auditRepository.StoreOutboundAudit(auditCtx, record); auditErr != nil {
		slog.WarnContext(
			auditCtx,
			"outbound audit insert failed",
			slog.String("client", record.Client),
			slog.String("operation", record.Operation),
			slog.String("host", record.Host),
			slog.String("path", record.Path),
			slog.Any("error", auditErr),
		)
	}
}

func requestSpanContext(req *http.Request) trace.SpanContext {
	if req == nil {
		return trace.SpanContext{}
	}

	if spanContext := trace.SpanContextFromContext(propagation.TraceContext{}.Extract(context.Background(), propagation.HeaderCarrier(req.Header))); spanContext.IsValid() {
		return spanContext
	}

	return trace.SpanContextFromContext(contextOrBackground(req.Context()))
}

func stripTraceContextHeaders(headers http.Header) {
	if headers == nil {
		return
	}

	headers.Del("Traceparent")
	headers.Del("Tracestate")
	headers.Del("Baggage")
}

type auditedResponseBody struct {
	reader   *httpcapture.CapturingReadCloser
	finalize func(error)
	once     sync.Once
}

func (body *auditedResponseBody) Read(p []byte) (int, error) {
	n, err := body.reader.Read(p)
	if err != nil {
		if errors.Is(err, io.EOF) {
			body.runFinalize(nil)
		} else {
			body.runFinalize(err)
		}
	}

	return n, err
}

func (body *auditedResponseBody) Close() error {
	err := body.reader.Close()
	body.runFinalize(err)
	return err
}

func (body *auditedResponseBody) runFinalize(err error) {
	body.once.Do(func() {
		if body.finalize != nil {
			body.finalize(err)
		}
	})
}

type requestMetadata struct {
	client    string
	operation string
}

type requestMetadataKey struct{}

func withRequestMetadata(req *http.Request, metadata requestMetadata) *http.Request {
	if req == nil {
		return nil
	}

	ctx := context.WithValue(req.Context(), requestMetadataKey{}, metadata)
	return req.WithContext(ctx)
}

func requestMetadataFromContext(ctx context.Context) requestMetadata {
	if ctx == nil {
		return requestMetadata{}
	}

	metadata, _ := ctx.Value(requestMetadataKey{}).(requestMetadata)
	return metadata
}

func requestPath(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}

	return req.URL.Path
}

func requestQuery(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}

	return redactQuery(req.URL.RawQuery)
}

func requestHost(req *http.Request) string {
	if req == nil {
		return ""
	}
	if req.URL != nil {
		if host := strings.TrimSpace(req.URL.Host); host != "" {
			return host
		}
	}

	return strings.TrimSpace(req.Host)
}

func requestScheme(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}

	return strings.ToLower(strings.TrimSpace(req.URL.Scheme))
}

func requestHeaders(req *http.Request) http.Header {
	if req == nil || req.Header == nil {
		return http.Header{}
	}

	return req.Header.Clone()
}

func responseHeaders(resp *http.Response) http.Header {
	if resp == nil || resp.Header == nil {
		return http.Header{}
	}

	return resp.Header.Clone()
}

func requestContentType(req *http.Request) string {
	if req == nil {
		return ""
	}

	return req.Header.Get("Content-Type")
}

func responseContentType(resp *http.Response) string {
	if resp == nil {
		return ""
	}

	return resp.Header.Get("Content-Type")
}

func requestContentLength(req *http.Request) int64 {
	if req == nil {
		return -1
	}

	return req.ContentLength
}

func responseContentLength(resp *http.Response) int64 {
	if resp == nil {
		return -1
	}

	return resp.ContentLength
}

func requestSizeBytes(req *http.Request, capture *httpcapture.CapturingReadCloser) int64 {
	if req != nil && req.ContentLength >= 0 {
		return req.ContentLength
	}
	if capture == nil {
		return 0
	}

	return capture.TotalBytes()
}

func responseSizeBytes(resp *http.Response, capture *httpcapture.CapturingReadCloser) int64 {
	if resp != nil && resp.ContentLength >= 0 {
		return resp.ContentLength
	}
	if capture == nil {
		return 0
	}

	return capture.TotalBytes()
}

func responseStatusCode(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	if resp.StatusCode < 100 {
		return 0
	}
	if resp.StatusCode > 599 {
		return 599
	}

	return resp.StatusCode
}

func capturedBytes(capture *httpcapture.CapturingReadCloser) []byte {
	if capture == nil {
		return nil
	}

	return capture.Bytes()
}

func captureTruncated(contentLength int64, capture *httpcapture.CapturingReadCloser) bool {
	if capture == nil {
		return false
	}
	if capture.Truncated() {
		return true
	}
	if capture.Completed() {
		return false
	}
	if contentLength == 0 && capture.TotalBytes() == 0 {
		return false
	}
	if contentLength > 0 {
		return capture.TotalBytes() < contentLength
	}

	return capture.TotalBytes() > 0
}

func errorKind(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		var dnsErr *net.DNSError
		if errors.As(urlErr.Err, &dnsErr) {
			return "dns"
		}
		return "transport"
	}

	return "transport"
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "context canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "context deadline exceeded"
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		operation := strings.TrimSpace(urlErr.Op)
		inner := errorMessage(urlErr.Err)
		if inner == "" {
			inner = "request failed"
		}
		if operation == "" {
			return truncateErrorMessage(inner)
		}

		return truncateErrorMessage(operation + ": " + inner)
	}

	return truncateErrorMessage(sanitizeErrorText(err.Error()))
}

func sanitizeErrorText(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	return absoluteURLPattern.ReplaceAllString(trimmed, "[REDACTED_URL]")
}

func truncateErrorMessage(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= maxErrorMessageLength {
		return trimmed
	}

	return strings.TrimSpace(trimmed[:maxErrorMessageLength])
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}

	return ctx
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
}

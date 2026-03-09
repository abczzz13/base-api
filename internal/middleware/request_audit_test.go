package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"

	extclientip "github.com/abczzz13/clientip"
	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel/trace"

	"github.com/abczzz13/base-api/internal/clientip"
	"github.com/abczzz13/base-api/internal/ratelimit"
	"github.com/abczzz13/base-api/internal/requestaudit"
	"github.com/abczzz13/base-api/internal/requestid"
)

func TestRequestAuditRedactsSensitiveHeadersAndJSONBodies(t *testing.T) {
	store := &recordingRequestAuditStore{}

	handler := RequestID()(RequestAudit(RequestAuditConfig{
		Store:      store,
		Server:     "public",
		RouteLabel: func(*http.Request) string { return "createWidget" },
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Fatalf("read request body: %v", err)
		}

		w.Header().Set("Set-Cookie", "session=xyz")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true,"refresh_token":"response-token","nested":{"client_secret":"shh","visible":"yes"}}`))
	})))

	req := httptest.NewRequest(http.MethodPost, "https://api.example.test/widgets?region=eu", strings.NewReader(`{"username":"alice","password":"top-secret","profile":{"email":"alice@example.com"},"tokens":{"access_token":"abc","keep":"value"},"array":[{"client_secret":"x","name":"first"}]}`))
	req.RemoteAddr = "10.20.30.40:43123"
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Cookie", "session=xyz")
	req.Header.Set("X-API-Key", "api-key-value")
	req.Header.Set("X-Forwarded-For", "93.184.216.34, 10.20.30.40")
	req.Header.Set("User-Agent", "request-audit-test")
	req.Header.Set(requestid.HeaderName, "req-123")

	traceID, err := trace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("trace id from hex: %v", err)
	}
	spanID, err := trace.SpanIDFromHex("0123456789abcdef")
	if err != nil {
		t.Fatalf("span id from hex: %v", err)
	}
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	req = req.WithContext(trace.ContextWithSpanContext(req.Context(), spanContext))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusCreated, rr.Code)
	}

	if len(store.records) != 1 {
		t.Fatalf("expected one request audit record, got %d", len(store.records))
	}

	record := store.records[0]

	if record.Server != "public" {
		t.Fatalf("server mismatch: want %q, got %q", "public", record.Server)
	}
	if record.Route != "createWidget" {
		t.Fatalf("route mismatch: want %q, got %q", "createWidget", record.Route)
	}
	if record.Method != http.MethodPost {
		t.Fatalf("method mismatch: want %q, got %q", http.MethodPost, record.Method)
	}
	if record.Path != "/widgets" {
		t.Fatalf("path mismatch: want %q, got %q", "/widgets", record.Path)
	}
	if record.Query != "region=eu" {
		t.Fatalf("query mismatch: want %q, got %q", "region=eu", record.Query)
	}
	if record.Host != "api.example.test" {
		t.Fatalf("host mismatch: want %q, got %q", "api.example.test", record.Host)
	}
	if record.Scheme != "https" {
		t.Fatalf("scheme mismatch: want %q, got %q", "https", record.Scheme)
	}
	if record.StatusCode != http.StatusCreated {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusCreated, record.StatusCode)
	}
	if record.ClientIP != "93.184.216.34" {
		t.Fatalf("client ip mismatch: want %q, got %q", "93.184.216.34", record.ClientIP)
	}
	if record.TraceID != traceID.String() {
		t.Fatalf("trace id mismatch: want %q, got %q", traceID.String(), record.TraceID)
	}
	if record.SpanID != spanID.String() {
		t.Fatalf("span id mismatch: want %q, got %q", spanID.String(), record.SpanID)
	}
	if record.RequestID != "req-123" {
		t.Fatalf("request id mismatch: want %q, got %q", "req-123", record.RequestID)
	}
	if record.RequestSizeBytes != req.ContentLength {
		t.Fatalf("request size mismatch: want %d, got %d", req.ContentLength, record.RequestSizeBytes)
	}
	if record.ResponseSizeBytes != int64(rr.Body.Len()) {
		t.Fatalf("response size mismatch: want %d, got %d", rr.Body.Len(), record.ResponseSizeBytes)
	}

	for headerName := range map[string]struct{}{
		"Authorization": {},
		"Cookie":        {},
		"X-Api-Key":     {},
	} {
		if diff := cmp.Diff([]string{requestAuditRedactedValue}, record.RequestHeaders[headerName]); diff != "" {
			t.Fatalf("request header %q mismatch (-want +got):\n%s", headerName, diff)
		}
	}

	if diff := cmp.Diff([]string{"request-audit-test"}, record.RequestHeaders["User-Agent"]); diff != "" {
		t.Fatalf("request header %q mismatch (-want +got):\n%s", "User-Agent", diff)
	}

	if diff := cmp.Diff([]string{requestAuditRedactedValue}, record.ResponseHeaders["Set-Cookie"]); diff != "" {
		t.Fatalf("response header %q mismatch (-want +got):\n%s", "Set-Cookie", diff)
	}
	if diff := cmp.Diff([]string{"req-123"}, record.ResponseHeaders[requestid.HeaderName]); diff != "" {
		t.Fatalf("response header %q mismatch (-want +got):\n%s", "X-Request-Id", diff)
	}

	wantRequestBody := map[string]any{
		"array": []any{
			map[string]any{
				"client_secret": requestAuditRedactedValue,
				"name":          "first",
			},
		},
		"password": requestAuditRedactedValue,
		"profile": map[string]any{
			"email": "alice@example.com",
		},
		"tokens": map[string]any{
			"access_token": requestAuditRedactedValue,
			"keep":         "value",
		},
		"username": "alice",
	}
	if diff := cmp.Diff(wantRequestBody, decodeAuditJSON(t, record.RequestBody)); diff != "" {
		t.Fatalf("request body mismatch (-want +got):\n%s", diff)
	}

	wantResponseBody := map[string]any{
		"nested": map[string]any{
			"client_secret": requestAuditRedactedValue,
			"visible":       "yes",
		},
		"ok":            true,
		"refresh_token": requestAuditRedactedValue,
	}
	if diff := cmp.Diff(wantResponseBody, decodeAuditJSON(t, record.ResponseBody)); diff != "" {
		t.Fatalf("response body mismatch (-want +got):\n%s", diff)
	}
}

func TestSharedClientIPResolverCachesResolutionAcrossMiddlewares(t *testing.T) {
	store := &recordingRequestAuditStore{}
	extractor := &countingClientIPExtractor{addr: netip.MustParseAddr("93.184.216.34")}
	resolver := clientip.NewResolverWithExtractor(extractor)

	handler := RequestAudit(RequestAuditConfig{
		ClientIPResolver: resolver,
		Store:            store,
		Server:           "public",
		RouteLabel:       func(*http.Request) string { return "getHealthz" },
	})(RateLimit(RateLimitConfig{
		ClientIPResolver: resolver,
		Store: ratelimit.StoreFunc(func(context.Context, string, ratelimit.Policy) (ratelimit.Decision, error) {
			return ratelimit.Decision{Allowed: true}, nil
		}),
		Server:        "public",
		RouteLabel:    func(*http.Request) string { return "getHealthz" },
		DefaultPolicy: ratelimit.Policy{RequestsPerSecond: 1, Burst: 1},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))

	req := httptest.NewRequest(http.MethodGet, "https://api.example.test/healthz", nil)
	req.RemoteAddr = "10.20.30.40:43123"
	req.Header.Set("X-Forwarded-For", "93.184.216.34, 10.20.30.40")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusNoContent, rr.Code)
	}
	if got := extractor.Calls(); got != 1 {
		t.Fatalf("extractor calls mismatch: want %d, got %d", 1, got)
	}
	if len(store.records) != 1 {
		t.Fatalf("expected one request audit record, got %d", len(store.records))
	}
	if got := store.records[0].ClientIP; got != "93.184.216.34" {
		t.Fatalf("client ip mismatch: want %q, got %q", "93.184.216.34", got)
	}
}

func TestRequestAuditUsesTruncatedPlaceholdersWhenBodyCaptureLimitIsReached(t *testing.T) {
	store := &recordingRequestAuditStore{}

	handler := RequestAudit(RequestAuditConfig{
		Store:        store,
		Server:       "public",
		RouteLabel:   func(*http.Request) string { return "updateWidget" },
		MaxBodyBytes: 10,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Fatalf("read request body: %v", err)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"token":"response-token","visible":"ok"}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "http://api.example.test/widgets", strings.NewReader(`{"token":"request-token","visible":"ok"}`))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusOK, rr.Code)
	}

	if len(store.records) != 1 {
		t.Fatalf("expected one request audit record, got %d", len(store.records))
	}

	record := store.records[0]
	if !record.RequestBodyTruncated {
		t.Fatal("expected request body to be marked as truncated")
	}
	if !record.ResponseBodyTruncated {
		t.Fatal("expected response body to be marked as truncated")
	}
	if record.RequestSizeBytes != req.ContentLength {
		t.Fatalf("request size mismatch: want %d, got %d", req.ContentLength, record.RequestSizeBytes)
	}
	if record.ResponseSizeBytes != int64(rr.Body.Len()) {
		t.Fatalf("response size mismatch: want %d, got %d", rr.Body.Len(), record.ResponseSizeBytes)
	}

	wantPlaceholder := map[string]any{"_redacted": "truncated"}
	if diff := cmp.Diff(wantPlaceholder, decodeAuditJSON(t, record.RequestBody)); diff != "" {
		t.Fatalf("request body placeholder mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantPlaceholder, decodeAuditJSON(t, record.ResponseBody)); diff != "" {
		t.Fatalf("response body placeholder mismatch (-want +got):\n%s", diff)
	}
}

func TestRequestAuditRedactsSensitiveQueryParameters(t *testing.T) {
	store := &recordingRequestAuditStore{}

	handler := RequestAudit(RequestAuditConfig{
		Store:      store,
		Server:     "public",
		RouteLabel: func(*http.Request) string { return "getWidget" },
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(
		http.MethodGet,
		"https://api.example.test/widgets?region=eu&token=abc&session_id=s-123&refreshToken=xyz",
		nil,
	)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusNoContent, rr.Code)
	}

	if len(store.records) != 1 {
		t.Fatalf("expected one request audit record, got %d", len(store.records))
	}

	redactedQuery, err := url.ParseQuery(store.records[0].Query)
	if err != nil {
		t.Fatalf("parse redacted query: %v", err)
	}

	wantQuery := url.Values{
		"region":       {"eu"},
		"token":        {requestAuditRedactedValue},
		"session_id":   {requestAuditRedactedValue},
		"refreshToken": {requestAuditRedactedValue},
	}
	if diff := cmp.Diff(wantQuery, redactedQuery); diff != "" {
		t.Fatalf("query mismatch (-want +got):\n%s", diff)
	}
}

func TestRequestAuditUsesPlaceholderForMalformedQuery(t *testing.T) {
	req := &http.Request{
		URL: &url.URL{RawQuery: "access_token=%zz"},
	}

	if got := requestAuditQuery(req); got != requestAuditInvalidQueryValue {
		t.Fatalf("query mismatch: want %q, got %q", requestAuditInvalidQueryValue, got)
	}
}

func TestRequestAuditStrictModeRejectsInvalidXForwardedFor(t *testing.T) {
	store := &recordingRequestAuditStore{}

	handler := RequestAudit(RequestAuditConfig{
		Store:      store,
		Server:     "public",
		RouteLabel: func(*http.Request) string { return "getWidget" },
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://api.example.test/widgets/123", nil)
	req.RemoteAddr = "10.0.0.10:43123"
	req.Header.Set("X-Forwarded-For", "not-an-ip")
	req.Header.Set("X-Real-IP", "8.8.8.8")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusNoContent, rr.Code)
	}

	if len(store.records) != 1 {
		t.Fatalf("expected one request audit record, got %d", len(store.records))
	}

	record := store.records[0]
	if record.ClientIP != "" {
		t.Fatalf("client ip mismatch: want empty value, got %q", record.ClientIP)
	}
}

func TestRequestAuditStrictModeRejectsProxyHeadersForUntrustedRemoteAddr(t *testing.T) {
	store := &recordingRequestAuditStore{}

	handler := RequestAudit(RequestAuditConfig{
		Store:      store,
		Server:     "public",
		RouteLabel: func(*http.Request) string { return "getWidget" },
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://api.example.test/widgets/123", nil)
	req.RemoteAddr = "93.184.216.99:43123"
	req.Header.Set("X-Forwarded-For", "1.1.1.1")
	req.Header.Set("X-Real-IP", "8.8.8.8")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusNoContent, rr.Code)
	}

	if len(store.records) != 1 {
		t.Fatalf("expected one request audit record, got %d", len(store.records))
	}

	record := store.records[0]
	if record.ClientIP != "" {
		t.Fatalf("client ip mismatch: want empty value, got %q", record.ClientIP)
	}
}

func TestRequestAuditUsesConfiguredTrustedProxyCIDRs(t *testing.T) {
	store := &recordingRequestAuditStore{}

	handler := RequestAudit(RequestAuditConfig{
		Store:             store,
		Server:            "public",
		RouteLabel:        func(*http.Request) string { return "getWidget" },
		TrustedProxyCIDRs: []netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://api.example.test/widgets/123", nil)
	req.RemoteAddr = "203.0.113.10:43123"
	req.Header.Set("X-Forwarded-For", "8.8.8.8")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusNoContent, rr.Code)
	}

	if len(store.records) != 1 {
		t.Fatalf("expected one request audit record, got %d", len(store.records))
	}

	record := store.records[0]
	if record.ClientIP != "8.8.8.8" {
		t.Fatalf("client ip mismatch: want %q, got %q", "8.8.8.8", record.ClientIP)
	}
}

func TestRequestAuditUsesInvalidJSONPlaceholderForMalformedBodies(t *testing.T) {
	store := &recordingRequestAuditStore{}

	handler := RequestAudit(RequestAuditConfig{
		Store:      store,
		Server:     "public",
		RouteLabel: func(*http.Request) string { return "createWidget" },
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Fatalf("read request body: %v", err)
		}

		_, _ = w.Write([]byte("not-json"))
	}))

	req := httptest.NewRequest(http.MethodPost, "http://api.example.test/widgets", strings.NewReader("not-json"))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusOK, rr.Code)
	}

	if len(store.records) != 1 {
		t.Fatalf("expected one request audit record, got %d", len(store.records))
	}

	record := store.records[0]
	if record.RequestBodyTruncated {
		t.Fatal("did not expect truncated request body")
	}
	if record.ResponseBodyTruncated {
		t.Fatal("did not expect truncated response body")
	}
	if record.RequestSizeBytes != req.ContentLength {
		t.Fatalf("request size mismatch: want %d, got %d", req.ContentLength, record.RequestSizeBytes)
	}
	if record.ResponseSizeBytes != int64(rr.Body.Len()) {
		t.Fatalf("response size mismatch: want %d, got %d", rr.Body.Len(), record.ResponseSizeBytes)
	}

	wantPlaceholder := map[string]any{"_redacted": "invalid_json"}
	if diff := cmp.Diff(wantPlaceholder, decodeAuditJSON(t, record.RequestBody)); diff != "" {
		t.Fatalf("request body placeholder mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantPlaceholder, decodeAuditJSON(t, record.ResponseBody)); diff != "" {
		t.Fatalf("response body placeholder mismatch (-want +got):\n%s", diff)
	}
}

func TestRequestAuditStoreErrorsDoNotAffectResponses(t *testing.T) {
	store := &recordingRequestAuditStore{err: errors.New("insert failed")}

	handler := RequestAudit(RequestAuditConfig{
		Store:      store,
		Server:     "public",
		RouteLabel: func(*http.Request) string { return "getWidget" },
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	req := httptest.NewRequest(http.MethodGet, "http://api.example.test/widgets/123", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusAccepted, rr.Code)
	}
	if got := rr.Body.String(); got != `{"ok":true}` {
		t.Fatalf("response body mismatch: want %q, got %q", `{"ok":true}`, got)
	}

	if len(store.records) != 1 {
		t.Fatalf("expected one request audit record, got %d", len(store.records))
	}
}

type recordingRequestAuditStore struct {
	records []requestaudit.Record
	err     error
}

type countingClientIPExtractor struct {
	mu    sync.Mutex
	addr  netip.Addr
	err   error
	calls int
}

func (e *countingClientIPExtractor) ExtractAddr(*http.Request, ...extclientip.OverrideOptions) (netip.Addr, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++

	return e.addr, e.err
}

func (e *countingClientIPExtractor) Calls() int {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.calls
}

func (store *recordingRequestAuditStore) StoreRequestAudit(ctx context.Context, record requestaudit.Record) error {
	store.records = append(store.records, record)

	if _, ok := ctx.Deadline(); !ok {
		return errors.New("request audit store context is missing deadline")
	}

	return store.err
}

func decodeAuditJSON(t *testing.T, payload []byte) any {
	t.Helper()

	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode JSON payload %q: %v", string(payload), err)
	}

	return decoded
}

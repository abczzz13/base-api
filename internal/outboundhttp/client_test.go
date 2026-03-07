package outboundhttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/abczzz13/base-api/internal/outboundaudit"
	"github.com/abczzz13/base-api/internal/requestid"
)

func TestServiceDoRecordsMetricsAndAudit(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	provider := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		_ = provider.Shutdown(context.Background())
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got == "" {
			t.Fatalf("expected Authorization header to be set")
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "session=secret")
		_, _ = w.Write([]byte(`{"access_token":"server-secret","ok":true}`))
	}))
	t.Cleanup(server.Close)

	registry := prometheus.NewRegistry()
	metrics, err := NewMetrics(registry)
	if err != nil {
		t.Fatalf("NewMetrics returned error: %v", err)
	}

	auditStore := &recordingAuditRepository{}
	service, err := New(Config{
		Client:          "billing",
		BaseURL:         server.URL,
		Metrics:         metrics,
		AuditRepository: auditStore,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := service.NewJSONRequest(context.Background(), "create_invoice", http.MethodPost, "/invoices?token=secret", map[string]any{
		"customer_id": "123",
		"password":    "super-secret",
	})
	if err != nil {
		t.Fatalf("NewJSONRequest returned error: %v", err)
	}
	req = req.WithContext(requestid.WithContext(req.Context(), "req-123"))
	req.Header.Set("Authorization", "Bearer secret")

	resp, err := service.Do(req)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	if diff := cmp.Diff(`{"access_token":"server-secret","ok":true}`, string(body)); diff != "" {
		t.Fatalf("response body mismatch (-want +got):\n%s", diff)
	}

	if got := testutil.ToFloat64(metrics.requestsTotal.WithLabelValues("billing", "create_invoice", http.MethodPost, "200", resultSuccess)); got != 1 {
		t.Fatalf("requests_total mismatch: want 1, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.inFlightRequests.WithLabelValues("billing", "create_invoice", http.MethodPost)); got != 0 {
		t.Fatalf("in_flight_requests mismatch: want 0, got %v", got)
	}

	if got, want := len(auditStore.records), 1; got != want {
		t.Fatalf("audit record count mismatch: want %d, got %d", want, got)
	}

	record := auditStore.records[0]
	traceparent := record.RequestHeaders["Traceparent"]
	if len(traceparent) != 1 || traceparent[0] == "" {
		t.Fatalf("expected outbound traceparent header to be captured, got %#v", traceparent)
	}
	if diff := cmp.Diff("billing", record.Client); diff != "" {
		t.Fatalf("client mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff("create_invoice", record.Operation); diff != "" {
		t.Fatalf("operation mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(http.MethodPost, record.Method); diff != "" {
		t.Fatalf("method mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff("token=%5BREDACTED%5D", record.Query); diff != "" {
		t.Fatalf("query mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(map[string][]string{"Authorization": {"[REDACTED]"}, "Accept": {"application/json"}, "Content-Type": {"application/json"}, "Traceparent": traceparent}, filterHeaders(record.RequestHeaders, "Accept", "Authorization", "Content-Type", "Traceparent")); diff != "" {
		t.Fatalf("request headers mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(map[string][]string{"Set-Cookie": {"[REDACTED]"}, "Content-Type": {"application/json"}}, filterHeaders(record.ResponseHeaders, "Content-Type", "Set-Cookie")); diff != "" {
		t.Fatalf("response headers mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(`{"customer_id":"123","password":"[REDACTED]"}`, string(record.RequestBody)); diff != "" {
		t.Fatalf("request body mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(`{"access_token":"[REDACTED]","ok":true}`, string(record.ResponseBody)); diff != "" {
		t.Fatalf("response body mismatch (-want +got):\n%s", diff)
	}
	if record.TraceID == "" {
		t.Fatal("expected trace id to be recorded")
	}
	if record.SpanID == "" {
		t.Fatal("expected span id to be recorded")
	}
	if diff := cmp.Diff("req-123", record.RequestID); diff != "" {
		t.Fatalf("request ID mismatch (-want +got):\n%s", diff)
	}
}

func TestServiceDoRecordsTransportErrors(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := NewMetrics(registry)
	if err != nil {
		t.Fatalf("NewMetrics returned error: %v", err)
	}

	auditStore := &recordingAuditRepository{}
	service, err := New(Config{
		Client:          "billing",
		BaseURL:         "https://billing.example",
		Metrics:         metrics,
		AuditRepository: auditStore,
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial failed")
		}),
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := service.NewRequest(context.Background(), "get_invoice", http.MethodGet, "/invoices/123", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}

	_, err = service.Do(req)
	if err == nil {
		t.Fatal("expected transport error")
	}

	if got := testutil.ToFloat64(metrics.requestsTotal.WithLabelValues("billing", "get_invoice", http.MethodGet, "0", resultTransportError)); got != 1 {
		t.Fatalf("requests_total mismatch: want 1, got %v", got)
	}

	if got, want := len(auditStore.records), 1; got != want {
		t.Fatalf("audit record count mismatch: want %d, got %d", want, got)
	}

	record := auditStore.records[0]
	if diff := cmp.Diff("transport", record.ErrorKind); diff != "" {
		t.Fatalf("error kind mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(0, record.StatusCode); diff != "" {
		t.Fatalf("status code mismatch (-want +got):\n%s", diff)
	}
}

func TestServiceDoSanitizesTransportErrorMessages(t *testing.T) {
	auditStore := &recordingAuditRepository{}
	service, err := New(Config{
		Client:          "billing",
		BaseURL:         "https://billing.example",
		AuditRepository: auditStore,
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, &url.Error{
				Op:  http.MethodGet,
				URL: "https://billing.example/invoices/123?token=secret",
				Err: errors.New("dial failed"),
			}
		}),
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := service.NewRequest(context.Background(), "get_invoice", http.MethodGet, "/invoices/123?token=secret", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}

	_, err = service.Do(req)
	if err == nil {
		t.Fatal("expected transport error")
	}

	if got, want := len(auditStore.records), 1; got != want {
		t.Fatalf("audit record count mismatch: want %d, got %d", want, got)
	}

	record := auditStore.records[0]
	if diff := cmp.Diff("GET: dial failed", record.ErrorMessage); diff != "" {
		t.Fatalf("error message mismatch (-want +got):\n%s", diff)
	}
	if strings.Contains(record.ErrorMessage, "secret") {
		t.Fatalf("expected sanitized error message, got %q", record.ErrorMessage)
	}
	if diff := cmp.Diff("token=%5BREDACTED%5D", record.Query); diff != "" {
		t.Fatalf("query mismatch (-want +got):\n%s", diff)
	}
}

func TestServiceDoMarksEarlyClosedResponseBodiesAsTruncated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"password":"server-secret","items":[1,2,3,4,5]}`))
	}))
	t.Cleanup(server.Close)

	auditStore := &recordingAuditRepository{}
	service, err := New(Config{
		Client:          "billing",
		BaseURL:         server.URL,
		AuditRepository: auditStore,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := service.NewRequest(context.Background(), "stream_invoice", http.MethodGet, "/stream", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}

	resp, err := service.Do(req)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}

	buffer := make([]byte, 8)
	_, _ = resp.Body.Read(buffer)
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	if got, want := len(auditStore.records), 1; got != want {
		t.Fatalf("audit record count mismatch: want %d, got %d", want, got)
	}
	if !auditStore.records[0].ResponseBodyTruncated {
		t.Fatal("expected response body to be marked truncated")
	}
}

func TestServiceDoRejectsAbsoluteURLsOutsideBaseOrigin(t *testing.T) {
	service, err := New(Config{
		Client:  "billing",
		BaseURL: "https://billing.example",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://evil.example/steal", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext returned error: %v", err)
	}

	_, err = service.Do(req)
	if err == nil {
		t.Fatal("expected origin validation error")
	}
	if diff := cmp.Diff("absolute request URL must match the service base URL origin", err.Error()); diff != "" {
		t.Fatalf("error mismatch (-want +got):\n%s", diff)
	}
}

func TestServiceNewRequestRejectsSchemeRelativeURLs(t *testing.T) {
	service, err := New(Config{
		Client:  "billing",
		BaseURL: "https://billing.example",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = service.NewRequest(context.Background(), "get_invoice", http.MethodGet, "//evil.example/steal", nil)
	if err == nil {
		t.Fatal("expected scheme-relative URL validation error")
	}
	if diff := cmp.Diff("request URL must not be scheme-relative", err.Error()); diff != "" {
		t.Fatalf("error mismatch (-want +got):\n%s", diff)
	}
}

func TestServiceDoRejectsURLsWithUserInfo(t *testing.T) {
	service, err := New(Config{
		Client:  "billing",
		BaseURL: "https://billing.example",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://user:pass@billing.example/invoices/123", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext returned error: %v", err)
	}

	_, err = service.Do(req)
	if err == nil {
		t.Fatal("expected user info validation error")
	}
	if diff := cmp.Diff("request URL must not include user info", err.Error()); diff != "" {
		t.Fatalf("error mismatch (-want +got):\n%s", diff)
	}
}

func TestServiceDoAcceptsAbsoluteURLsWithExplicitDefaultPort(t *testing.T) {
	service, err := New(Config{
		Client:  "billing",
		BaseURL: "https://billing.example",
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if diff := cmp.Diff("billing.example:443", req.URL.Host); diff != "" {
				t.Fatalf("host mismatch (-want +got):\n%s", diff)
			}

			return &http.Response{
				StatusCode: http.StatusNoContent,
				Header:     make(http.Header),
				Body:       http.NoBody,
			}, nil
		}),
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://billing.example:443/invoices/123", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext returned error: %v", err)
	}

	resp, err := service.Do(req)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestNewRejectsBaseURLWithPath(t *testing.T) {
	service, err := New(Config{
		Client:  "billing",
		BaseURL: "https://billing.example/v1",
	})
	if service != nil {
		t.Fatal("expected nil service")
	}
	if err == nil {
		t.Fatal("expected error")
	}
	if diff := cmp.Diff("base URL must not include a path; pass request paths to NewRequest instead", err.Error()); diff != "" {
		t.Fatalf("error mismatch (-want +got):\n%s", diff)
	}
}

type recordingAuditRepository struct {
	records []outboundaudit.Record
}

func (repo *recordingAuditRepository) StoreOutboundAudit(_ context.Context, record outboundaudit.Record) error {
	repo.records = append(repo.records, record)
	return nil
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func filterHeaders(headers map[string][]string, keys ...string) map[string][]string {
	result := make(map[string][]string, len(keys))
	for _, key := range keys {
		if values, ok := headers[key]; ok {
			result[key] = values
		}
	}

	return result
}

package publicapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/infraapi"
	"github.com/abczzz13/base-api/internal/middleware"
	"github.com/abczzz13/base-api/internal/publicapi"
	"github.com/abczzz13/base-api/internal/requestaudit"
)

func TestRequestAuditOnlyWrapsPublicHandler(t *testing.T) {
	requestMetrics, registry := newRequestMetricsForTest(t)
	auditStore := &recordingRequestAuditStore{}

	publicHandler, err := publicapi.NewHandler(config.Config{Environment: "test"}, publicapi.Dependencies{
		RequestMetrics:         requestMetrics,
		RequestAuditRepository: auditStore,
	})
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	infraHandler, err := infraapi.NewHandler(config.Config{Environment: "test"}, infraapi.Dependencies{
		RequestMetrics:  requestMetrics,
		MetricsGatherer: registry,
	})
	if err != nil {
		t.Fatalf("infra NewHandler returned error: %v", err)
	}

	publicRecorder := httptest.NewRecorder()
	publicRequest := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	publicHandler.ServeHTTP(publicRecorder, publicRequest)

	if publicRecorder.Code != http.StatusOK {
		t.Fatalf("public status mismatch: want %d, got %d", http.StatusOK, publicRecorder.Code)
	}

	infraRecorder := httptest.NewRecorder()
	infraRequest := httptest.NewRequest(http.MethodGet, "/livez", nil)
	infraHandler.ServeHTTP(infraRecorder, infraRequest)

	if infraRecorder.Code != http.StatusOK {
		t.Fatalf("infra status mismatch: want %d, got %d", http.StatusOK, infraRecorder.Code)
	}

	if len(auditStore.records) != 1 {
		t.Fatalf("expected exactly one request audit record, got %d", len(auditStore.records))
	}

	record := auditStore.records[0]
	if record.Server != "public" {
		t.Fatalf("request audit server mismatch: want %q, got %q", "public", record.Server)
	}
	if record.Route != "getHealthz" {
		t.Fatalf("request audit route mismatch: want %q, got %q", "getHealthz", record.Route)
	}
	if record.Path != "/healthz" {
		t.Fatalf("request audit path mismatch: want %q, got %q", "/healthz", record.Path)
	}
}

func TestNewPublicHandlerRequiresAuditStoreWhenAuditEnabled(t *testing.T) {
	requestMetrics, _ := newRequestMetricsForTest(t)
	auditEnabled := true

	_, err := publicapi.NewHandler(config.Config{
		Environment: "test",
		RequestAudit: config.RequestAuditConfig{
			Enabled: &auditEnabled,
		},
	}, publicapi.Dependencies{
		RequestMetrics:         requestMetrics,
		RequestAuditRepository: nil,
	})
	if err == nil {
		t.Fatal("NewHandler returned nil error")
	}

	if !strings.Contains(err.Error(), "request audit store dependency is required") {
		t.Fatalf("NewHandler error mismatch: got %q", err.Error())
	}
}

func TestNewPublicHandlerDisablesRequestAuditMiddleware(t *testing.T) {
	requestMetrics, _ := newRequestMetricsForTest(t)
	auditStore := &recordingRequestAuditStore{}
	auditEnabled := false

	handler, err := publicapi.NewHandler(config.Config{
		Environment: "test",
		RequestAudit: config.RequestAuditConfig{
			Enabled: &auditEnabled,
		},
	}, publicapi.Dependencies{
		RequestMetrics:         requestMetrics,
		RequestAuditRepository: auditStore,
	})
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusOK, recorder.Code)
	}

	if len(auditStore.records) != 0 {
		t.Fatalf("expected no request audit records when middleware is disabled, got %d", len(auditStore.records))
	}
}

func TestNewPublicHandlerAllowsNilAuditStoreWhenAuditDisabled(t *testing.T) {
	requestMetrics, _ := newRequestMetricsForTest(t)
	auditEnabled := false

	handler, err := publicapi.NewHandler(config.Config{
		Environment: "test",
		RequestAudit: config.RequestAuditConfig{
			Enabled: &auditEnabled,
		},
	}, publicapi.Dependencies{
		RequestMetrics:         requestMetrics,
		RequestAuditRepository: nil,
	})
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusOK, recorder.Code)
	}
}

func newRequestMetricsForTest(t *testing.T) (*middleware.HTTPRequestMetrics, *prometheus.Registry) {
	t.Helper()

	registry := prometheus.NewRegistry()
	requestMetrics, err := middleware.NewHTTPRequestMetrics(registry)
	if err != nil {
		t.Fatalf("create request metrics: %v", err)
	}

	return requestMetrics, registry
}

type recordingRequestAuditStore struct {
	records []requestaudit.Record
}

func (store *recordingRequestAuditStore) StoreRequestAudit(_ context.Context, record requestaudit.Record) error {
	store.records = append(store.records, record)
	return nil
}

package requestaudit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

func TestAsyncRequestAuditStorePersistsRecords(t *testing.T) {
	t.Parallel()

	delegate := newBlockingRequestAuditStore(nil)
	store, shutdown := NewAsyncRepositoryWithConfig(delegate, AsyncConfig{
		QueueSize:    2,
		WriteTimeout: time.Second,
	})
	t.Cleanup(shutdown)

	record := Record{
		Method:     http.MethodGet,
		Path:       "/healthz",
		StatusCode: http.StatusOK,
	}
	if err := store.StoreRequestAudit(context.Background(), record); err != nil {
		t.Fatalf("StoreRequestAudit returned error: %v", err)
	}

	if !delegate.waitForRecords(1, time.Second) {
		t.Fatal("timed out waiting for async request-audit record")
	}
}

func TestAsyncRequestAuditStoreDropsRecordsWhenQueueIsFull(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	delegate := newBlockingRequestAuditStore(gate)
	store, shutdown := NewAsyncRepositoryWithConfig(delegate, AsyncConfig{
		QueueSize:    1,
		WriteTimeout: 5 * time.Second,
	})
	t.Cleanup(shutdown)

	makeRecord := func(path string) Record {
		return Record{
			Method:     http.MethodGet,
			Path:       path,
			StatusCode: http.StatusOK,
		}
	}

	if err := store.StoreRequestAudit(context.Background(), makeRecord("/one")); err != nil {
		t.Fatalf("StoreRequestAudit(first) returned error: %v", err)
	}

	if !delegate.waitForStarted(time.Second) {
		t.Fatal("worker did not start processing first record")
	}

	if err := store.StoreRequestAudit(context.Background(), makeRecord("/two")); err != nil {
		t.Fatalf("StoreRequestAudit(second) returned error: %v", err)
	}
	if err := store.StoreRequestAudit(context.Background(), makeRecord("/three")); err != nil {
		t.Fatalf("StoreRequestAudit(third) returned error: %v", err)
	}

	close(gate)
	shutdown()

	if got := delegate.recordCount(); got != 2 {
		t.Fatalf("record count mismatch: want %d, got %d", 2, got)
	}
}

func TestAsyncRequestAuditStoreShutdownTimeoutDropsQueuedRecords(t *testing.T) {
	delegate := RepositoryFunc(func(ctx context.Context, _ Record) error {
		<-ctx.Done()
		return ctx.Err()
	})

	metrics, err := NewMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("NewMetrics returned error: %v", err)
	}

	store, shutdown := NewAsyncRepositoryWithConfig(delegate, AsyncConfig{
		QueueSize:       32,
		WriteTimeout:    50 * time.Millisecond,
		ShutdownTimeout: 10 * time.Millisecond,
		Metrics:         metrics,
	})

	for i := 0; i < 20; i++ {
		record := Record{
			Server:     "public",
			Method:     http.MethodGet,
			Path:       fmt.Sprintf("/slow/%d", i),
			StatusCode: http.StatusOK,
		}
		if err := store.StoreRequestAudit(context.Background(), record); err != nil {
			t.Fatalf("StoreRequestAudit returned error: %v", err)
		}
	}

	startedAt := time.Now()
	shutdown()
	elapsed := time.Since(startedAt)

	if elapsed > 300*time.Millisecond {
		t.Fatalf("shutdown exceeded timeout budget: want <= 300ms, got %s", elapsed)
	}

	if got := testutil.ToFloat64(metrics.recordsTotal.WithLabelValues("public", requestAuditResultDroppedShutdown)); got < 1 {
		t.Fatalf("dropped shutdown counter mismatch: want >= 1, got %v", got)
	}

	if got := testutil.ToFloat64(metrics.queueDepth); got != 0 {
		t.Fatalf("queue depth mismatch after shutdown: want 0, got %v", got)
	}
}

func TestAsyncRequestAuditStoreTracksMetricsForOutcomes(t *testing.T) {
	gate := make(chan struct{})

	delegate := newBlockingRequestAuditStore(gate)
	delegate.storeFn = func(_ context.Context, record Record) error {
		if record.Path == "/fail" {
			return errors.New("insert failed")
		}

		return nil
	}

	registry := prometheus.NewRegistry()
	metrics, err := NewMetrics(registry)
	if err != nil {
		t.Fatalf("NewMetrics returned error: %v", err)
	}

	store, shutdown := NewAsyncRepositoryWithConfig(delegate, AsyncConfig{
		QueueSize:       1,
		WriteTimeout:    time.Second,
		ShutdownTimeout: time.Second,
		Metrics:         metrics,
	})
	t.Cleanup(shutdown)

	if err := store.StoreRequestAudit(context.Background(), Record{
		Server:     "public",
		Method:     http.MethodPost,
		Path:       "/fail",
		StatusCode: http.StatusInternalServerError,
	}); err != nil {
		t.Fatalf("StoreRequestAudit(first) returned error: %v", err)
	}

	if !delegate.waitForStarted(time.Second) {
		t.Fatal("worker did not start processing first record")
	}

	if err := store.StoreRequestAudit(context.Background(), Record{
		Server:     "public",
		Method:     http.MethodPost,
		Path:       "/ok",
		StatusCode: http.StatusOK,
	}); err != nil {
		t.Fatalf("StoreRequestAudit(second) returned error: %v", err)
	}

	if err := store.StoreRequestAudit(context.Background(), Record{
		Server:     "public",
		Method:     http.MethodPost,
		Path:       "/dropped",
		StatusCode: http.StatusAccepted,
	}); err != nil {
		t.Fatalf("StoreRequestAudit(third) returned error: %v", err)
	}

	close(gate)
	shutdown()

	if got := testutil.ToFloat64(metrics.recordsTotal.WithLabelValues("public", requestAuditResultEnqueued)); got != 2 {
		t.Fatalf("enqueued counter mismatch: want 2, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.recordsTotal.WithLabelValues("public", requestAuditResultDroppedQueueFull)); got != 1 {
		t.Fatalf("queue full drop counter mismatch: want 1, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.recordsTotal.WithLabelValues("public", requestAuditResultWriteError)); got != 1 {
		t.Fatalf("write error counter mismatch: want 1, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.recordsTotal.WithLabelValues("public", requestAuditResultStored)); got != 1 {
		t.Fatalf("stored counter mismatch: want 1, got %v", got)
	}

	if got := testutil.ToFloat64(metrics.queueDepth); got != 0 {
		t.Fatalf("queue depth mismatch after drain: want 0, got %v", got)
	}

	if got := histogramSampleCount(t, registry, "base_api_request_audit_write_duration_seconds", map[string]string{"server": "public", "result": requestAuditWriteResultError}); got != 1 {
		t.Fatalf("write duration error samples mismatch: want 1, got %d", got)
	}
	if got := histogramSampleCount(t, registry, "base_api_request_audit_write_duration_seconds", map[string]string{"server": "public", "result": requestAuditWriteResultSuccess}); got != 1 {
		t.Fatalf("write duration success samples mismatch: want 1, got %d", got)
	}
}

type blockingRequestAuditStore struct {
	block <-chan struct{}

	storeFn func(context.Context, Record) error

	started chan struct{}
	records chan Record

	mu    sync.Mutex
	count int
}

func newBlockingRequestAuditStore(block <-chan struct{}) *blockingRequestAuditStore {
	return &blockingRequestAuditStore{
		block:   block,
		started: make(chan struct{}, 1),
		records: make(chan Record, 8),
	}
}

func (store *blockingRequestAuditStore) StoreRequestAudit(ctx context.Context, record Record) error {
	select {
	case store.started <- struct{}{}:
	default:
	}

	if store.block != nil {
		<-store.block
	}

	store.mu.Lock()
	store.count++
	store.mu.Unlock()

	store.records <- record

	if store.storeFn != nil {
		return store.storeFn(ctx, record)
	}

	return nil
}

func (store *blockingRequestAuditStore) waitForStarted(timeout time.Duration) bool {
	select {
	case <-store.started:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (store *blockingRequestAuditStore) waitForRecords(want int, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for i := 0; i < want; i++ {
		select {
		case <-store.records:
		case <-deadline:
			return false
		}
	}

	return true
}

func (store *blockingRequestAuditStore) recordCount() int {
	store.mu.Lock()
	defer store.mu.Unlock()

	return store.count
}

func histogramSampleCount(t *testing.T, gatherer prometheus.Gatherer, familyName string, labels map[string]string) uint64 {
	t.Helper()

	families, err := gatherer.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	for _, family := range families {
		if family.GetName() != familyName {
			continue
		}

		for _, metric := range family.GetMetric() {
			if !metricHasLabels(metric, labels) {
				continue
			}

			histogram := metric.GetHistogram()
			if histogram == nil {
				t.Fatalf("metric %q is not a histogram", familyName)
			}

			return histogram.GetSampleCount()
		}
	}

	return 0
}

func metricHasLabels(metric *dto.Metric, labels map[string]string) bool {
	for key, want := range labels {
		found := false
		for _, label := range metric.GetLabel() {
			if label.GetName() != key {
				continue
			}

			found = label.GetValue() == want
			break
		}

		if !found {
			return false
		}
	}

	return true
}

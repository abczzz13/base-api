package outboundaudit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

func TestAsyncRepositoryPersistsRecords(t *testing.T) {
	t.Parallel()

	delegate := newBlockingRepository(nil)
	store, shutdown := NewAsyncRepositoryWithConfig(delegate, AsyncConfig{
		QueueSize:    2,
		WriteTimeout: time.Second,
	})
	t.Cleanup(shutdown)

	record := Record{
		Client:    "billing",
		Operation: "get_invoice",
		Method:    "GET",
		Host:      "billing.example",
		Path:      "/invoices/123",
	}
	if err := store.StoreOutboundAudit(context.Background(), record); err != nil {
		t.Fatalf("StoreOutboundAudit returned error: %v", err)
	}

	if !delegate.waitForRecords(1, time.Second) {
		t.Fatal("timed out waiting for async outbound audit record")
	}
}

func TestAsyncRepositoryDropsRecordsWhenQueueIsFull(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	delegate := newBlockingRepository(gate)
	store, shutdown := NewAsyncRepositoryWithConfig(delegate, AsyncConfig{
		QueueSize:    1,
		WriteTimeout: 5 * time.Second,
	})
	t.Cleanup(shutdown)

	makeRecord := func(path string) Record {
		return Record{
			Client:    "billing",
			Operation: "lookup",
			Method:    "GET",
			Host:      "billing.example",
			Path:      path,
		}
	}

	if err := store.StoreOutboundAudit(context.Background(), makeRecord("/one")); err != nil {
		t.Fatalf("StoreOutboundAudit(first) returned error: %v", err)
	}

	if !delegate.waitForStarted(time.Second) {
		t.Fatal("worker did not start processing first record")
	}

	if err := store.StoreOutboundAudit(context.Background(), makeRecord("/two")); err != nil {
		t.Fatalf("StoreOutboundAudit(second) returned error: %v", err)
	}
	if err := store.StoreOutboundAudit(context.Background(), makeRecord("/three")); err != nil {
		t.Fatalf("StoreOutboundAudit(third) returned error: %v", err)
	}

	close(gate)
	shutdown()

	if got := delegate.recordCount(); got != 2 {
		t.Fatalf("record count mismatch: want %d, got %d", 2, got)
	}
}

func TestAsyncRepositoryShutdownTimeoutDropsQueuedRecords(t *testing.T) {
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
			Client:    "billing",
			Operation: "slow_lookup",
			Method:    "GET",
			Host:      "billing.example",
			Path:      fmt.Sprintf("/slow/%d", i),
		}
		if err := store.StoreOutboundAudit(context.Background(), record); err != nil {
			t.Fatalf("StoreOutboundAudit returned error: %v", err)
		}
	}

	startedAt := time.Now()
	shutdown()
	elapsed := time.Since(startedAt)

	if elapsed > 300*time.Millisecond {
		t.Fatalf("shutdown exceeded timeout budget: want <= 300ms, got %s", elapsed)
	}

	if got := testutil.ToFloat64(metrics.recordsTotal.WithLabelValues("billing", outboundAuditResultDroppedShutdown)); got < 1 {
		t.Fatalf("dropped shutdown counter mismatch: want >= 1, got %v", got)
	}

	if got := testutil.ToFloat64(metrics.queueDepth); got != 0 {
		t.Fatalf("queue depth mismatch after shutdown: want 0, got %v", got)
	}
}

func TestAsyncRepositoryTracksMetricsForOutcomes(t *testing.T) {
	gate := make(chan struct{})

	delegate := newBlockingRepository(gate)
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

	if err := store.StoreOutboundAudit(context.Background(), Record{
		Client:    "billing",
		Operation: "create_invoice",
		Method:    "POST",
		Host:      "billing.example",
		Path:      "/fail",
	}); err != nil {
		t.Fatalf("StoreOutboundAudit(first) returned error: %v", err)
	}

	if !delegate.waitForStarted(time.Second) {
		t.Fatal("worker did not start processing first record")
	}

	if err := store.StoreOutboundAudit(context.Background(), Record{
		Client:    "billing",
		Operation: "create_invoice",
		Method:    "POST",
		Host:      "billing.example",
		Path:      "/ok",
	}); err != nil {
		t.Fatalf("StoreOutboundAudit(second) returned error: %v", err)
	}

	if err := store.StoreOutboundAudit(context.Background(), Record{
		Client:    "billing",
		Operation: "create_invoice",
		Method:    "POST",
		Host:      "billing.example",
		Path:      "/dropped",
	}); err != nil {
		t.Fatalf("StoreOutboundAudit(third) returned error: %v", err)
	}

	close(gate)
	shutdown()

	if got := testutil.ToFloat64(metrics.recordsTotal.WithLabelValues("billing", outboundAuditResultEnqueued)); got != 2 {
		t.Fatalf("enqueued counter mismatch: want 2, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.recordsTotal.WithLabelValues("billing", outboundAuditResultDroppedQueueFull)); got != 1 {
		t.Fatalf("queue full drop counter mismatch: want 1, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.recordsTotal.WithLabelValues("billing", outboundAuditResultWriteError)); got != 1 {
		t.Fatalf("write error counter mismatch: want 1, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.recordsTotal.WithLabelValues("billing", outboundAuditResultStored)); got != 1 {
		t.Fatalf("stored counter mismatch: want 1, got %v", got)
	}

	if got := testutil.ToFloat64(metrics.queueDepth); got != 0 {
		t.Fatalf("queue depth mismatch after drain: want 0, got %v", got)
	}

	if got := histogramSampleCount(t, registry, "base_api_http_client_audit_write_duration_seconds", map[string]string{"client": "billing", "result": outboundAuditWriteResultError}); got != 1 {
		t.Fatalf("write duration error samples mismatch: want 1, got %d", got)
	}
	if got := histogramSampleCount(t, registry, "base_api_http_client_audit_write_duration_seconds", map[string]string{"client": "billing", "result": outboundAuditWriteResultSuccess}); got != 1 {
		t.Fatalf("write duration success samples mismatch: want 1, got %d", got)
	}
}

type blockingRepository struct {
	block <-chan struct{}

	storeFn func(context.Context, Record) error

	started chan struct{}
	records chan Record

	mu    sync.Mutex
	count int
}

func newBlockingRepository(block <-chan struct{}) *blockingRepository {
	return &blockingRepository{
		block:   block,
		started: make(chan struct{}, 1),
		records: make(chan Record, 8),
	}
}

func (store *blockingRepository) StoreOutboundAudit(ctx context.Context, record Record) error {
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

func (store *blockingRepository) waitForStarted(timeout time.Duration) bool {
	select {
	case <-store.started:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (store *blockingRepository) waitForRecords(want int, timeout time.Duration) bool {
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

func (store *blockingRepository) recordCount() int {
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

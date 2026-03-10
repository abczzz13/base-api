package asyncaudit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

type testRecord struct {
	Label string
	Path  string
}

func testParams(store func(context.Context, testRecord) error) Params[testRecord] {
	return Params[testRecord]{
		Store:       store,
		MetricLabel: func(r testRecord) string { return r.Label },
		LogAttrs: func(r testRecord) []slog.Attr {
			return []slog.Attr{slog.String("path", r.Path)}
		},
		EntityName: "test audit",
	}
}

func testMetricsConfig() MetricsConfig {
	return MetricsConfig{
		Namespace:    "test",
		Subsystem:    "audit",
		LabelName:    "label",
		UnknownLabel: "unknown",
		HelpPrefix:   "test audit",
	}
}

func TestAsyncRepositoryPersistsRecords(t *testing.T) {
	t.Parallel()

	delegate := newBlockingStore(nil)
	repo, shutdown := New[testRecord](testParams(delegate.store), Config{
		QueueSize:    2,
		WriteTimeout: time.Second,
	})
	t.Cleanup(shutdown)

	record := testRecord{Label: "svc", Path: "/healthz"}
	if err := repo.Store(context.Background(), record); err != nil {
		t.Fatalf("Store returned error: %v", err)
	}

	if !delegate.waitForRecords(1, time.Second) {
		t.Fatal("timed out waiting for async record")
	}
}

func TestAsyncRepositoryDropsRecordsWhenQueueIsFull(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	delegate := newBlockingStore(gate)
	repo, shutdown := New[testRecord](testParams(delegate.store), Config{
		QueueSize:    1,
		WriteTimeout: 5 * time.Second,
	})
	t.Cleanup(shutdown)

	if err := repo.Store(context.Background(), testRecord{Label: "svc", Path: "/one"}); err != nil {
		t.Fatalf("Store(first) returned error: %v", err)
	}

	if !delegate.waitForStarted(time.Second) {
		t.Fatal("worker did not start processing first record")
	}

	if err := repo.Store(context.Background(), testRecord{Label: "svc", Path: "/two"}); err != nil {
		t.Fatalf("Store(second) returned error: %v", err)
	}
	if err := repo.Store(context.Background(), testRecord{Label: "svc", Path: "/three"}); err != nil {
		t.Fatalf("Store(third) returned error: %v", err)
	}

	close(gate)
	shutdown()

	if got := delegate.recordCount(); got != 2 {
		t.Fatalf("record count mismatch: want %d, got %d", 2, got)
	}
}

func TestAsyncRepositoryShutdownTimeoutDropsQueuedRecords(t *testing.T) {
	delegate := func(ctx context.Context, _ testRecord) error {
		<-ctx.Done()
		return ctx.Err()
	}

	metrics, err := NewMetrics(prometheus.NewRegistry(), testMetricsConfig())
	if err != nil {
		t.Fatalf("NewMetrics returned error: %v", err)
	}

	repo, shutdown := New[testRecord](testParams(delegate), Config{
		QueueSize:       32,
		WriteTimeout:    50 * time.Millisecond,
		ShutdownTimeout: 10 * time.Millisecond,
		Metrics:         metrics,
	})

	for i := 0; i < 20; i++ {
		record := testRecord{Label: "svc", Path: fmt.Sprintf("/slow/%d", i)}
		if err := repo.Store(context.Background(), record); err != nil {
			t.Fatalf("Store returned error: %v", err)
		}
	}

	startedAt := time.Now()
	shutdown()
	elapsed := time.Since(startedAt)

	if elapsed > 300*time.Millisecond {
		t.Fatalf("shutdown exceeded timeout budget: want <= 300ms, got %s", elapsed)
	}

	if got := testutil.ToFloat64(metrics.recordsTotal.WithLabelValues("svc", ResultDroppedShutdown)); got < 1 {
		t.Fatalf("dropped shutdown counter mismatch: want >= 1, got %v", got)
	}

	if got := testutil.ToFloat64(metrics.queueDepth); got != 0 {
		t.Fatalf("queue depth mismatch after shutdown: want 0, got %v", got)
	}
}

func TestAsyncRepositoryTracksMetricsForOutcomes(t *testing.T) {
	gate := make(chan struct{})

	delegate := newBlockingStore(gate)
	delegate.storeFn = func(_ context.Context, record testRecord) error {
		if record.Path == "/fail" {
			return errors.New("insert failed")
		}

		return nil
	}

	registry := prometheus.NewRegistry()
	metrics, err := NewMetrics(registry, testMetricsConfig())
	if err != nil {
		t.Fatalf("NewMetrics returned error: %v", err)
	}

	repo, shutdown := New[testRecord](testParams(delegate.store), Config{
		QueueSize:       1,
		WriteTimeout:    time.Second,
		ShutdownTimeout: time.Second,
		Metrics:         metrics,
	})
	t.Cleanup(shutdown)

	if err := repo.Store(context.Background(), testRecord{Label: "svc", Path: "/fail"}); err != nil {
		t.Fatalf("Store(first) returned error: %v", err)
	}

	if !delegate.waitForStarted(time.Second) {
		t.Fatal("worker did not start processing first record")
	}

	if err := repo.Store(context.Background(), testRecord{Label: "svc", Path: "/ok"}); err != nil {
		t.Fatalf("Store(second) returned error: %v", err)
	}

	if err := repo.Store(context.Background(), testRecord{Label: "svc", Path: "/dropped"}); err != nil {
		t.Fatalf("Store(third) returned error: %v", err)
	}

	close(gate)
	shutdown()

	if got := testutil.ToFloat64(metrics.recordsTotal.WithLabelValues("svc", ResultEnqueued)); got != 2 {
		t.Fatalf("enqueued counter mismatch: want 2, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.recordsTotal.WithLabelValues("svc", ResultDroppedQueueFull)); got != 1 {
		t.Fatalf("queue full drop counter mismatch: want 1, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.recordsTotal.WithLabelValues("svc", ResultWriteError)); got != 1 {
		t.Fatalf("write error counter mismatch: want 1, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.recordsTotal.WithLabelValues("svc", ResultStored)); got != 1 {
		t.Fatalf("stored counter mismatch: want 1, got %v", got)
	}

	if got := testutil.ToFloat64(metrics.queueDepth); got != 0 {
		t.Fatalf("queue depth mismatch after drain: want 0, got %v", got)
	}

	if got := histogramSampleCount(t, registry, "test_audit_write_duration_seconds", map[string]string{"label": "svc", "result": WriteResultError}); got != 1 {
		t.Fatalf("write duration error samples mismatch: want 1, got %d", got)
	}
	if got := histogramSampleCount(t, registry, "test_audit_write_duration_seconds", map[string]string{"label": "svc", "result": WriteResultSuccess}); got != 1 {
		t.Fatalf("write duration success samples mismatch: want 1, got %d", got)
	}
}

func TestNewAppliesSafeDefaultsForNilParams(t *testing.T) {
	t.Parallel()

	registry := prometheus.NewRegistry()
	metrics, err := NewMetrics(registry, testMetricsConfig())
	if err != nil {
		t.Fatalf("NewMetrics returned error: %v", err)
	}

	repo, shutdown := New[testRecord](Params[testRecord]{}, Config{
		QueueSize:       1,
		WriteTimeout:    time.Second,
		ShutdownTimeout: time.Second,
		Metrics:         metrics,
	})

	if err := repo.Store(context.Background(), testRecord{}); err != nil {
		t.Fatalf("Store returned error: %v", err)
	}

	shutdown()

	if got := testutil.ToFloat64(metrics.recordsTotal.WithLabelValues("unknown", ResultEnqueued)); got != 1 {
		t.Fatalf("enqueued counter mismatch: want 1, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.recordsTotal.WithLabelValues("unknown", ResultStored)); got != 1 {
		t.Fatalf("stored counter mismatch: want 1, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.queueDepth); got != 0 {
		t.Fatalf("queue depth mismatch after shutdown: want 0, got %v", got)
	}
	if got := histogramSampleCount(t, registry, "test_audit_write_duration_seconds", map[string]string{"label": "unknown", "result": WriteResultSuccess}); got != 1 {
		t.Fatalf("write duration success samples mismatch: want 1, got %d", got)
	}
}

type blockingStore struct {
	block <-chan struct{}

	storeFn func(context.Context, testRecord) error

	started chan struct{}
	records chan testRecord

	mu    sync.Mutex
	count int
}

func newBlockingStore(block <-chan struct{}) *blockingStore {
	return &blockingStore{
		block:   block,
		started: make(chan struct{}, 1),
		records: make(chan testRecord, 8),
	}
}

func (s *blockingStore) store(ctx context.Context, record testRecord) error {
	select {
	case s.started <- struct{}{}:
	default:
	}

	if s.block != nil {
		<-s.block
	}

	s.mu.Lock()
	s.count++
	s.mu.Unlock()

	s.records <- record

	if s.storeFn != nil {
		return s.storeFn(ctx, record)
	}

	return nil
}

func (s *blockingStore) waitForStarted(timeout time.Duration) bool {
	select {
	case <-s.started:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (s *blockingStore) waitForRecords(want int, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for i := 0; i < want; i++ {
		select {
		case <-s.records:
		case <-deadline:
			return false
		}
	}

	return true
}

func (s *blockingStore) recordCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.count
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

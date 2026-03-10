package asyncaudit

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const (
	defaultQueueSize       = 1024
	defaultWriteTimeout    = 250 * time.Millisecond
	defaultShutdownTimeout = 2 * time.Second
)

// Config configures asynchronous audit writes.
type Config struct {
	QueueSize       int
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
	Metrics         *Metrics
}

// Params parameterizes a Repository with record-specific behavior.
type Params[R any] struct {
	// Store writes a single record to the underlying storage.
	Store func(context.Context, R) error
	// MetricLabel returns the primary label value for a record (e.g. server or client name).
	MetricLabel func(R) string
	// LogAttrs returns structured log attributes for warning messages about a record.
	LogAttrs func(R) []slog.Attr
	// EntityName appears in log messages (e.g. "request audit", "outbound audit").
	EntityName string
}

type job[R any] struct {
	ctx    context.Context
	record R
}

// Repository is a generic queue-backed asynchronous audit writer.
type Repository[R any] struct {
	params          Params[R]
	writeTimeout    time.Duration
	shutdownTimeout time.Duration
	metrics         *Metrics
	queue           chan job[R]
	stopWorkerCh    chan struct{}

	mu        sync.RWMutex
	closed    bool
	closeOnce sync.Once
	stopOnce  sync.Once
	workerWG  sync.WaitGroup
}

// New wraps a store function with async, queue-backed writes.
// Returns the repository and a shutdown function.
func New[R any](params Params[R], cfg Config) (*Repository[R], func()) {
	params = defaultParams(params)

	queueSize := cfg.QueueSize
	if queueSize <= 0 {
		queueSize = defaultQueueSize
	}

	writeTimeout := cfg.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = defaultWriteTimeout
	}

	shutdownTimeout := cfg.ShutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = defaultShutdownTimeout
	}

	repo := &Repository[R]{
		params:          params,
		writeTimeout:    writeTimeout,
		shutdownTimeout: shutdownTimeout,
		metrics:         cfg.Metrics,
		queue:           make(chan job[R], queueSize),
		stopWorkerCh:    make(chan struct{}),
	}
	repo.observeQueueDepth(0)
	repo.startWorker()

	return repo, repo.Close
}

func defaultParams[R any](params Params[R]) Params[R] {
	if params.Store == nil {
		params.Store = func(context.Context, R) error { return nil }
	}
	if params.MetricLabel == nil {
		params.MetricLabel = func(R) string { return "" }
	}
	if params.LogAttrs == nil {
		params.LogAttrs = func(R) []slog.Attr { return nil }
	}
	if strings.TrimSpace(params.EntityName) == "" {
		params.EntityName = "audit"
	}

	return params
}

// Store enqueues a record for asynchronous persistence.
func (repo *Repository[R]) Store(ctx context.Context, record R) error {
	if repo == nil {
		return nil
	}

	j := job[R]{
		ctx:    context.WithoutCancel(ContextOrBackground(ctx)),
		record: record,
	}

	repo.mu.RLock()
	if repo.closed {
		repo.mu.RUnlock()
		repo.observeRecordResult(record, ResultDroppedShutdown)
		return nil
	}

	select {
	case repo.queue <- j:
		repo.observeRecordResult(record, ResultEnqueued)
		repo.observeQueueDepth(len(repo.queue))
		repo.mu.RUnlock()
		return nil
	default:
		repo.mu.RUnlock()
		repo.observeRecordResult(record, ResultDroppedQueueFull)
		repo.observeQueueDepth(len(repo.queue))

		attrs := repo.params.LogAttrs(record)
		slog.WarnContext(
			ContextOrBackground(ctx),
			repo.params.EntityName+" queue full; dropping record",
			groupAttrs(attrs)...,
		)
		return nil
	}
}

// Close performs a graceful shutdown, draining queued records up to the configured timeout.
func (repo *Repository[R]) Close() {
	if repo == nil {
		return
	}

	repo.closeOnce.Do(func() {
		repo.mu.Lock()
		if !repo.closed {
			repo.closed = true
			close(repo.queue)
		}
		repo.mu.Unlock()

		waitDone := make(chan struct{})
		go func() {
			repo.workerWG.Wait()
			close(waitDone)
		}()

		timer := time.NewTimer(repo.shutdownTimeout)
		defer timer.Stop()

		select {
		case <-waitDone:
			repo.observeQueueDepth(0)
		case <-timer.C:
			slog.Warn(
				repo.params.EntityName+" async shutdown timed out; forcing queue drop",
				slog.Duration("shutdown_timeout", repo.shutdownTimeout),
			)
			repo.signalStopWorker()
			<-waitDone
			repo.observeQueueDepth(0)
		}
	})
}

func (repo *Repository[R]) startWorker() {
	repo.workerWG.Add(1)
	go repo.runWorker()
}

func (repo *Repository[R]) runWorker() {
	defer repo.workerWG.Done()

	for {
		if repo.shouldStopWorker() {
			return
		}

		j, ok := <-repo.queue
		if !ok {
			repo.observeQueueDepth(0)
			return
		}
		repo.observeQueueDepth(len(repo.queue))

		startedAt := time.Now()
		if err := repo.writeRecord(j); err != nil {
			repo.observeRecordResult(j.record, ResultWriteError)
			repo.observeWriteDuration(j.record, time.Since(startedAt), writeResultFromError(err))

			attrs := append(repo.params.LogAttrs(j.record), slog.Any("error", err))
			slog.WarnContext(
				ContextOrBackground(j.ctx),
				repo.params.EntityName+" insert failed",
				groupAttrs(attrs)...,
			)
			continue
		}

		repo.observeRecordResult(j.record, ResultStored)
		repo.observeWriteDuration(j.record, time.Since(startedAt), WriteResultSuccess)
	}
}

func (repo *Repository[R]) shouldStopWorker() bool {
	select {
	case <-repo.stopWorkerCh:
		droppedCount := repo.dropQueuedJobs()
		if droppedCount > 0 {
			slog.Warn(
				repo.params.EntityName+" shutdown dropped queued records",
				slog.Int("dropped_count", droppedCount),
			)
		}
		return true
	default:
		return false
	}
}

func (repo *Repository[R]) dropQueuedJobs() int {
	droppedCount := 0

	for {
		select {
		case j, ok := <-repo.queue:
			if !ok {
				repo.observeQueueDepth(0)
				return droppedCount
			}

			droppedCount++
			repo.observeRecordResult(j.record, ResultDroppedShutdown)
			repo.observeQueueDepth(len(repo.queue))
		default:
			repo.observeQueueDepth(len(repo.queue))
			return droppedCount
		}
	}
}

func (repo *Repository[R]) signalStopWorker() {
	repo.stopOnce.Do(func() {
		close(repo.stopWorkerCh)
	})
}

func (repo *Repository[R]) observeRecordResult(record R, result string) {
	if repo == nil || repo.metrics == nil {
		return
	}

	repo.metrics.observeRecordResult(repo.params.MetricLabel(record), result)
}

func (repo *Repository[R]) observeWriteDuration(record R, duration time.Duration, result string) {
	if repo == nil || repo.metrics == nil {
		return
	}

	repo.metrics.observeWriteDuration(repo.params.MetricLabel(record), duration, result)
}

func (repo *Repository[R]) observeQueueDepth(depth int) {
	if repo == nil || repo.metrics == nil {
		return
	}

	repo.metrics.observeQueueDepth(depth)
}

func (repo *Repository[R]) writeRecord(j job[R]) error {
	writeCtx := ContextOrBackground(j.ctx)
	cancel := func() {}
	if repo.writeTimeout > 0 {
		writeCtx, cancel = context.WithTimeout(writeCtx, repo.writeTimeout)
	}
	defer cancel()

	return repo.params.Store(writeCtx, j.record)
}

func writeResultFromError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return WriteResultTimeout
	}

	return WriteResultError
}

func groupAttrs(attrs []slog.Attr) []any {
	result := make([]any, len(attrs))
	for i, attr := range attrs {
		result[i] = attr
	}

	return result
}

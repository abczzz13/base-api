package requestaudit

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

const (
	defaultAsyncQueueSize       = 1024
	defaultAsyncWriteTimeout    = 250 * time.Millisecond
	defaultAsyncShutdownTimeout = 2 * time.Second
)

// AsyncConfig configures asynchronous request-audit writes.
type AsyncConfig struct {
	QueueSize       int
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
	Metrics         *Metrics
}

type asyncJob struct {
	ctx    context.Context
	record Record
}

type asyncRepository struct {
	repository      Repository
	writeTimeout    time.Duration
	shutdownTimeout time.Duration
	metrics         *Metrics
	queue           chan asyncJob
	stopWorkerCh    chan struct{}

	mu        sync.RWMutex
	closed    bool
	closeOnce sync.Once
	stopOnce  sync.Once
	workerWG  sync.WaitGroup
}

// NewAsyncRepository wraps a request-audit repository with async writes.
func NewAsyncRepository(repository Repository) (Repository, func()) {
	return NewAsyncRepositoryWithConfig(repository, AsyncConfig{
		QueueSize:    defaultAsyncQueueSize,
		WriteTimeout: defaultAsyncWriteTimeout,
	})
}

// NewAsyncRepositoryWithConfig wraps a request-audit repository with async writes.
func NewAsyncRepositoryWithConfig(repository Repository, cfg AsyncConfig) (Repository, func()) {
	if repository == nil {
		return NopRepository(), func() {}
	}

	queueSize := cfg.QueueSize
	if queueSize <= 0 {
		queueSize = defaultAsyncQueueSize
	}

	writeTimeout := cfg.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = defaultAsyncWriteTimeout
	}

	shutdownTimeout := cfg.ShutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = defaultAsyncShutdownTimeout
	}

	asyncRepo := &asyncRepository{
		repository:      repository,
		writeTimeout:    writeTimeout,
		shutdownTimeout: shutdownTimeout,
		metrics:         cfg.Metrics,
		queue:           make(chan asyncJob, queueSize),
		stopWorkerCh:    make(chan struct{}),
	}
	asyncRepo.observeQueueDepth(0)
	asyncRepo.startWorker()

	return asyncRepo, asyncRepo.Close
}

func (repo *asyncRepository) StoreRequestAudit(ctx context.Context, record Record) error {
	if repo == nil || repo.repository == nil {
		return nil
	}

	job := asyncJob{
		ctx:    context.WithoutCancel(contextOrBackground(ctx)),
		record: record,
	}

	repo.mu.RLock()
	if repo.closed {
		repo.mu.RUnlock()
		repo.observeRecordResult(record, requestAuditResultDroppedShutdown)
		return nil
	}

	select {
	case repo.queue <- job:
		repo.observeRecordResult(record, requestAuditResultEnqueued)
		repo.observeQueueDepth(len(repo.queue))
		repo.mu.RUnlock()
		return nil
	default:
		repo.mu.RUnlock()
		repo.observeRecordResult(record, requestAuditResultDroppedQueueFull)
		repo.observeQueueDepth(len(repo.queue))
		slog.WarnContext(
			contextOrBackground(ctx),
			"request audit queue full; dropping record",
			slog.String("method", record.Method),
			slog.String("path", record.Path),
			slog.Int("status", record.StatusCode),
		)
		return nil
	}
}

func (repo *asyncRepository) Close() {
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
				"request audit async shutdown timed out; forcing queue drop",
				slog.Duration("shutdown_timeout", repo.shutdownTimeout),
			)
			repo.signalStopWorker()
			<-waitDone
			repo.observeQueueDepth(0)
		}
	})
}

func (repo *asyncRepository) startWorker() {
	repo.workerWG.Add(1)
	go repo.runWorker()
}

func (repo *asyncRepository) runWorker() {
	defer repo.workerWG.Done()

	for {
		if repo.shouldStopWorker() {
			return
		}

		job, ok := <-repo.queue
		if !ok {
			repo.observeQueueDepth(0)
			return
		}
		repo.observeQueueDepth(len(repo.queue))

		startedAt := time.Now()
		if err := repo.writeRecord(job); err != nil {
			repo.observeRecordResult(job.record, requestAuditResultWriteError)
			repo.observeWriteDuration(job.record, time.Since(startedAt), requestAuditWriteResultFromError(err))
			slog.WarnContext(
				contextOrBackground(job.ctx),
				"request audit insert failed",
				slog.String("method", job.record.Method),
				slog.String("path", job.record.Path),
				slog.Int("status", job.record.StatusCode),
				slog.Any("error", err),
			)
			continue
		}

		repo.observeRecordResult(job.record, requestAuditResultStored)
		repo.observeWriteDuration(job.record, time.Since(startedAt), requestAuditWriteResultSuccess)
	}
}

func (repo *asyncRepository) shouldStopWorker() bool {
	select {
	case <-repo.stopWorkerCh:
		droppedCount := repo.dropQueuedJobs()
		if droppedCount > 0 {
			slog.Warn(
				"request audit shutdown dropped queued records",
				slog.Int("dropped_count", droppedCount),
			)
		}
		return true
	default:
		return false
	}
}

func (repo *asyncRepository) dropQueuedJobs() int {
	droppedCount := 0

	for {
		select {
		case job, ok := <-repo.queue:
			if !ok {
				repo.observeQueueDepth(0)
				return droppedCount
			}

			droppedCount++
			repo.observeRecordResult(job.record, requestAuditResultDroppedShutdown)
			repo.observeQueueDepth(len(repo.queue))
		default:
			repo.observeQueueDepth(len(repo.queue))
			return droppedCount
		}
	}
}

func (repo *asyncRepository) signalStopWorker() {
	repo.stopOnce.Do(func() {
		close(repo.stopWorkerCh)
	})
}

func (repo *asyncRepository) observeRecordResult(record Record, result string) {
	if repo == nil || repo.metrics == nil {
		return
	}

	repo.metrics.observeRecordResult(record.Server, result)
}

func (repo *asyncRepository) observeWriteDuration(record Record, duration time.Duration, result string) {
	if repo == nil || repo.metrics == nil {
		return
	}

	repo.metrics.observeWriteDuration(record.Server, duration, result)
}

func (repo *asyncRepository) observeQueueDepth(depth int) {
	if repo == nil || repo.metrics == nil {
		return
	}

	repo.metrics.observeQueueDepth(depth)
}

func requestAuditWriteResultFromError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return requestAuditWriteResultTimeout
	}

	return requestAuditWriteResultError
}

func (repo *asyncRepository) writeRecord(job asyncJob) error {
	writeCtx := contextOrBackground(job.ctx)
	cancel := func() {}
	if repo.writeTimeout > 0 {
		writeCtx, cancel = context.WithTimeout(writeCtx, repo.writeTimeout)
	}
	defer cancel()

	return repo.repository.StoreRequestAudit(writeCtx, job.record)
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}

	return ctx
}

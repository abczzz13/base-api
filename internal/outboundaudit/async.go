package outboundaudit

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

// AsyncConfig configures asynchronous outbound-audit writes.
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

// NewAsyncRepository wraps an outbound-audit repository with async writes.
func NewAsyncRepository(repository Repository) (Repository, func()) {
	return NewAsyncRepositoryWithConfig(repository, AsyncConfig{
		QueueSize:    defaultAsyncQueueSize,
		WriteTimeout: defaultAsyncWriteTimeout,
	})
}

// NewAsyncRepositoryWithConfig wraps an outbound-audit repository with async writes.
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

func (repo *asyncRepository) StoreOutboundAudit(ctx context.Context, record Record) error {
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
		repo.observeRecordResult(record, outboundAuditResultDroppedShutdown)
		return nil
	}

	select {
	case repo.queue <- job:
		repo.observeRecordResult(record, outboundAuditResultEnqueued)
		repo.observeQueueDepth(len(repo.queue))
		repo.mu.RUnlock()
		return nil
	default:
		repo.mu.RUnlock()
		repo.observeRecordResult(record, outboundAuditResultDroppedQueueFull)
		repo.observeQueueDepth(len(repo.queue))
		slog.WarnContext(
			contextOrBackground(ctx),
			"outbound audit queue full; dropping record",
			slog.String("client", record.Client),
			slog.String("operation", record.Operation),
			slog.String("method", record.Method),
			slog.String("host", record.Host),
			slog.String("path", record.Path),
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
				"outbound audit async shutdown timed out; forcing queue drop",
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
			repo.observeRecordResult(job.record, outboundAuditResultWriteError)
			repo.observeWriteDuration(job.record, time.Since(startedAt), writeResultFromError(err))
			slog.WarnContext(
				contextOrBackground(job.ctx),
				"outbound audit insert failed",
				slog.String("client", job.record.Client),
				slog.String("operation", job.record.Operation),
				slog.String("host", job.record.Host),
				slog.String("path", job.record.Path),
				slog.Any("error", err),
			)
			continue
		}

		repo.observeRecordResult(job.record, outboundAuditResultStored)
		repo.observeWriteDuration(job.record, time.Since(startedAt), outboundAuditWriteResultSuccess)
	}
}

func (repo *asyncRepository) shouldStopWorker() bool {
	select {
	case <-repo.stopWorkerCh:
		droppedCount := repo.dropQueuedJobs()
		if droppedCount > 0 {
			slog.Warn(
				"outbound audit shutdown dropped queued records",
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
			repo.observeRecordResult(job.record, outboundAuditResultDroppedShutdown)
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

	repo.metrics.observeRecordResult(record.Client, result)
}

func (repo *asyncRepository) observeWriteDuration(record Record, duration time.Duration, result string) {
	if repo == nil || repo.metrics == nil {
		return
	}

	repo.metrics.observeWriteDuration(record.Client, duration, result)
}

func (repo *asyncRepository) observeQueueDepth(depth int) {
	if repo == nil || repo.metrics == nil {
		return
	}

	repo.metrics.observeQueueDepth(depth)
}

func writeResultFromError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return outboundAuditWriteResultTimeout
	}

	return outboundAuditWriteResultError
}

func (repo *asyncRepository) writeRecord(job asyncJob) error {
	writeCtx := contextOrBackground(job.ctx)
	cancel := func() {}
	if repo.writeTimeout > 0 {
		writeCtx, cancel = context.WithTimeout(writeCtx, repo.writeTimeout)
	}
	defer cancel()

	return repo.repository.StoreOutboundAudit(writeCtx, job.record)
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}

	return ctx
}

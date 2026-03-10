package requestaudit

import (
	"context"
	"log/slog"
	"time"

	"github.com/abczzz13/base-api/internal/asyncaudit"
)

// AsyncConfig configures asynchronous request-audit writes.
type AsyncConfig struct {
	QueueSize       int
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
	Metrics         *Metrics
}

type asyncRepository struct {
	inner *asyncaudit.Repository[Record]
}

// NewAsyncRepository wraps a request-audit repository with async writes.
func NewAsyncRepository(repository Repository, cfg AsyncConfig) (Repository, func()) {
	if repository == nil {
		return NopRepository(), func() {}
	}

	params := asyncaudit.Params[Record]{
		Store:       repository.StoreRequestAudit,
		MetricLabel: func(r Record) string { return r.Server },
		LogAttrs: func(r Record) []slog.Attr {
			return []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.Path),
				slog.Int("status", r.StatusCode),
			}
		},
		EntityName: "request audit",
	}

	repo, close := asyncaudit.New[Record](params, asyncaudit.Config{
		QueueSize:       cfg.QueueSize,
		WriteTimeout:    cfg.WriteTimeout,
		ShutdownTimeout: cfg.ShutdownTimeout,
		Metrics:         cfg.Metrics,
	})

	return &asyncRepository{inner: repo}, close
}

func (repo *asyncRepository) StoreRequestAudit(ctx context.Context, record Record) error {
	return repo.inner.Store(ctx, record)
}

package outboundaudit

import (
	"context"
	"log/slog"
	"time"

	"github.com/abczzz13/base-api/internal/asyncaudit"
)

// AsyncConfig configures asynchronous outbound-audit writes.
type AsyncConfig struct {
	QueueSize       int
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
	Metrics         *Metrics
}

type asyncRepository struct {
	inner *asyncaudit.Repository[Record]
}

// NewAsyncRepository wraps an outbound-audit repository with async writes.
func NewAsyncRepository(repository Repository, cfg AsyncConfig) (Repository, func()) {
	if repository == nil {
		return NopRepository(), func() {}
	}

	params := asyncaudit.Params[Record]{
		Store:       repository.StoreOutboundAudit,
		MetricLabel: func(r Record) string { return r.Client },
		LogAttrs: func(r Record) []slog.Attr {
			return []slog.Attr{
				slog.String("client", r.Client),
				slog.String("operation", r.Operation),
				slog.String("method", r.Method),
				slog.String("host", r.Host),
				slog.String("path", r.Path),
			}
		},
		EntityName: "outbound audit",
	}

	repo, close := asyncaudit.New[Record](params, asyncaudit.Config{
		QueueSize:       cfg.QueueSize,
		WriteTimeout:    cfg.WriteTimeout,
		ShutdownTimeout: cfg.ShutdownTimeout,
		Metrics:         cfg.Metrics,
	})

	return &asyncRepository{inner: repo}, close
}

func (repo *asyncRepository) StoreOutboundAudit(ctx context.Context, record Record) error {
	return repo.inner.Store(ctx, record)
}

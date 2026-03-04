package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	dbmigrations "github.com/abczzz13/base-api/db/migrations"
)

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errors.New("database pool is required")
	}

	db := stdlib.OpenDBFromPool(pool)
	defer func() {
		if err := db.Close(); err != nil {
			slog.WarnContext(ctx, "close migration database handle", slog.Any("error", err))
		}
	}()

	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("configure migration lock: %w", err)
	}

	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		dbmigrations.FS,
		goose.WithSessionLocker(locker),
	)
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}

	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	if len(results) == 0 {
		slog.InfoContext(ctx, "database migrations complete", slog.Int("count", 0), slog.Bool("pending", false))
		return nil
	}

	for _, result := range results {
		if result == nil || result.Source == nil {
			continue
		}

		source := result.Source.Path
		if source != "" {
			source = filepath.Base(source)
		}

		slog.InfoContext(
			ctx,
			"database migration applied",
			slog.Int64("version", result.Source.Version),
			slog.String("source", source),
			slog.String("direction", result.Direction),
			slog.Duration("duration", result.Duration),
			slog.Bool("empty", result.Empty),
		)
	}

	slog.InfoContext(ctx, "database migrations complete", slog.Int("count", len(results)))

	return nil
}

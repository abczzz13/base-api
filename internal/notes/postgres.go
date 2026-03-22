package notes

import (
	"context"
	"errors"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/abczzz13/base-api/internal/dbsqlc"
)

type postgresRepository struct {
	queries *dbsqlc.Queries
}

// NewPostgresRepository creates a sqlc-backed notes repository.
func NewPostgresRepository(database dbsqlc.DBTX) (*postgresRepository, error) {
	if database == nil {
		return nil, errors.New("database dependency is required")
	}

	return &postgresRepository{queries: dbsqlc.New(database)}, nil
}

func (repo *postgresRepository) CreateNote(ctx context.Context, params CreateParams) (CreateResult, error) {
	row, err := repo.queries.InsertNote(ctx, dbsqlc.InsertNoteParams{
		Title:               params.Title,
		Body:                params.Body,
		LocationQuery:       params.LocationQuery,
		ResolvedLocation:    params.ResolvedLocation,
		WeatherProvider:     params.Weather.Provider,
		WeatherCondition:    params.Weather.Condition,
		WeatherTemperatureC: params.Weather.TemperatureC,
		WeatherObservedAt:   pgtype.Timestamptz{Time: params.Weather.ObservedAt, Valid: true},
	})
	if err != nil {
		return CreateResult{}, err
	}

	return CreateResult{
		ID:        row.ID,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

func (repo *postgresRepository) GetNote(ctx context.Context, id uuid.UUID) (Note, error) {
	row, err := repo.queries.GetNote(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Note{}, ErrNoteNotFound
		}
		return Note{}, err
	}

	return mapNote(row), nil
}

func (repo *postgresRepository) ListNotesPage(ctx context.Context, params ListPageParams) ([]Note, error) {
	queryParams := dbsqlc.ListNotesPageParams{
		CursorCreatedAt: pgtype.Timestamptz{},
		CursorID:        uuid.Nil,
		LimitCount:      clampPositiveInt32(params.Limit),
	}
	if params.Cursor != nil {
		queryParams.CursorCreatedAt = pgtype.Timestamptz{Time: params.Cursor.CreatedAt, Valid: true}
		queryParams.CursorID = params.Cursor.ID
	}

	rows, err := repo.queries.ListNotesPage(ctx, queryParams)
	if err != nil {
		return nil, err
	}

	notes := make([]Note, 0, len(rows))
	for _, row := range rows {
		notes = append(notes, mapNote(row))
	}

	return notes, nil
}

func (repo *postgresRepository) UpdateNote(ctx context.Context, params UpdateParams) (UpdateResult, error) {
	row, err := repo.queries.UpdateNote(ctx, dbsqlc.UpdateNoteParams{
		ID:                  params.ID,
		Title:               params.Title,
		Body:                params.Body,
		LocationQuery:       params.LocationQuery,
		ResolvedLocation:    params.ResolvedLocation,
		WeatherProvider:     params.Weather.Provider,
		WeatherCondition:    params.Weather.Condition,
		WeatherTemperatureC: params.Weather.TemperatureC,
		WeatherObservedAt:   pgtype.Timestamptz{Time: params.Weather.ObservedAt, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UpdateResult{}, ErrNoteNotFound
		}
		return UpdateResult{}, err
	}

	return UpdateResult{UpdatedAt: row.Time}, nil
}

func (repo *postgresRepository) DeleteNote(ctx context.Context, id uuid.UUID) error {
	deleted, err := repo.queries.DeleteNote(ctx, id)
	if err != nil {
		return err
	}
	if deleted == 0 {
		return ErrNoteNotFound
	}

	return nil
}

func mapNote(row dbsqlc.Note) Note {
	return Note{
		ID:               row.ID,
		Title:            row.Title,
		Body:             row.Body,
		LocationQuery:    row.LocationQuery,
		ResolvedLocation: row.ResolvedLocation,
		Weather: WeatherSnapshot{
			Provider:     row.WeatherProvider,
			Condition:    row.WeatherCondition,
			TemperatureC: row.WeatherTemperatureC,
			ObservedAt:   row.WeatherObservedAt.Time,
		},
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}
}

func clampPositiveInt32(value int) int32 {
	value = max(value, 1)
	if value > math.MaxInt32 {
		return math.MaxInt32
	}

	return int32(value)
}

package notes

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jackc/pgx/v5/pgxpool"

	internalpostgres "github.com/abczzz13/base-api/internal/postgres"
)

func TestPostgresRepositoryRoundTripAndPagination(t *testing.T) {
	dbURL := strings.TrimSpace(os.Getenv("TEST_DB_URL"))
	if dbURL == "" {
		t.Fatal("set TEST_DB_URL to run database-backed tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("PostgreSQL integration unavailable: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := internalpostgres.Migrate(ctx, pool); err != nil {
		t.Fatalf("PostgreSQL integration unavailable: %v", err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE TABLE notes"); err != nil {
		t.Fatalf("truncate notes: %v", err)
	}

	repo, err := NewPostgresRepository(pool)
	if err != nil {
		t.Fatalf("NewPostgresRepository returned error: %v", err)
	}
	created1, err := repo.CreateNote(ctx, CreateParams{
		Title:            "First",
		Body:             "Body 1",
		LocationQuery:    "Amsterdam",
		ResolvedLocation: "Amsterdam, Netherlands",
		Weather: WeatherSnapshot{
			Provider:     "open-meteo",
			Condition:    "Cloudy",
			TemperatureC: 10,
			ObservedAt:   time.Date(2026, time.March, 21, 8, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("CreateNote first returned error: %v", err)
	}
	created2, err := repo.CreateNote(ctx, CreateParams{
		Title:            "Second",
		Body:             "Body 2",
		LocationQuery:    "Rotterdam",
		ResolvedLocation: "Rotterdam, Netherlands",
		Weather: WeatherSnapshot{
			Provider:     "open-meteo",
			Condition:    "Rain",
			TemperatureC: 7,
			ObservedAt:   time.Date(2026, time.March, 21, 9, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("CreateNote second returned error: %v", err)
	}
	created3, err := repo.CreateNote(ctx, CreateParams{
		Title:            "Third",
		Body:             "Body 3",
		LocationQuery:    "Utrecht",
		ResolvedLocation: "Utrecht, Netherlands",
		Weather: WeatherSnapshot{
			Provider:     "open-meteo",
			Condition:    "Sunny",
			TemperatureC: 15,
			ObservedAt:   time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("CreateNote third returned error: %v", err)
	}

	setCreatedAt := func(id any, value time.Time) {
		t.Helper()
		if _, err := pool.Exec(ctx, "UPDATE notes SET created_at = $2, updated_at = $2 WHERE id = $1", id, value); err != nil {
			t.Fatalf("update timestamps for note %v: %v", id, err)
		}
	}
	setCreatedAt(created1.ID, time.Date(2026, time.March, 21, 8, 0, 0, 0, time.UTC))
	tiedCreatedAt := time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC)
	setCreatedAt(created2.ID, tiedCreatedAt)
	setCreatedAt(created3.ID, tiedCreatedAt)

	got, err := repo.GetNote(ctx, created2.ID)
	if err != nil {
		t.Fatalf("GetNote returned error: %v", err)
	}
	if diff := cmp.Diff(created2.ID, got.ID); diff != "" {
		t.Fatalf("GetNote ID mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff("Second", got.Title); diff != "" {
		t.Fatalf("GetNote title mismatch (-want +got):\n%s", diff)
	}

	updateResult, err := repo.UpdateNote(ctx, UpdateParams{
		ID:               created2.ID,
		Title:            "Second updated",
		Body:             "Body 2 updated",
		LocationQuery:    got.LocationQuery,
		ResolvedLocation: got.ResolvedLocation,
		Weather:          got.Weather,
	})
	if err != nil {
		t.Fatalf("UpdateNote returned error: %v", err)
	}
	if !updateResult.UpdatedAt.After(got.UpdatedAt) {
		t.Fatalf("UpdateNote updated_at was not advanced: before=%s after=%s", got.UpdatedAt, updateResult.UpdatedAt)
	}

	page, err := repo.ListNotesPage(ctx, ListPageParams{Limit: 2})
	if err != nil {
		t.Fatalf("ListNotesPage returned error: %v", err)
	}
	firstExpected, secondExpected := created2.ID.String(), created3.ID.String()
	if firstExpected < secondExpected {
		firstExpected, secondExpected = secondExpected, firstExpected
	}
	gotIDs := []string{page[0].ID.String(), page[1].ID.String()}
	if diff := cmp.Diff([]string{firstExpected, secondExpected}, gotIDs); diff != "" {
		t.Fatalf("ListNotesPage first page IDs mismatch (-want +got):\n%s", diff)
	}

	page, err = repo.ListNotesPage(ctx, ListPageParams{
		Limit:  1,
		Cursor: &Cursor{CreatedAt: page[1].CreatedAt, ID: page[1].ID},
	})
	if err != nil {
		t.Fatalf("ListNotesPage second page returned error: %v", err)
	}
	gotIDs = []string{page[0].ID.String()}
	if diff := cmp.Diff([]string{created1.ID.String()}, gotIDs); diff != "" {
		t.Fatalf("ListNotesPage second page IDs mismatch (-want +got):\n%s", diff)
	}

	if err := repo.DeleteNote(ctx, created1.ID); err != nil {
		t.Fatalf("DeleteNote returned error: %v", err)
	}
	_, err = repo.GetNote(ctx, created1.ID)
	if diff := cmp.Diff(ErrNoteNotFound, err, cmpopts.EquateErrors()); diff != "" {
		t.Fatalf("GetNote after delete mismatch (-want +got):\n%s", diff)
	}
}

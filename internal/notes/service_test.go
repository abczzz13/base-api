package notes

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/abczzz13/base-api/internal/apierrors"
	"github.com/abczzz13/base-api/internal/clients/weather"
)

func TestNewServiceValidation(t *testing.T) {
	tests := []struct {
		name          string
		repository    Repository
		weatherClient weather.Client
		wantErr       string
	}{
		{
			name:          "nil repository",
			weatherClient: weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) { return weather.CurrentWeather{}, nil }),
			wantErr:       "notes repository is required",
		},
		{
			name:       "nil weather client",
			repository: RepositoryFuncs{},
			wantErr:    "weather client is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewService(tt.repository, tt.weatherClient)
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("NewService error mismatch: want %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestServiceCreateNote(t *testing.T) {
	observedAt := time.Date(2026, time.March, 21, 13, 0, 0, 0, time.UTC)
	noteID := uuid.MustParse("0195b1d3-e65d-779b-a3e9-6f3a25ed42b1")
	service, err := NewService(RepositoryFuncs{
		CreateNoteFunc: func(_ context.Context, params CreateParams) (CreateResult, error) {
			want := CreateParams{
				Title:            "Pack lunch",
				Body:             "Remember an umbrella too",
				LocationQuery:    "Amsterdam",
				ResolvedLocation: "Amsterdam, Netherlands",
				Weather: WeatherSnapshot{
					Provider:     "open-meteo",
					Condition:    "Rain",
					TemperatureC: 8.5,
					ObservedAt:   observedAt,
				},
			}
			if diff := cmp.Diff(want, params); diff != "" {
				return CreateResult{}, errors.New(diff)
			}

			return CreateResult{ID: noteID, CreatedAt: observedAt, UpdatedAt: observedAt}, nil
		},
	}, weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
		return weather.CurrentWeather{
			Provider:     "open-meteo",
			Location:     "Amsterdam, Netherlands",
			Condition:    "Rain",
			TemperatureC: 8.5,
			ObservedAt:   observedAt,
		}, nil
	}))
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	got, err := service.CreateNote(context.Background(), CreateInput{
		Title:    "Pack lunch",
		Body:     "Remember an umbrella too",
		Location: "Amsterdam",
	})
	if err != nil {
		t.Fatalf("CreateNote returned error: %v", err)
	}

	if diff := cmp.Diff(noteID, got.ID); diff != "" {
		t.Fatalf("CreateNote ID mismatch (-want +got):\n%s", diff)
	}
}

func TestServiceCreateNoteWeatherErrors(t *testing.T) {
	tests := []struct {
		name    string
		client  weather.Client
		wantErr apierrors.Error
	}{
		{
			name: "location not found becomes unprocessable entity",
			client: weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
				return weather.CurrentWeather{}, &weather.NotFoundError{Location: "Atlantis"}
			}),
			wantErr: apierrors.New(http.StatusUnprocessableEntity, "note_location_not_found", "note location could not be resolved"),
		},
		{
			name: "timeout becomes gateway timeout",
			client: weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
				return weather.CurrentWeather{}, context.DeadlineExceeded
			}),
			wantErr: apierrors.New(http.StatusGatewayTimeout, "weather_timeout", "weather provider request timed out"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewService(RepositoryFuncs{
				CreateNoteFunc: func(context.Context, CreateParams) (CreateResult, error) {
					return CreateResult{}, errors.New("unexpected repository call")
				},
			}, tt.client)
			if err != nil {
				t.Fatalf("NewService returned error: %v", err)
			}

			_, err = service.CreateNote(context.Background(), CreateInput{
				Title:    "Pack lunch",
				Body:     "Remember an umbrella too",
				Location: "Atlantis",
			})
			if diff := cmp.Diff(tt.wantErr, err); diff != "" {
				t.Fatalf("CreateNote error mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestServiceCreateNoteAcceptsMaxRuneLengthTitle(t *testing.T) {
	title := strings.Repeat("界", maxTitleLength)
	observedAt := time.Date(2026, time.March, 21, 13, 0, 0, 0, time.UTC)

	service, err := NewService(RepositoryFuncs{
		CreateNoteFunc: func(_ context.Context, params CreateParams) (CreateResult, error) {
			if diff := cmp.Diff(title, params.Title); diff != "" {
				return CreateResult{}, errors.New(diff)
			}

			return CreateResult{
				ID:        uuid.MustParse("0195b1d3-e65d-779b-a3e9-6f3a25ed42b7"),
				CreatedAt: observedAt,
				UpdatedAt: observedAt,
			}, nil
		},
	}, weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
		return weather.CurrentWeather{
			Provider:     "open-meteo",
			Location:     "Amsterdam, Netherlands",
			Condition:    "Rain",
			TemperatureC: 8.5,
			ObservedAt:   observedAt,
		}, nil
	}))
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	if _, err := service.CreateNote(context.Background(), CreateInput{
		Title:    title,
		Body:     "Remember an umbrella too",
		Location: "Amsterdam",
	}); err != nil {
		t.Fatalf("CreateNote returned error: %v", err)
	}
}

func TestServiceUpdateNoteWithoutLocationChangeSkipsWeatherLookup(t *testing.T) {
	var weatherCalls atomic.Int32
	noteID := uuid.MustParse("0195b1d3-e65d-779b-a3e9-6f3a25ed42b2")
	current := Note{
		ID:               noteID,
		Title:            "Old title",
		Body:             "Old body",
		LocationQuery:    "Amsterdam",
		ResolvedLocation: "Amsterdam, Netherlands",
		Weather: WeatherSnapshot{
			Provider:     "open-meteo",
			Condition:    "Cloudy",
			TemperatureC: 10,
			ObservedAt:   time.Date(2026, time.March, 21, 9, 0, 0, 0, time.UTC),
		},
		CreatedAt: time.Date(2026, time.March, 20, 9, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, time.March, 20, 9, 0, 0, 0, time.UTC),
	}

	service, err := NewService(RepositoryFuncs{
		GetNoteFunc: func(context.Context, uuid.UUID) (Note, error) {
			return current, nil
		},
		UpdateNoteFunc: func(_ context.Context, params UpdateParams) (UpdateResult, error) {
			if diff := cmp.Diff(current.Weather, params.Weather); diff != "" {
				return UpdateResult{}, errors.New(diff)
			}
			return UpdateResult{UpdatedAt: current.UpdatedAt.Add(time.Hour)}, nil
		},
	}, weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
		weatherCalls.Add(1)
		return weather.CurrentWeather{}, nil
	}))
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	title := "New title"
	got, err := service.UpdateNote(context.Background(), current.ID, UpdateInput{Title: &title})
	if err != nil {
		t.Fatalf("UpdateNote returned error: %v", err)
	}
	if got.Title != "New title" {
		t.Fatalf("UpdateNote title mismatch: want %q, got %q", "New title", got.Title)
	}
	if weatherCalls.Load() != 0 {
		t.Fatalf("weather client call count mismatch: want %d, got %d", 0, weatherCalls.Load())
	}
}

func TestServiceListNotesBuildsNextCursor(t *testing.T) {
	page := []Note{
		{ID: uuid.MustParse("0195b1d3-e65d-779b-a3e9-6f3a25ed42b3"), CreatedAt: time.Date(2026, time.March, 21, 12, 0, 0, 0, time.UTC)},
		{ID: uuid.MustParse("0195b1d3-e65d-779b-a3e9-6f3a25ed42b4"), CreatedAt: time.Date(2026, time.March, 21, 11, 0, 0, 0, time.UTC)},
		{ID: uuid.MustParse("0195b1d3-e65d-779b-a3e9-6f3a25ed42b5"), CreatedAt: time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC)},
	}
	service, err := NewService(RepositoryFuncs{
		ListNotesPageFunc: func(_ context.Context, params ListPageParams) ([]Note, error) {
			if diff := cmp.Diff(ListPageParams{Limit: 3}, params); diff != "" {
				return nil, errors.New(diff)
			}
			return page, nil
		},
	}, weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
		return weather.CurrentWeather{}, nil
	}))
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	result, err := service.ListNotes(context.Background(), ListInput{Limit: 2})
	if err != nil {
		t.Fatalf("ListNotes returned error: %v", err)
	}
	if diff := cmp.Diff(page[:2], result.Items); diff != "" {
		t.Fatalf("ListNotes items mismatch (-want +got):\n%s", diff)
	}

	decoded, err := decodeCursor(result.NextCursor)
	if err != nil {
		t.Fatalf("decodeCursor returned error: %v", err)
	}
	if diff := cmp.Diff(Cursor{CreatedAt: page[1].CreatedAt, ID: page[1].ID}, decoded); diff != "" {
		t.Fatalf("next cursor mismatch (-want +got):\n%s", diff)
	}
}

func TestServiceListNotesRejectsWhitespaceCursor(t *testing.T) {
	service, err := NewService(RepositoryFuncs{
		ListNotesPageFunc: func(context.Context, ListPageParams) ([]Note, error) {
			return nil, errors.New("unexpected repository call")
		},
	}, weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
		return weather.CurrentWeather{}, nil
	}))
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	_, err = service.ListNotes(context.Background(), ListInput{Cursor: "   "})
	if diff := cmp.Diff(apierrors.New(http.StatusBadRequest, "invalid_cursor", "cursor is invalid"), err); diff != "" {
		t.Fatalf("ListNotes error mismatch (-want +got):\n%s", diff)
	}
}

func TestServiceDeleteNoteMissingMapsNotFound(t *testing.T) {
	service, err := NewService(RepositoryFuncs{
		DeleteNoteFunc: func(context.Context, uuid.UUID) error {
			return ErrNoteNotFound
		},
	}, weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
		return weather.CurrentWeather{}, nil
	}))
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	err = service.DeleteNote(context.Background(), uuid.MustParse("0195b1d3-e65d-779b-a3e9-6f3a25ed42b6"))
	if diff := cmp.Diff(apierrors.New(http.StatusNotFound, "note_not_found", "note not found"), err); diff != "" {
		t.Fatalf("DeleteNote error mismatch (-want +got):\n%s", diff)
	}
}

package publicapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/abczzz13/base-api/internal/config"
	"github.com/abczzz13/base-api/internal/notes"
)

func TestNewPublicHandlerCreateNoteIncludesRequestID(t *testing.T) {
	handler, err := newPublicHandlerForTestWithDependencies(t, config.Config{Environment: "test"}, Dependencies{
		NotesRepository: notes.RepositoryFuncs{
			CreateNoteFunc: func(_ context.Context, params notes.CreateParams) (notes.CreateResult, error) {
				observedAt := time.Date(2026, time.March, 21, 13, 45, 0, 0, time.UTC)
				_ = params
				return notes.CreateResult{
					ID:        uuid.MustParse("0195b1d3-e65d-779b-a3e9-6f3a25ed42d2"),
					CreatedAt: observedAt,
					UpdatedAt: observedAt,
				}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("newPublicHandlerForTestWithDependencies returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(`{"title":"Bring coat","body":"Expect rain after work","location":"Amsterdam"}`))
	req.Header.Set("Content-Type", "application/json")

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusCreated, rr.Code)
	}
	if got := rr.Header().Get("X-Request-Id"); got == "" {
		t.Fatal("expected non-empty X-Request-Id response header")
	}
	if body := rr.Body.String(); !strings.Contains(body, `"id":"0195b1d3-e65d-779b-a3e9-6f3a25ed42d2"`) || !strings.Contains(body, `"locationQuery":"Amsterdam"`) {
		t.Fatalf("body mismatch: got %q", body)
	}
}

func TestNewPublicHandlerListNotesInvalidCursor(t *testing.T) {
	handler, err := newPublicHandlerForTest(t, config.Config{Environment: "test"})
	if err != nil {
		t.Fatalf("newPublicHandlerForTest returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/notes?cursor=not-a-valid-cursor", nil)

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusBadRequest, rr.Code)
	}
	if got := rr.Header().Get("X-Request-Id"); got == "" {
		t.Fatal("expected non-empty X-Request-Id response header")
	}
	if body := rr.Body.String(); !strings.Contains(body, `"code":"invalid_cursor"`) || !strings.Contains(body, `"message":"cursor is invalid"`) {
		t.Fatalf("body mismatch: got %q", body)
	}
}

func TestNewPublicHandlerGetNote(t *testing.T) {
	noteID := uuid.MustParse("0195b1d3-e65d-779b-a3e9-6f3a25ed42d3")
	observedAt := time.Date(2026, time.March, 21, 13, 45, 0, 0, time.UTC)
	handler, err := newPublicHandlerForTestWithDependencies(t, config.Config{Environment: "test"}, Dependencies{
		NotesRepository: notes.RepositoryFuncs{
			GetNoteFunc: func(context.Context, uuid.UUID) (notes.Note, error) {
				return notes.Note{
					ID:               noteID,
					Title:            "Bring coat",
					Body:             "Expect rain after work",
					LocationQuery:    "Amsterdam",
					ResolvedLocation: "Amsterdam, Netherlands",
					Weather: notes.WeatherSnapshot{
						Provider:     "open-meteo",
						Condition:    "Rain",
						TemperatureC: 7.5,
						ObservedAt:   observedAt,
					},
					CreatedAt: observedAt,
					UpdatedAt: observedAt,
				}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("newPublicHandlerForTestWithDependencies returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/notes/"+noteID.String(), nil)

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusOK, rr.Code)
	}
	if got := rr.Header().Get("X-Request-Id"); got == "" {
		t.Fatal("expected non-empty X-Request-Id response header")
	}
	if body := rr.Body.String(); !strings.Contains(body, `"id":"`+noteID.String()+`"`) || !strings.Contains(body, `"title":"Bring coat"`) {
		t.Fatalf("body mismatch: got %q", body)
	}
}

func TestNewPublicHandlerListNotesIncludesNextCursor(t *testing.T) {
	baseTime := time.Date(2026, time.March, 21, 13, 45, 0, 0, time.UTC)
	handler, err := newPublicHandlerForTestWithDependencies(t, config.Config{Environment: "test"}, Dependencies{
		NotesRepository: notes.RepositoryFuncs{
			ListNotesPageFunc: func(_ context.Context, params notes.ListPageParams) ([]notes.Note, error) {
				if got, want := params.Limit, 3; got != want {
					return nil, errors.New("unexpected limit")
				}

				return []notes.Note{
					{
						ID:               uuid.MustParse("0195b1d3-e65d-779b-a3e9-6f3a25ed42d4"),
						Title:            "First",
						Body:             "One",
						LocationQuery:    "Amsterdam",
						ResolvedLocation: "Amsterdam, Netherlands",
						Weather:          notes.WeatherSnapshot{Provider: "open-meteo", Condition: "Rain", TemperatureC: 7.5, ObservedAt: baseTime},
						CreatedAt:        baseTime,
						UpdatedAt:        baseTime,
					},
					{
						ID:               uuid.MustParse("0195b1d3-e65d-779b-a3e9-6f3a25ed42d5"),
						Title:            "Second",
						Body:             "Two",
						LocationQuery:    "Rotterdam",
						ResolvedLocation: "Rotterdam, Netherlands",
						Weather:          notes.WeatherSnapshot{Provider: "open-meteo", Condition: "Cloudy", TemperatureC: 8.5, ObservedAt: baseTime.Add(-time.Minute)},
						CreatedAt:        baseTime.Add(-time.Minute),
						UpdatedAt:        baseTime.Add(-time.Minute),
					},
					{
						ID:               uuid.MustParse("0195b1d3-e65d-779b-a3e9-6f3a25ed42d6"),
						Title:            "Third",
						Body:             "Three",
						LocationQuery:    "Utrecht",
						ResolvedLocation: "Utrecht, Netherlands",
						Weather:          notes.WeatherSnapshot{Provider: "open-meteo", Condition: "Sunny", TemperatureC: 9.5, ObservedAt: baseTime.Add(-2 * time.Minute)},
						CreatedAt:        baseTime.Add(-2 * time.Minute),
						UpdatedAt:        baseTime.Add(-2 * time.Minute),
					},
				}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("newPublicHandlerForTestWithDependencies returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/notes?limit=2", nil)

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusOK, rr.Code)
	}
	if got := rr.Header().Get("X-Request-Id"); got == "" {
		t.Fatal("expected non-empty X-Request-Id response header")
	}
	if body := rr.Body.String(); !strings.Contains(body, `"title":"First"`) || !strings.Contains(body, `"title":"Second"`) || !strings.Contains(body, `"nextCursor":"`) {
		t.Fatalf("body mismatch: got %q", body)
	}
}

func TestNewPublicHandlerUpdateNote(t *testing.T) {
	noteID := uuid.MustParse("0195b1d3-e65d-779b-a3e9-6f3a25ed42d7")
	createdAt := time.Date(2026, time.March, 21, 13, 45, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	handler, err := newPublicHandlerForTestWithDependencies(t, config.Config{Environment: "test"}, Dependencies{
		NotesRepository: notes.RepositoryFuncs{
			GetNoteFunc: func(context.Context, uuid.UUID) (notes.Note, error) {
				return notes.Note{
					ID:               noteID,
					Title:            "Bring coat",
					Body:             "Expect rain after work",
					LocationQuery:    "Amsterdam",
					ResolvedLocation: "Amsterdam, Netherlands",
					Weather: notes.WeatherSnapshot{
						Provider:     "open-meteo",
						Condition:    "Rain",
						TemperatureC: 7.5,
						ObservedAt:   createdAt,
					},
					CreatedAt: createdAt,
					UpdatedAt: createdAt,
				}, nil
			},
			UpdateNoteFunc: func(_ context.Context, params notes.UpdateParams) (notes.UpdateResult, error) {
				if got, want := params.Title, "Bring umbrella"; got != want {
					return notes.UpdateResult{}, errors.New("unexpected title")
				}

				return notes.UpdateResult{UpdatedAt: updatedAt}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("newPublicHandlerForTestWithDependencies returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/notes/"+noteID.String(), bytes.NewBufferString(`{"title":"Bring umbrella"}`))
	req.Header.Set("Content-Type", "application/json")

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusOK, rr.Code)
	}
	if got := rr.Header().Get("X-Request-Id"); got == "" {
		t.Fatal("expected non-empty X-Request-Id response header")
	}
	if body := rr.Body.String(); !strings.Contains(body, `"title":"Bring umbrella"`) || !strings.Contains(body, `"updatedAt":"2026-03-21T14:45:00Z"`) {
		t.Fatalf("body mismatch: got %q", body)
	}
}

func TestNewPublicHandlerDeleteNote(t *testing.T) {
	noteID := uuid.MustParse("0195b1d3-e65d-779b-a3e9-6f3a25ed42d8")
	handler, err := newPublicHandlerForTestWithDependencies(t, config.Config{Environment: "test"}, Dependencies{
		NotesRepository: notes.RepositoryFuncs{
			DeleteNoteFunc: func(context.Context, uuid.UUID) error {
				return nil
			},
		},
	})
	if err != nil {
		t.Fatalf("newPublicHandlerForTestWithDependencies returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/notes/"+noteID.String(), nil)

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusNoContent, rr.Code)
	}
	if got := rr.Header().Get("X-Request-Id"); got == "" {
		t.Fatal("expected non-empty X-Request-Id response header")
	}
	if body := rr.Body.String(); body != "" {
		t.Fatalf("expected empty body, got %q", body)
	}
}

func TestNewPublicHandlerCreateNoteRejectsWhitespaceTitle(t *testing.T) {
	handler, err := newPublicHandlerForTestWithDependencies(t, config.Config{Environment: "test"}, Dependencies{
		NotesRepository: notes.RepositoryFuncs{
			CreateNoteFunc: func(context.Context, notes.CreateParams) (notes.CreateResult, error) {
				return notes.CreateResult{}, errors.New("unexpected repository call")
			},
		},
	})
	if err != nil {
		t.Fatalf("newPublicHandlerForTestWithDependencies returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(`{"title":"   ","body":"Expect rain after work","location":"Amsterdam"}`))
	req.Header.Set("Content-Type", "application/json")

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusBadRequest, rr.Code)
	}
	if got := rr.Header().Get("X-Request-Id"); got == "" {
		t.Fatal("expected non-empty X-Request-Id response header")
	}
	if body := rr.Body.String(); !strings.Contains(body, `"code":"bad_request"`) || !strings.Contains(body, `"message":"bad request"`) {
		t.Fatalf("body mismatch: got %q", body)
	}
}

func TestNewPublicHandlerUpdateNoteRejectsUnknownField(t *testing.T) {
	handler, err := newPublicHandlerForTestWithDependencies(t, config.Config{Environment: "test"}, Dependencies{
		NotesRepository: notes.RepositoryFuncs{
			GetNoteFunc: func(context.Context, uuid.UUID) (notes.Note, error) {
				return notes.Note{}, errors.New("unexpected repository call")
			},
			UpdateNoteFunc: func(context.Context, notes.UpdateParams) (notes.UpdateResult, error) {
				return notes.UpdateResult{}, errors.New("unexpected repository call")
			},
		},
	})
	if err != nil {
		t.Fatalf("newPublicHandlerForTestWithDependencies returned error: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/notes/0195b1d3-e65d-779b-a3e9-6f3a25ed42d9", bytes.NewBufferString(`{"foo":"bar"}`))
	req.Header.Set("Content-Type", "application/json")

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status mismatch: want %d, got %d", http.StatusBadRequest, rr.Code)
	}
	if got := rr.Header().Get("X-Request-Id"); got == "" {
		t.Fatal("expected non-empty X-Request-Id response header")
	}
	if body := rr.Body.String(); !strings.Contains(body, `"code":"bad_request"`) || !strings.Contains(body, `"message":"bad request"`) {
		t.Fatalf("body mismatch: got %q", body)
	}
}

package publicapi_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/abczzz13/base-api/internal/clients/weather"
	"github.com/abczzz13/base-api/internal/notes"
	"github.com/abczzz13/base-api/internal/publicapi"
	"github.com/abczzz13/base-api/internal/publicoas"
	"github.com/abczzz13/base-api/internal/requestid"
)

func TestOASHandlerCreateNoteIncludesRequestIDHeader(t *testing.T) {
	observedAt := time.Date(2026, time.March, 21, 13, 30, 0, 0, time.UTC)
	noteID := uuid.MustParse("0195b1d3-e65d-779b-a3e9-6f3a25ed42d1")
	notesService, err := notes.NewService(notes.RepositoryFuncs{
		CreateNoteFunc: func(context.Context, notes.CreateParams) (notes.CreateResult, error) {
			return notes.CreateResult{ID: noteID, CreatedAt: observedAt, UpdatedAt: observedAt}, nil
		},
	}, weather.ClientFunc(func(context.Context, string) (weather.CurrentWeather, error) {
		return weather.CurrentWeather{
			Provider:     "open-meteo",
			Location:     "Amsterdam, Netherlands",
			Condition:    "Rain",
			TemperatureC: 7.5,
			ObservedAt:   observedAt,
		}, nil
	}))
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	handler, err := publicapi.NewOASHandler(publicapi.NewService(), notesService)
	if err != nil {
		t.Fatalf("NewOASHandler returned error: %v", err)
	}

	ctx := requestid.WithContext(context.Background(), "req-456")
	got, err := handler.CreateNote(ctx, &publicoas.CreateNoteRequest{
		Title:    "Bring coat",
		Body:     "Expect rain after work",
		Location: "Amsterdam",
	})
	if err != nil {
		t.Fatalf("CreateNote returned error: %v", err)
	}

	gotResp, ok := got.(*publicoas.NoteHeaders)
	if !ok {
		t.Fatalf("CreateNote response type mismatch: got %T", got)
	}

	want := &publicoas.NoteHeaders{
		XRequestID: publicoas.NewOptString("req-456"),
		Response: publicoas.Note{
			ID:               noteID,
			Title:            "Bring coat",
			Body:             "Expect rain after work",
			LocationQuery:    "Amsterdam",
			ResolvedLocation: "Amsterdam, Netherlands",
			Weather: publicoas.NoteWeatherSnapshot{
				Provider:     "open-meteo",
				Condition:    "Rain",
				TemperatureC: 7.5,
				ObservedAt:   observedAt,
			},
			CreatedAt: observedAt,
			UpdatedAt: observedAt,
		},
	}
	if diff := cmp.Diff(want, gotResp); diff != "" {
		t.Fatalf("CreateNote mismatch (-want +got):\n%s", diff)
	}
}

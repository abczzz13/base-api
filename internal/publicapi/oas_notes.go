package publicapi

import (
	"context"

	"github.com/abczzz13/base-api/internal/notes"
	"github.com/abczzz13/base-api/internal/publicoas"
)

func (h *oasHandler) CreateNote(ctx context.Context, req *publicoas.CreateNoteRequest) (publicoas.CreateNoteRes, error) {
	created, err := h.notesService.CreateNote(ctx, notes.CreateInput{
		Title:    req.Title,
		Body:     req.Body,
		Location: req.Location,
	})
	if err != nil {
		return nil, publicDefaultError(ctx, err)
	}

	wrapped := &publicoas.NoteHeaders{Response: publicNote(created)}
	setPublicRequestIDHeader(wrapped, ctx)

	return wrapped, nil
}

func (h *oasHandler) DeleteNote(ctx context.Context, params publicoas.DeleteNoteParams) (publicoas.DeleteNoteRes, error) {
	if err := h.notesService.DeleteNote(ctx, params.NoteId); err != nil {
		return nil, publicDefaultError(ctx, err)
	}

	wrapped := &publicoas.DeleteNoteNoContent{}
	setPublicRequestIDHeader(wrapped, ctx)

	return wrapped, nil
}

func (h *oasHandler) GetNote(ctx context.Context, params publicoas.GetNoteParams) (publicoas.GetNoteRes, error) {
	note, err := h.notesService.GetNote(ctx, params.NoteId)
	if err != nil {
		return nil, publicDefaultError(ctx, err)
	}

	wrapped := &publicoas.NoteHeaders{Response: publicNote(note)}
	setPublicRequestIDHeader(wrapped, ctx)

	return wrapped, nil
}

func (h *oasHandler) ListNotes(ctx context.Context, params publicoas.ListNotesParams) (publicoas.ListNotesRes, error) {
	result, err := h.notesService.ListNotes(ctx, notes.ListInput{
		Limit:  int(params.Limit.Or(notes.DefaultListLimit)),
		Cursor: params.Cursor.Or(""),
	})
	if err != nil {
		return nil, publicDefaultError(ctx, err)
	}

	items := make([]publicoas.Note, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, publicNote(item))
	}

	wrapped := &publicoas.ListNotesResponseHeaders{
		Response: publicoas.ListNotesResponse{Items: items},
	}
	if result.NextCursor != "" {
		wrapped.Response.NextCursor = publicoas.NewOptString(result.NextCursor)
	}
	setPublicRequestIDHeader(wrapped, ctx)

	return wrapped, nil
}

func (h *oasHandler) UpdateNote(ctx context.Context, req *publicoas.UpdateNoteRequest, params publicoas.UpdateNoteParams) (publicoas.UpdateNoteRes, error) {
	updated, err := h.notesService.UpdateNote(ctx, params.NoteId, notes.UpdateInput{
		Title:    optStringPtr(req.Title),
		Body:     optStringPtr(req.Body),
		Location: optStringPtr(req.Location),
	})
	if err != nil {
		return nil, publicDefaultError(ctx, err)
	}

	wrapped := &publicoas.NoteHeaders{Response: publicNote(updated)}
	setPublicRequestIDHeader(wrapped, ctx)

	return wrapped, nil
}

func optStringPtr(value publicoas.OptString) *string {
	if !value.IsSet() {
		return nil
	}

	v := value.Value
	return &v
}

func publicNote(note notes.Note) publicoas.Note {
	return publicoas.Note{
		ID:               note.ID,
		Title:            note.Title,
		Body:             note.Body,
		LocationQuery:    note.LocationQuery,
		ResolvedLocation: note.ResolvedLocation,
		Weather: publicoas.NoteWeatherSnapshot{
			Provider:     note.Weather.Provider,
			Condition:    note.Weather.Condition,
			TemperatureC: note.Weather.TemperatureC,
			ObservedAt:   note.Weather.ObservedAt,
		},
		CreatedAt: note.CreatedAt,
		UpdatedAt: note.UpdatedAt,
	}
}

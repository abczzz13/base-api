package notes

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrNoteNotFound = errors.New("note not found")

// Repository persists notes.
type Repository interface {
	CreateNote(context.Context, CreateParams) (CreateResult, error)
	GetNote(context.Context, uuid.UUID) (Note, error)
	ListNotesPage(context.Context, ListPageParams) ([]Note, error)
	UpdateNote(context.Context, UpdateParams) (UpdateResult, error)
	DeleteNote(context.Context, uuid.UUID) error
}

// RepositoryFuncs adapts function fields into Repository for tests.
type RepositoryFuncs struct {
	CreateNoteFunc    func(context.Context, CreateParams) (CreateResult, error)
	GetNoteFunc       func(context.Context, uuid.UUID) (Note, error)
	ListNotesPageFunc func(context.Context, ListPageParams) ([]Note, error)
	UpdateNoteFunc    func(context.Context, UpdateParams) (UpdateResult, error)
	DeleteNoteFunc    func(context.Context, uuid.UUID) error
}

// CreateNote implements Repository. A nil func returns a zero CreateResult and nil error.
func (f RepositoryFuncs) CreateNote(ctx context.Context, params CreateParams) (CreateResult, error) {
	if f.CreateNoteFunc == nil {
		return CreateResult{}, nil
	}

	return f.CreateNoteFunc(ctx, params)
}

// GetNote implements Repository. A nil func returns ErrNoteNotFound.
func (f RepositoryFuncs) GetNote(ctx context.Context, id uuid.UUID) (Note, error) {
	if f.GetNoteFunc == nil {
		return Note{}, ErrNoteNotFound
	}

	return f.GetNoteFunc(ctx, id)
}

// ListNotesPage implements Repository. A nil func returns nil, nil.
func (f RepositoryFuncs) ListNotesPage(ctx context.Context, params ListPageParams) ([]Note, error) {
	if f.ListNotesPageFunc == nil {
		return nil, nil
	}

	return f.ListNotesPageFunc(ctx, params)
}

// UpdateNote implements Repository. A nil func returns a zero UpdateResult and nil error.
func (f RepositoryFuncs) UpdateNote(ctx context.Context, params UpdateParams) (UpdateResult, error) {
	if f.UpdateNoteFunc == nil {
		return UpdateResult{}, nil
	}

	return f.UpdateNoteFunc(ctx, params)
}

// DeleteNote implements Repository. A nil func returns ErrNoteNotFound.
func (f RepositoryFuncs) DeleteNote(ctx context.Context, id uuid.UUID) error {
	if f.DeleteNoteFunc == nil {
		return ErrNoteNotFound
	}

	return f.DeleteNoteFunc(ctx, id)
}

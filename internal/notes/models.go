package notes

import (
	"time"

	"github.com/google/uuid"
)

const (
	DefaultListLimit  = 20
	MaxListLimit      = 100
	maxTitleLength    = 200
	maxBodyLength     = 10000
	maxLocationLength = 200
)

// Note is the public domain model for a weather-enriched note.
type Note struct {
	ID               uuid.UUID
	Title            string
	Body             string
	LocationQuery    string
	ResolvedLocation string
	Weather          WeatherSnapshot
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// WeatherSnapshot captures the weather data associated with a note.
type WeatherSnapshot struct {
	Provider     string
	Condition    string
	TemperatureC float64
	ObservedAt   time.Time
}

// CreateInput contains fields required to create a note.
type CreateInput struct {
	Title    string
	Body     string
	Location string
}

// UpdateInput contains optional fields for a partial note update.
type UpdateInput struct {
	Title    *string
	Body     *string
	Location *string
}

// ListInput defines cursor pagination input for notes.
type ListInput struct {
	Limit  int
	Cursor string
}

// ListResult is a page of notes plus a cursor for the next page.
type ListResult struct {
	Items      []Note
	NextCursor string
}

// CreateParams contains validated repository fields for note creation.
type CreateParams struct {
	Title            string
	Body             string
	LocationQuery    string
	ResolvedLocation string
	Weather          WeatherSnapshot
}

// UpdateParams contains validated repository fields for note updates.
type UpdateParams struct {
	ID               uuid.UUID
	Title            string
	Body             string
	LocationQuery    string
	ResolvedLocation string
	Weather          WeatherSnapshot
}

// CreateResult contains DB-owned fields returned after note creation.
type CreateResult struct {
	ID        uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

// UpdateResult contains DB-owned fields returned after note updates.
type UpdateResult struct {
	UpdatedAt time.Time
}

// ListPageParams contains repository pagination inputs.
type ListPageParams struct {
	Limit  int
	Cursor *Cursor
}

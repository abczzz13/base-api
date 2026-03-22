package notes

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/abczzz13/base-api/internal/apierrors"
	"github.com/abczzz13/base-api/internal/clients/weather"
)

// Service contains handwritten note API behavior, independent of generated transport types.
type Service struct {
	repository    Repository
	weatherClient weather.Client
}

// NewService creates a new notes service.
func NewService(repository Repository, weatherClient weather.Client) (*Service, error) {
	if repository == nil {
		return nil, errors.New("notes repository is required")
	}
	if weatherClient == nil {
		return nil, errors.New("weather client is required")
	}

	return &Service{repository: repository, weatherClient: weatherClient}, nil
}

func (s *Service) CreateNote(ctx context.Context, input CreateInput) (Note, error) {
	title, err := validateRequiredString(input.Title, "title", maxTitleLength)
	if err != nil {
		return Note{}, err
	}
	body, err := validateRequiredString(input.Body, "body", maxBodyLength)
	if err != nil {
		return Note{}, err
	}
	location, err := validateRequiredString(input.Location, "location", maxLocationLength)
	if err != nil {
		return Note{}, err
	}

	weatherSnapshot, resolvedLocation, err := s.resolveWeather(ctx, location)
	if err != nil {
		return Note{}, err
	}

	created, err := s.repository.CreateNote(ctx, CreateParams{
		Title:            title,
		Body:             body,
		LocationQuery:    location,
		ResolvedLocation: resolvedLocation,
		Weather:          weatherSnapshot,
	})
	if err != nil {
		return Note{}, err
	}

	return Note{
		ID:               created.ID,
		Title:            title,
		Body:             body,
		LocationQuery:    location,
		ResolvedLocation: resolvedLocation,
		Weather:          weatherSnapshot,
		CreatedAt:        created.CreatedAt,
		UpdatedAt:        created.UpdatedAt,
	}, nil
}

func (s *Service) ListNotes(ctx context.Context, input ListInput) (ListResult, error) {
	limit := normalizeLimit(input.Limit)

	var cursor *Cursor
	if input.Cursor != "" {
		decoded, err := decodeCursor(input.Cursor)
		if err != nil {
			return ListResult{}, apierrors.New(http.StatusBadRequest, "invalid_cursor", "cursor is invalid")
		}
		cursor = &decoded
	}

	page, err := s.repository.ListNotesPage(ctx, ListPageParams{
		Limit:  limit + 1,
		Cursor: cursor,
	})
	if err != nil {
		return ListResult{}, err
	}

	result := ListResult{}
	if len(page) <= limit {
		result.Items = page
		return result, nil
	}

	result.Items = page[:limit]
	nextCursor, err := encodeCursor(Cursor{
		CreatedAt: result.Items[len(result.Items)-1].CreatedAt,
		ID:        result.Items[len(result.Items)-1].ID,
	})
	if err != nil {
		return ListResult{}, err
	}
	result.NextCursor = nextCursor

	return result, nil
}

func (s *Service) GetNote(ctx context.Context, id uuid.UUID) (Note, error) {
	note, err := s.repository.GetNote(ctx, id)
	if err != nil {
		return Note{}, noteError(err)
	}

	return note, nil
}

func (s *Service) UpdateNote(ctx context.Context, id uuid.UUID, input UpdateInput) (Note, error) {
	if input.Title == nil && input.Body == nil && input.Location == nil {
		return Note{}, apierrors.New(http.StatusBadRequest, "invalid_note_update", "at least one field must be provided")
	}

	current, err := s.repository.GetNote(ctx, id)
	if err != nil {
		return Note{}, noteError(err)
	}

	updated := UpdateParams{
		ID:               current.ID,
		Title:            current.Title,
		Body:             current.Body,
		LocationQuery:    current.LocationQuery,
		ResolvedLocation: current.ResolvedLocation,
		Weather:          current.Weather,
	}

	if input.Title != nil {
		updated.Title, err = validateRequiredString(*input.Title, "title", maxTitleLength)
		if err != nil {
			return Note{}, err
		}
	}
	if input.Body != nil {
		updated.Body, err = validateRequiredString(*input.Body, "body", maxBodyLength)
		if err != nil {
			return Note{}, err
		}
	}
	if input.Location != nil {
		updated.LocationQuery, err = validateRequiredString(*input.Location, "location", maxLocationLength)
		if err != nil {
			return Note{}, err
		}
		if updated.LocationQuery != current.LocationQuery {
			updated.Weather, updated.ResolvedLocation, err = s.resolveWeather(ctx, updated.LocationQuery)
			if err != nil {
				return Note{}, err
			}
		}
	}

	result, err := s.repository.UpdateNote(ctx, updated)
	if err != nil {
		return Note{}, noteError(err)
	}

	return Note{
		ID:               current.ID,
		Title:            updated.Title,
		Body:             updated.Body,
		LocationQuery:    updated.LocationQuery,
		ResolvedLocation: updated.ResolvedLocation,
		Weather:          updated.Weather,
		CreatedAt:        current.CreatedAt,
		UpdatedAt:        result.UpdatedAt,
	}, nil
}

func (s *Service) DeleteNote(ctx context.Context, id uuid.UUID) error {
	return noteError(s.repository.DeleteNote(ctx, id))
}

func (s *Service) resolveWeather(ctx context.Context, location string) (WeatherSnapshot, string, error) {
	currentWeather, err := s.weatherClient.GetCurrent(ctx, location)
	if err != nil {
		return WeatherSnapshot{}, "", noteWeatherError(err)
	}

	return WeatherSnapshot{
		Provider:     currentWeather.Provider,
		Condition:    currentWeather.Condition,
		TemperatureC: currentWeather.TemperatureC,
		ObservedAt:   currentWeather.ObservedAt,
	}, currentWeather.Location, nil
}

func noteError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNoteNotFound) {
		return apierrors.New(http.StatusNotFound, "note_not_found", "note not found")
	}

	return err
}

func noteWeatherError(err error) error {
	var notFoundErr *weather.NotFoundError
	if errors.As(err, &notFoundErr) {
		return apierrors.New(http.StatusUnprocessableEntity, "note_location_not_found", "note location could not be resolved")
	}

	var upstreamErr *weather.UpstreamError
	var decodeErr *weather.DecodeError
	if errors.As(err, &upstreamErr) || errors.As(err, &decodeErr) {
		return apierrors.New(http.StatusBadGateway, "weather_upstream_error", "weather provider returned an invalid response")
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return apierrors.New(http.StatusGatewayTimeout, "weather_timeout", "weather provider request timed out")
	}

	return apierrors.New(http.StatusBadGateway, "weather_request_failed", "weather provider request failed")
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return DefaultListLimit
	}

	return min(limit, MaxListLimit)
}

func validateRequiredString(value, field string, maxLength int) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", apierrors.New(http.StatusBadRequest, "invalid_note", field+" is required")
	}
	if maxLength > 0 && utf8.RuneCountInString(trimmed) > maxLength {
		return "", apierrors.New(http.StatusBadRequest, "invalid_note", field+" exceeds maximum length")
	}

	return trimmed, nil
}

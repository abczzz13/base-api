-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS notes (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    location_query TEXT NOT NULL,
    resolved_location TEXT NOT NULL,
    weather_provider TEXT NOT NULL,
    weather_condition TEXT NOT NULL,
    weather_temperature_c DOUBLE PRECISION NOT NULL,
    weather_observed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notes_title_non_empty CHECK (btrim(title) <> ''),
    CONSTRAINT notes_title_max_length CHECK (char_length(title) <= 200),
    CONSTRAINT notes_body_non_empty CHECK (btrim(body) <> ''),
    CONSTRAINT notes_body_max_length CHECK (char_length(body) <= 10000),
    CONSTRAINT notes_location_query_non_empty CHECK (btrim(location_query) <> ''),
    CONSTRAINT notes_location_query_max_length CHECK (char_length(location_query) <= 200),
    CONSTRAINT notes_resolved_location_non_empty CHECK (btrim(resolved_location) <> ''),
    CONSTRAINT notes_weather_provider_non_empty CHECK (btrim(weather_provider) <> ''),
    CONSTRAINT notes_weather_condition_non_empty CHECK (btrim(weather_condition) <> ''),
    CONSTRAINT notes_weather_temperature_reasonable CHECK (weather_temperature_c BETWEEN -150 AND 150),
    CONSTRAINT notes_updated_at_not_before_created_at CHECK (updated_at >= created_at)
);

CREATE INDEX IF NOT EXISTS idx_notes_created_at_id_desc ON notes (created_at DESC, id DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS notes;
-- +goose StatementEnd

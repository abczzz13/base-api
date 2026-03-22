-- name: InsertNote :one
INSERT INTO notes (
    title,
    body,
    location_query,
    resolved_location,
    weather_provider,
    weather_condition,
    weather_temperature_c,
    weather_observed_at
) VALUES (
    sqlc.arg(title),
    sqlc.arg(body),
    sqlc.arg(location_query),
    sqlc.arg(resolved_location),
    sqlc.arg(weather_provider),
    sqlc.arg(weather_condition),
    sqlc.arg(weather_temperature_c),
    sqlc.arg(weather_observed_at)
)
RETURNING id, created_at, updated_at;

-- name: GetNote :one
SELECT id, title, body, location_query, resolved_location, weather_provider, weather_condition, weather_temperature_c, weather_observed_at, created_at, updated_at
FROM notes
WHERE id = sqlc.arg(id);

-- name: ListNotesPage :many
SELECT id, title, body, location_query, resolved_location, weather_provider, weather_condition, weather_temperature_c, weather_observed_at, created_at, updated_at
FROM notes
WHERE (
    sqlc.narg(cursor_created_at)::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg(cursor_created_at)::timestamptz, sqlc.arg(cursor_id)::uuid)
)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count)::int;

-- name: UpdateNote :one
UPDATE notes
SET
    title = sqlc.arg(title),
    body = sqlc.arg(body),
    location_query = sqlc.arg(location_query),
    resolved_location = sqlc.arg(resolved_location),
    weather_provider = sqlc.arg(weather_provider),
    weather_condition = sqlc.arg(weather_condition),
    weather_temperature_c = sqlc.arg(weather_temperature_c),
    weather_observed_at = sqlc.arg(weather_observed_at),
    updated_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING updated_at;

-- name: DeleteNote :execrows
DELETE FROM notes
WHERE id = sqlc.arg(id);

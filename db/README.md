# Database Tooling

This project uses:

- `goose` for SQL migrations (`db/migrations`)
- `sqlc` for type-safe query code generation (`db/queries` -> `internal/dbsqlc`)

The API requires `DB_URL` at startup and exits fast when it is missing.
For local `docker compose` runs, the API container gets a service-local
`DB_URL` built from `POSTGRES_*` settings, so host-local `DB_URL` values (for
example `127.0.0.1`) do not break container startup.
When configured, the API runs pending `goose` migrations on startup unless
`DB_MIGRATE_ON_STARTUP=false`.
When set, `DB_CONNECT_TIMEOUT` overrides any `connect_timeout` value embedded
in `DB_URL`. Set `DB_CONNECT_TIMEOUT=0s` to keep `connect_timeout` from
`DB_URL`.
The initial baseline migration creates an `app_metadata` table.

`sqlc.yaml` uses the canonical schema snapshot in `db/schema.sql` together with
queries in `db/queries`. Regenerate code with `just sqlc-generate`; CI runs
`just check`, which includes `sqlc-check` to ensure generated `internal/dbsqlc`
code is up to date. Query code is currently scaffolding for upcoming runtime
query integration and is not wired into API handlers yet.

Common commands:

- `just db-up`
- `just db-down`
- `just db-status`
- `just db-create <name>`
- `just sqlc-generate`
- `TEST_DB_URL=postgres://postgres:postgres@127.0.0.1:5432/base_api?sslmode=disable go test ./internal/postgres -run TestOpenMigrateAndMetricsIntegration`

By default, `just db-*` commands use `DB_URL` when set, otherwise they fall back to:

`postgres://postgres:postgres@127.0.0.1:5432/base_api?sslmode=disable`

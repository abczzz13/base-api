# Base API

`base-api` is an opinionated Go API starter built to be cloned and renamed into new services.
It favors a production-minded default stack over framework flexibility:

- Go + standard library HTTP server
- OpenAPI-first handlers with `ogen`
- PostgreSQL migrations with `goose`
- Type-safe SQL access with `sqlc`
- Prometheus metrics and OpenTelemetry tracing
- Container-first local development with `docker compose`

The goal is to give new services a clean, idiomatic starting point with strong defaults for configuration, observability, testing, security, and delivery hygiene.

## What This Template Includes

- Separate public and infra HTTP servers
- Health, readiness, liveness, metrics, and docs endpoints
- Strict env-based configuration with `<KEY>_FILE` support
- Request IDs propagated through logs, errors, and audit records
- Request and outbound HTTP audit persistence in PostgreSQL
- An opt-in outbound weather integration example built on `internal/outboundhttp`
- Reproducible code generation checks for `ogen` and `sqlc`
- Distroless container image and hardened compose setup
- Split GitHub Actions workflows for CI, security, image delivery, and manual deployment

## Architecture

The runtime is intentionally small at the entrypoint and explicit at the composition root.

- `cmd/api`: process entrypoint
- `internal/server`: application composition root and lifecycle
- `internal/config`: config loading and validation
- `internal/publicapi`: public HTTP transport wiring
- `internal/infraapi`: infra HTTP transport wiring
- `internal/publicoas` and `internal/infraoas`: generated OpenAPI server code
- `internal/postgres`: database runtime setup, migrations, and metrics
- `internal/requestaudit` and `internal/outboundaudit`: audit persistence
- `internal/outboundhttp`: reusable instrumented outbound HTTP client
- `internal/weather`: example typed outbound integration

Request flow is:

1. request enters public or infra server
2. request ID, metrics, tracing, logging, recovery, and policy middleware run
3. generated `ogen` router dispatches to handwritten service code
4. errors are encoded to a shared schema and include `requestId`
5. request metadata is correlated in logs, traces, and audit records

The template includes one concrete outbound example: `GET /weather/current?location=...`, backed by Open-Meteo geocoding and forecast APIs.
When `WEATHER_ENABLED=true`, it calls a fixed-origin upstream through the shared outbound HTTP client, emitting outbound metrics and audit records automatically.

## Quickstart

### Prerequisites

- Go `1.26+`
- `just`
- Docker

### Local development

```bash
just tools
just env-init
just compose-up
just check
```

Useful endpoints after startup:

- Public API: `http://127.0.0.1:8080/healthz`
- Public weather example (after setting `WEATHER_ENABLED=true`): `http://127.0.0.1:8080/weather/current?location=Amsterdam`
- Infra liveness: `http://127.0.0.1:9090/livez`
- Infra readiness: `http://127.0.0.1:9090/readyz`
- Infra metrics: `http://127.0.0.1:9090/metrics`
- Swagger UI: `http://127.0.0.1:9090/docs`

Run directly on the host instead of compose:

```bash
cp .env.example .env
# Edit .env and set WEATHER_ENABLED=true to enable the example endpoint.
just db-start
just run
```

## Common Commands

```bash
just tools
just fmt
just lint-actions
just test
just check
just security
just pre-pr
just sqlc-generate
just ogen-generate
```

## Automation

- `CI` runs `just check` plus container build validation on pull requests, `main`, and manual dispatches.
- `Security` runs `just security`, optionally runs GitHub dependency review when the `ENABLE_DEPENDENCY_REVIEW` repository variable is set to `true`, runs Trivy config scanning, uploads Trivy SARIF into GitHub code scanning, and runs CodeQL on pull requests, `main`, a weekly schedule, and manual dispatches.
- `Release` runs on pushes to `main`, waits for successful `Lint`, `Test`, `Image Validation`, and `Security` runs for the same commit, then runs `go-semantic-release` to derive the next semver tag from Conventional Commits and create the GitHub Release. Configure the `RELEASE_TOKEN` secret so the generated `v*` tag can trigger downstream publish automation.
- `Publish` builds and scans per-platform image archives before publishing multi-arch `ghcr.io/abczzz13/base-api` images for `main`, `v*` tags, and manual feature-branch preview publishes, signs images with `cosign`, and emits deploy metadata.
- `Deploy` is a manual `workflow_dispatch` workflow that promotes a successful `Publish` run to `test` or `production` after cross-checking the publish metadata against immutable GitHub run data. Successful published runs from `main`, manual feature-branch previews, and `v*` tags may deploy to `test`; only successful tag-push runs built from `v*` tags may deploy to `production`.
- Placeholder deploy commands live in `justfile` as `just deploy-test` and `just deploy-prod`, so replacing the placeholder behavior later does not require reshaping the workflows.
- Optional Slack notifications are sent for deploy jobs when the `SLACK_WEBHOOK_URL` GitHub secret is configured; when it is absent, the notification steps skip cleanly.

## Rename This Template

When starting a new service, update the project identity before adding features.

1. Change the module path in `go.mod`.
2. Update the local import prefix in `justfile`.
3. Rename image names and OCI metadata in `Dockerfile` and `compose.yaml`.
4. Update OpenAPI titles and service naming in `api/openapi.yaml`, `api/infra_openapi.yaml`, and `internal/telemetry/telemetry.go`.
5. Update example env values, database names, and published URLs in `.env.example` and docs.
6. Search the repo for `base-api` and replace the remaining project-specific references.

After renaming, regenerate code and verify everything still passes:

```bash
just sqlc-generate
just ogen-generate
just check
```

## Development Notes

- Migrations live in `db/migrations`.
- SQL queries live in `db/queries` and generate code into `internal/dbsqlc`.
- OpenAPI specs live in `api/` and generate server code into `internal/publicoas` and `internal/infraoas`.
- The weather example is opt-in via `WEATHER_ENABLED=true`, uses Open-Meteo by default, and can be pointed at other origins with `WEATHER_GEOCODING_BASE_URL` and `WEATHER_FORECAST_BASE_URL`.
- Every package should keep a `doc.go` package comment.

## Project Docs

- Contribution guide: `CONTRIBUTING.md`
- Security policy: `SECURITY.md`
- Release process: `RELEASING.md`

## License

This project is licensed under the MIT License. See `LICENSE`.

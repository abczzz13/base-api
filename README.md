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
- An outbound weather integration example built on `internal/httpclient`
- Reproducible code generation checks for `ogen` and `sqlc`
- Distroless container image and hardened compose setup
- Split GitHub Actions workflows for CI, security, image delivery, and manual deployment

## Architecture

The runtime is intentionally small at the entrypoint and explicit at the composition root.

- `cmd/api`: process entrypoint
- `internal/server`: application composition root and lifecycle
- `internal/config`: config loading and validation
- `internal/publicapi`: public HTTP transport wiring, shared middleware, and mounted public handlers
- `internal/infraapi`: infra HTTP transport wiring
- `internal/publicoas`, `internal/weatheroas`, and `internal/infraoas`: generated OpenAPI server code

- `internal/postgres`: database runtime setup, migrations, and metrics
- `internal/requestaudit` and `internal/outboundaudit`: audit persistence
- `internal/httpclient`: reusable instrumented outbound HTTP client
- `internal/clients/weather`: example typed outbound integration client

Request flow is:

1. request enters public or infra server
2. request ID, metrics, tracing, logging, recovery, and policy middleware run
3. generated `ogen` router dispatches to handwritten service code
4. errors are encoded to a shared schema and include `requestId`
5. request metadata is correlated in logs, traces, and audit records

The template includes one concrete outbound example API: `GET /weather/current?location=...`, backed by Open-Meteo geocoding and forecast APIs.
The weather endpoint is part of the standard public surface and is served from its own OpenAPI spec and generated package while still sharing the main public HTTP entrypoint, middleware chain, outbound metrics, and audit records.

## Quickstart

### Prerequisites

- Nix with flakes enabled
- Docker

The Nix dev shell provides Go, `just`, and the full repo toolchain.
This repo commits `.envrc` on purpose so `nix-direnv` users can run `direnv allow` once and auto-load the shared project shell.

### Local development

```bash
nix develop
just env-init
just compose-up
just check
```

Quick verification from any shell:

```bash
nix develop -c just check
```

If you prefer a helper command that reuses your current shell, run:

```bash
just shell
```

If you use Nushell, `nix-direnv` is usually the smoothest option because it keeps you in your existing shell instead of spawning a separate `nix develop` session.

Useful endpoints after startup:

- Public API: `http://127.0.0.1:8080/healthz`
- Public weather example: `http://127.0.0.1:8080/weather/current?location=Amsterdam`
- Infra liveness: `http://127.0.0.1:9090/livez`
- Infra readiness: `http://127.0.0.1:9090/readyz`
- Infra metrics: `http://127.0.0.1:9090/metrics`
- Swagger UI: `http://127.0.0.1:9090/docs`
- Swagger specs: `http://127.0.0.1:9090/openapi/public.yaml`, `http://127.0.0.1:9090/openapi/weather.yaml`, `http://127.0.0.1:9090/openapi/infra.yaml`

Run directly on the host instead of compose:

```bash
nix develop
cp .env.example .env
just db-start
just run
```

## Common Commands

```bash
nix develop
just fmt
just fmt-nix
just lint-actions
just test
just check
just security
just pre-pr
just flake-check
just sqlc-generate
just ogen-generate
```

## Automation

- `CI` runs repository checks inside the pinned Nix dev shell, enforces the race detector and coverage gate, and smoke-tests the built runtime image on pull requests, `main`, and manual dispatches.
- `Security` runs repository security checks inside the pinned Nix dev shell, runs GitHub dependency review on pull requests, runs Trivy config scanning, uploads Trivy SARIF into GitHub code scanning, and runs CodeQL on pull requests, `main`, a weekly schedule, and manual dispatches.
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
nix develop -c just sqlc-generate
nix develop -c just ogen-generate
nix develop -c just check
```

## Development Notes

- Migrations live in `db/migrations`.
- SQL queries live in `db/queries` and generate code into `internal/dbsqlc`.
- OpenAPI specs live in `api/` and generate server code into `internal/publicoas`, `internal/weatheroas`, and `internal/infraoas`.
- The public API is split across `public.yaml` for core endpoints and `weather.yaml` for the weather endpoint.
- The weather endpoint uses Open-Meteo by default and can be pointed at other origins with `WEATHER_GEOCODING_BASE_URL` and `WEATHER_FORECAST_BASE_URL`.
- Every package should keep a `doc.go` package comment.

## Project Docs

- Contribution guide: `CONTRIBUTING.md`
- Security policy: `SECURITY.md`
- Release process: `RELEASING.md`

## License

This project is licensed under the MIT License. See `LICENSE`.

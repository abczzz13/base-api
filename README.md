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
- Reproducible code generation checks for `ogen` and `sqlc`
- Distroless container image and hardened compose setup
- CI quality gates for formatting, linting, tests, vuln scanning, and secret scanning

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

Request flow is:

1. request enters public or infra server
2. request ID, metrics, tracing, logging, recovery, and policy middleware run
3. generated `ogen` router dispatches to handwritten service code
4. errors are encoded to a shared schema and include `requestId`
5. request metadata is correlated in logs, traces, and audit records

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
- Infra liveness: `http://127.0.0.1:9090/livez`
- Infra readiness: `http://127.0.0.1:9090/readyz`
- Infra metrics: `http://127.0.0.1:9090/metrics`
- Swagger UI: `http://127.0.0.1:9090/docs`

Run directly on the host instead of compose:

```bash
cp .env.example .env
just db-start
just run
```

## Common Commands

```bash
just tools
just fmt
just test
just check
just security
just pre-pr
just sqlc-generate
just ogen-generate
```

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
- Every package should keep a `doc.go` package comment.

## Project Docs

- Contribution guide: `CONTRIBUTING.md`
- Security policy: `SECURITY.md`
- Release process: `RELEASING.md`

## License

This project is licensed under the MIT License. See `LICENSE`.

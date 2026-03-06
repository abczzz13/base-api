set dotenv-load := false
set ignore-comments := true
set shell := ["bash", "-eu", "-o", "pipefail", "-c"]
set script-interpreter := ["bash", "-eu", "-o", "pipefail"]

golangci_lint_version := "v2.10.1"
yamlfmt_version := "v0.21.0"
gofumpt_version := "v0.9.2"
goimports_version := "v0.42.0"
govulncheck_version := "v1.1.4"
gitleaks_version := "v8.30.0"
sqlc_version := "v1.27.0"
goose_version := "v3.27.0"
sqlc_docker_image := "sqlc/sqlc:1.27.0"
goimports_local_prefix := "github.com/abczzz13/base-api"
bin_dir := ".bin"
golangci_lint := ".bin/golangci-lint"
yamlfmt := ".bin/yamlfmt"
gofumpt := ".bin/gofumpt"
goimports := ".bin/goimports"
govulncheck := ".bin/govulncheck"
gitleaks := ".bin/gitleaks"
sqlc := ".bin/sqlc"
goose := ".bin/goose"
db_migrations_dir := "db/migrations"
sqlc_config := "sqlc.yaml"
db_url_default := "postgres://postgres:postgres@127.0.0.1:5432/base_api?sslmode=disable"
docker_compose := "docker compose --env-file .env"
go_api_package := "./cmd/api"
go_mod_mode := "-mod=vendor"
yaml_paths := "api/openapi.yaml api/infra_openapi.yaml compose.yaml .ogen.yml .github/workflows"
version := `git describe --tags --always --dirty 2>/dev/null || echo 'dev'`
git_branch := `git branch --show-current 2>/dev/null || echo 'unknown'`
git_commit := `git rev-parse HEAD 2>/dev/null || echo 'unknown'`
git_tag := `git describe --exact-match --tags 2>/dev/null || echo 'unknown'`
git_tree_state := `git rev-parse --is-inside-work-tree >/dev/null 2>&1 && { [ -n "$(git status --porcelain 2>/dev/null)" ] && echo 'dirty' || echo 'clean'; } || echo 'unknown'`
build_time := `date -u '+%Y-%m-%dT%H:%M:%SZ'`
version_package := "github.com/abczzz13/base-api/internal/version"

default:
    @just --list

ci: tools check security

# Expected local pre-PR quality gate.
pre-pr: check security


[script]
env-init:
    if [ -f .env ]; then
        exit 0
    fi

    cp .env.example .env
    printf 'Created .env from .env.example\n'

compose-up: env-init
    {{docker_compose}} up --build -d

compose-down: env-init
    {{docker_compose}} down --remove-orphans

compose-logs *args: env-init
    {{docker_compose}} logs -f {{args}}

compose-build: env-init
    {{docker_compose}} build

db-start: env-init
    {{docker_compose}} up -d postgres

build-go:
    go build {{go_mod_mode}} ./...

[script]
_version-ldflags:
    ldflags=(
        "-s"
        "-w"
        "-X" {{quote(version_package + ".buildVersion=" + version)}}
        "-X" {{quote(version_package + ".gitCommit=" + git_commit)}}
        "-X" {{quote(version_package + ".gitBranch=" + git_branch)}}
        "-X" {{quote(version_package + ".gitTag=" + git_tag)}}
        "-X" {{quote(version_package + ".gitTreeState=" + git_tree_state)}}
        "-X" {{quote(version_package + ".buildTime=" + build_time)}}
    )

    printf '%s' "${ldflags[*]}"

[script]
build-api: env-init
    mkdir -p build
    ldflags="$(just --quiet _version-ldflags)"
    go build {{go_mod_mode}} -ldflags "$ldflags" -o build/api {{go_api_package}}

[script]
build-image: env-init
    VERSION={{quote(version)}} \
    GIT_COMMIT={{quote(git_commit)}} \
    GIT_BRANCH={{quote(git_branch)}} \
    GIT_TAG={{quote(git_tag)}} \
    GIT_TREE_STATE={{quote(git_tree_state)}} \
    BUILD_TIME={{quote(build_time)}} \
    {{docker_compose}} build api

build: build-go build-api build-image

[script]
run-go: env-init
    ldflags="$(just --quiet _version-ldflags)"
    go run {{go_mod_mode}} -ldflags "$ldflags" {{go_api_package}}

run: run-go


tools:
    mkdir -p {{bin_dir}}
    GOBIN="${PWD}/{{bin_dir}}" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@{{golangci_lint_version}}
    GOBIN="${PWD}/{{bin_dir}}" go install github.com/google/yamlfmt/cmd/yamlfmt@{{yamlfmt_version}}
    GOBIN="${PWD}/{{bin_dir}}" go install mvdan.cc/gofumpt@{{gofumpt_version}}
    GOBIN="${PWD}/{{bin_dir}}" go install golang.org/x/tools/cmd/goimports@{{goimports_version}}
    GOBIN="${PWD}/{{bin_dir}}" go install golang.org/x/vuln/cmd/govulncheck@{{govulncheck_version}}
    GOBIN="${PWD}/{{bin_dir}}" go install github.com/zricethezav/gitleaks/v8@{{gitleaks_version}}
    GOBIN="${PWD}/{{bin_dir}}" go install github.com/sqlc-dev/sqlc/cmd/sqlc@{{sqlc_version}} || printf 'Warning: failed to install sqlc locally; sqlc commands will use Docker fallback\n'
    GOBIN="${PWD}/{{bin_dir}}" go install github.com/pressly/goose/v3/cmd/goose@{{goose_version}}

[script]
db-up: env-init
    [ -x "{{goose}}" ] || { printf 'Missing required tool: {{goose}}\nInstall with: just tools\n'; exit 1; }
    db_url="${DB_URL:-{{db_url_default}}}"
    {{goose}} -dir {{db_migrations_dir}} postgres "$db_url" up

[script]
db-down: env-init
    [ -x "{{goose}}" ] || { printf 'Missing required tool: {{goose}}\nInstall with: just tools\n'; exit 1; }
    db_url="${DB_URL:-{{db_url_default}}}"
    {{goose}} -dir {{db_migrations_dir}} postgres "$db_url" down

[script]
db-status: env-init
    [ -x "{{goose}}" ] || { printf 'Missing required tool: {{goose}}\nInstall with: just tools\n'; exit 1; }
    db_url="${DB_URL:-{{db_url_default}}}"
    {{goose}} -dir {{db_migrations_dir}} postgres "$db_url" status

db-create name:
    [ -x "{{goose}}" ] || { printf 'Missing required tool: {{goose}}\nInstall with: just tools\n'; exit 1; }
    {{goose}} -dir {{db_migrations_dir}} create {{name}} sql

sqlc-generate:
    sqlc_cmd=()
    if [ -x "{{sqlc}}" ]; then sqlc_cmd=("{{sqlc}}"); else sqlc_cmd=(docker run --rm -v "${PWD}:/src" -w /src "{{sqlc_docker_image}}"); fi
    "${sqlc_cmd[@]}" generate --file {{sqlc_config}}

[script]
ogen-generate:
    go generate {{go_mod_mode}} ./internal/publicoas ./internal/infraoas

[script]
sqlc-check:
    sqlc_cmd=()
    if [ -x "{{sqlc}}" ]; then sqlc_cmd=("{{sqlc}}"); else sqlc_cmd=(docker run --rm -v "${PWD}:/src" -w /src "{{sqlc_docker_image}}"); fi

    before="$(git status --porcelain -- internal/dbsqlc)"
    "${sqlc_cmd[@]}" generate --file {{sqlc_config}}
    after="$(git status --porcelain -- internal/dbsqlc)"

    if [ "$before" != "$after" ]; then
        printf 'sqlc generated code is out of date. Run: just sqlc-generate\n'
        printf 'Status before:\n%s\n' "${before:-<clean>}"
        printf 'Status after:\n%s\n' "${after:-<clean>}"
        exit 1
    fi

[script]
ogen-check:
    before="$(git status --porcelain -- internal/publicoas internal/infraoas)"
    go generate {{go_mod_mode}} ./internal/publicoas ./internal/infraoas
    after="$(git status --porcelain -- internal/publicoas internal/infraoas)"

    if [ "$before" != "$after" ]; then
        printf 'ogen generated code is out of date. Run: just ogen-generate\n'
        printf 'Status before:\n%s\n' "${before:-<clean>}"
        printf 'Status after:\n%s\n' "${after:-<clean>}"
        exit 1
    fi

[script]
check-tools:
    missing=()

    [ -x "{{golangci_lint}}" ] || missing+=("{{golangci_lint}}")
    [ -x "{{yamlfmt}}" ] || missing+=("{{yamlfmt}}")
    [ -x "{{gofumpt}}" ] || missing+=("{{gofumpt}}")
    [ -x "{{goimports}}" ] || missing+=("{{goimports}}")

    if [ "${#missing[@]}" -ne 0 ]; then
        printf 'Missing required local tools:\n'
        printf '  - %s\n' "${missing[@]}"
        printf 'Install them with: just tools\n'
        exit 1
    fi

[script]
fmt-go: check-tools
    files=()
    while IFS= read -r file; do
        if [ ! -f "$file" ]; then
            continue
        fi

        case "$file" in
            vendor/*)
                continue
                ;;
        esac

        first_line=""
        IFS= read -r first_line < "$file" || true
        case "$first_line" in
            "// Code generated "*)
                continue
                ;;
        esac

        files+=("$file")
    done < <(git ls-files --cached --others --exclude-standard -- '*.go')

    if [ "${#files[@]}" -eq 0 ]; then
        exit 0
    fi

    {{gofumpt}} -extra -w "${files[@]}"
    {{goimports}} -local '{{goimports_local_prefix}}' -w "${files[@]}"

[script]
fmt-go-check: check-tools
    files=()
    while IFS= read -r file; do
        if [ ! -f "$file" ]; then
            continue
        fi

        case "$file" in
            vendor/*)
                continue
                ;;
        esac

        first_line=""
        IFS= read -r first_line < "$file" || true
        case "$first_line" in
            "// Code generated "*)
                continue
                ;;
        esac

        files+=("$file")
    done < <(git ls-files --cached --others --exclude-standard -- '*.go')

    if [ "${#files[@]}" -eq 0 ]; then
        exit 0
    fi

    gofumpt_unformatted="$({{gofumpt}} -extra -l "${files[@]}")"
    goimports_unformatted="$({{goimports}} -local '{{goimports_local_prefix}}' -l "${files[@]}")"

    if [ -n "$gofumpt_unformatted" ] || [ -n "$goimports_unformatted" ]; then
        if [ -n "$gofumpt_unformatted" ]; then
            printf 'gofumpt formatting required:\n%s\n' "$gofumpt_unformatted"
        fi
        if [ -n "$goimports_unformatted" ]; then
            printf 'goimports formatting required:\n%s\n' "$goimports_unformatted"
        fi
        exit 1
    fi

fmt-yaml: check-tools
    {{yamlfmt}} {{yaml_paths}}

fmt: fmt-go fmt-yaml

lint-go: check-tools
    {{golangci_lint}} run ./...

lint-yaml: check-tools
	{{yamlfmt}} -lint {{yaml_paths}}

lint: lint-go lint-yaml

[script]
check-security-tools:
    missing=()

    [ -x "{{govulncheck}}" ] || missing+=("{{govulncheck}}")
    [ -x "{{gitleaks}}" ] || missing+=("{{gitleaks}}")

    if [ "${#missing[@]}" -ne 0 ]; then
        printf 'Missing required security tools:\n'
        printf '  - %s\n' "${missing[@]}"
        printf 'Install them with: just tools\n'
        exit 1
    fi

security-vuln: check-security-tools
    GOFLAGS="${GOFLAGS:+$GOFLAGS }{{go_mod_mode}}" {{govulncheck}} ./...

[script]
security-secrets: check-security-tools
    args=({{gitleaks}} git --redact --exit-code 1 --no-banner)
    if [ -n "${GITLEAKS_LOG_OPTS:-}" ]; then
        args+=(--log-opts "$GITLEAKS_LOG_OPTS")
    fi
    "${args[@]}"

security: security-vuln security-secrets

[script]
test pattern="" *args:
    echo {{ if pattern == "" { "Running full test suite with shuffle..." } else { if pattern == "--" { "Running full test suite with shuffle..." } else { "Running tests matching pattern: " + pattern } } }}
    if [ -f .env ]; then
        set -a
        # shellcheck disable=SC1091
        source .env
        set +a
    fi

    test_db_url="${TEST_DB_URL:-${DB_URL:-{{db_url_default}}}}"
    TEST_DB_URL="$test_db_url" go test {{go_mod_mode}} -v -p 1 -count=1 ./... {{ if pattern == "" { "-shuffle=on" } else { if pattern == "--" { "-shuffle=on" } else { "-run \"" + pattern + "\"" } } }} {{args}}

race:
    go test {{go_mod_mode}} -race ./...

[script]
coverage:
    packages=()
    while IFS= read -r pkg; do
        case "$pkg" in
            */internal/publicoas|*/internal/publicoas/*|*/internal/infraoas|*/internal/infraoas/*)
                continue
                ;;
        esac
        packages+=("$pkg")
    done < <(go list {{go_mod_mode}} ./...)

    go test {{go_mod_mode}} -coverprofile=coverage.out "${packages[@]}"
    go tool cover -func=coverage.out

coverage-all:
    go test {{go_mod_mode}} -coverprofile=coverage-all.out ./...
    go tool cover -func=coverage-all.out

vet:
    go vet {{go_mod_mode}} ./...

[script]
vendor-check:
    before="$(git status --porcelain -- go.mod go.sum vendor)"
    go mod tidy
    go mod vendor
    after="$(git status --porcelain -- go.mod go.sum vendor)"

    if [ "$before" != "$after" ]; then
        printf 'Go dependencies or vendor tree are out of date. Run: go mod tidy && go mod vendor\n'
        printf 'Status before:\n%s\n' "${before:-<clean>}"
        printf 'Status after:\n%s\n' "${after:-<clean>}"
        exit 1
    fi

tidy-check: vendor-check

check: fmt-go-check lint test sqlc-check ogen-check vendor-check

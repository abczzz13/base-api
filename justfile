set dotenv-load := false
set ignore-comments := true
set shell := ["bash", "-eu", "-o", "pipefail", "-c"]
set script-interpreter := ["bash", "-eu", "-o", "pipefail"]

sqlc_docker_image := "sqlc/sqlc:1.30.0"
goimports_local_prefix := "github.com/abczzz13/base-api"
golangci_lint := "golangci-lint"
yamlfmt := "yamlfmt"
gofumpt := "gofumpt"
goimports := "goimports"
govulncheck := "govulncheck"
gitleaks := "gitleaks"
actionlint := "actionlint"
shellcheck := "shellcheck"
nix := "nix"
sqlc := "sqlc"
goose := "goose"
ogen := "ogen"
db_migrations_dir := "db/migrations"
sqlc_config := "sqlc.yaml"
db_url_default := "postgres://postgres:postgres@127.0.0.1:5432/base_api?sslmode=disable"
coverage_min_percent := "70.0"
docker_compose := "docker compose --env-file .env"
go_api_package := "./cmd/api"
go_mod_mode := "-mod=vendor"
yaml_paths := "api/openapi.yaml api/weather_openapi.yaml api/infra_openapi.yaml compose.yaml .ogen.yml .github/workflows .github/actions"
nix_paths := "flake.nix"
version := `git describe --tags --always --dirty 2>/dev/null || echo 'dev'`
git_branch := `git branch --show-current 2>/dev/null || echo 'unknown'`
git_commit := `git rev-parse HEAD 2>/dev/null || echo 'unknown'`
git_tag := `git describe --exact-match --tags 2>/dev/null || echo 'unknown'`
git_tree_state := `git rev-parse --is-inside-work-tree >/dev/null 2>&1 && { [ -n "$(git status --porcelain 2>/dev/null)" ] && echo 'dirty' || echo 'clean'; } || echo 'unknown'`
build_time := `date -u '+%Y-%m-%dT%H:%M:%SZ'`
version_package := "github.com/abczzz13/base-api/internal/version"

default:
    @just --list

ci: check security

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

valkey-start: env-init
    {{docker_compose}} up -d valkey

services: env-init
    {{docker_compose}} up -d postgres valkey

services-down: env-init
    {{docker_compose}} stop postgres valkey

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

# Build the runtime Docker image without compose (for CI validation).
build-image-ci:
    docker build --target runner -t base-api:ci .

build: build-go build-api build-image

[script]
run-go: env-init
    ldflags="$(just --quiet _version-ldflags)"
    go run {{go_mod_mode}} -ldflags "$ldflags" {{go_api_package}}

run: run-go

[script]
shell:
    target_shell="${SHELL:-bash}"
    if [ -n "${NU_VERSION:-}" ] && command -v nu >/dev/null 2>&1; then
        target_shell="$(command -v nu)"
    fi

    if [ -n "${IN_NIX_SHELL:-}" ]; then
        exec "$target_shell"
    fi

    exec nix develop -c "$target_shell"


tools:
    printf 'This repository is Nix-first. Run `nix develop` to get the full toolchain.\n'

[script]
db-up: env-init
    command -v {{goose}} >/dev/null 2>&1 || { printf 'Missing required tool: {{goose}}\nRun inside `nix develop`.\n'; exit 1; }
    db_url="${DB_URL:-{{db_url_default}}}"
    {{goose}} -dir {{db_migrations_dir}} postgres "$db_url" up

[script]
db-down: env-init
    command -v {{goose}} >/dev/null 2>&1 || { printf 'Missing required tool: {{goose}}\nRun inside `nix develop`.\n'; exit 1; }
    db_url="${DB_URL:-{{db_url_default}}}"
    {{goose}} -dir {{db_migrations_dir}} postgres "$db_url" down

[script]
db-status: env-init
    command -v {{goose}} >/dev/null 2>&1 || { printf 'Missing required tool: {{goose}}\nRun inside `nix develop`.\n'; exit 1; }
    db_url="${DB_URL:-{{db_url_default}}}"
    {{goose}} -dir {{db_migrations_dir}} postgres "$db_url" status

db-create name:
    command -v {{goose}} >/dev/null 2>&1 || { printf 'Missing required tool: {{goose}}\nRun inside `nix develop`.\n'; exit 1; }
    {{goose}} -dir {{db_migrations_dir}} create {{name}} sql

[script]
sqlc-generate:
    sqlc_cmd=()
    if command -v {{sqlc}} >/dev/null 2>&1; then sqlc_cmd=("{{sqlc}}"); else sqlc_cmd=(docker run --rm -v "${PWD}:/src" -w /src "{{sqlc_docker_image}}"); fi
    "${sqlc_cmd[@]}" generate --file {{sqlc_config}}

[script]
ogen-generate:
	go generate {{go_mod_mode}} ./internal/publicoas ./internal/weatheroas ./internal/infraoas

[script]
sqlc-check:
    sqlc_cmd=()
    if command -v {{sqlc}} >/dev/null 2>&1; then sqlc_cmd=("{{sqlc}}"); else sqlc_cmd=(docker run --rm -v "${PWD}:/src" -w /src "{{sqlc_docker_image}}"); fi

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
	before="$(git status --porcelain -- internal/publicoas internal/weatheroas internal/infraoas)"
	go generate {{go_mod_mode}} ./internal/publicoas ./internal/weatheroas ./internal/infraoas
	after="$(git status --porcelain -- internal/publicoas internal/weatheroas internal/infraoas)"

	if [ "$before" != "$after" ]; then
		printf 'ogen generated code is out of date. Run: just ogen-generate\n'
		printf 'Status before:\n%s\n' "${before:-<clean>}"
		printf 'Status after:\n%s\n' "${after:-<clean>}"
		exit 1
	fi

[script]
check-tools:
    missing=()

    command -v {{golangci_lint}} >/dev/null 2>&1 || missing+=("{{golangci_lint}}")
    command -v {{yamlfmt}} >/dev/null 2>&1 || missing+=("{{yamlfmt}}")
    command -v {{gofumpt}} >/dev/null 2>&1 || missing+=("{{gofumpt}}")
    command -v {{goimports}} >/dev/null 2>&1 || missing+=("{{goimports}}")
    command -v {{actionlint}} >/dev/null 2>&1 || missing+=("{{actionlint}}")
    command -v {{shellcheck}} >/dev/null 2>&1 || missing+=("{{shellcheck}}")
    command -v {{ogen}} >/dev/null 2>&1 || missing+=("{{ogen}}")

    if [ "${#missing[@]}" -ne 0 ]; then
        printf 'Missing required local tools:\n'
        printf '  - %s\n' "${missing[@]}"
        printf 'Run inside `nix develop`.\n'
        exit 1
    fi

check-nix-cli:
    command -v {{nix}} >/dev/null 2>&1 || { printf 'Missing required tool: {{nix}}\n'; exit 1; }

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

fmt-nix: check-nix-cli
    {{nix}} fmt {{nix_paths}}

[script]
fmt-nix-check: check-nix-cli
    {{nix}} fmt -- --check {{nix_paths}}

fmt: fmt-go fmt-yaml fmt-nix

lint-go: check-tools
    {{golangci_lint}} run ./...

lint-yaml: check-tools
	{{yamlfmt}} -lint {{yaml_paths}}

lint-actions: check-tools
    just check-action-pins
    {{actionlint}}

[script]
lint-shell: check-tools
    files=()
    while IFS= read -r file; do
        if [ ! -f "$file" ]; then
            continue
        fi

        files+=("$file")
    done < <(git ls-files --cached --others --exclude-standard -- 'scripts/*.sh' 'scripts/**/*.sh')

    if [ "${#files[@]}" -eq 0 ]; then
        exit 0
    fi

    {{shellcheck}} "${files[@]}"

lint: lint-go lint-yaml lint-actions lint-shell

flake-check: check-nix-cli
    {{nix}} flake check

[script]
check-security-tools:
    missing=()

    command -v {{govulncheck}} >/dev/null 2>&1 || missing+=("{{govulncheck}}")
    command -v {{gitleaks}} >/dev/null 2>&1 || missing+=("{{gitleaks}}")

    if [ "${#missing[@]}" -ne 0 ]; then
        printf 'Missing required security tools:\n'
        printf '  - %s\n' "${missing[@]}"
        printf 'Run inside `nix develop`.\n'
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
deploy-test:
    image_ref="${IMAGE_REF:-}"
    if [ -z "$image_ref" ]; then
        printf 'IMAGE_REF is required\n'
        exit 1
    fi

    printf 'Placeholder deploy for test environment\n'
    printf 'IMAGE_REF=%s\n' "$image_ref"
    # TODO: replace with real deploy command

[script]
deploy-prod:
    image_ref="${IMAGE_REF:-}"
    if [ -z "$image_ref" ]; then
        printf 'IMAGE_REF is required\n'
        exit 1
    fi

    printf 'Placeholder deploy for production environment\n'
    printf 'IMAGE_REF=%s\n' "$image_ref"
    # TODO: replace with real deploy command

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
            */internal/publicoas|*/internal/publicoas/*|*/internal/weatheroas|*/internal/weatheroas/*|*/internal/infraoas|*/internal/infraoas/*)
                continue
                ;;
        esac
        packages+=("$pkg")
    done < <(go list {{go_mod_mode}} ./...)

    go test {{go_mod_mode}} -coverprofile=coverage.out "${packages[@]}"
    go tool cover -func=coverage.out

[script]
coverage-enforce profile="coverage.out" min=coverage_min_percent:
    profile_path="{{profile}}"
    min_percent="{{min}}"

    if [ ! -f "$profile_path" ]; then
        printf 'coverage profile %s not found\n' "$profile_path"
        exit 1
    fi

    total_percent="$(go tool cover -func="$profile_path" | python3 -c 'import sys; lines = sys.stdin.read().splitlines(); matches = [line.split()[-1].rstrip("%") for line in lines if line.startswith("total:")]; print(matches[0] if matches else (_ for _ in ()).throw(SystemExit("missing total coverage line")))')"

    python3 -c 'import sys; total = float(sys.argv[1]); minimum = float(sys.argv[2]); print(f"coverage gate passed: {total:.1f}% >= {minimum:.1f}%") if total >= minimum else (_ for _ in ()).throw(SystemExit(f"coverage {total:.1f}% is below required minimum {minimum:.1f}%"))' "$total_percent" "$min_percent"

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

[script]
_pin-actions-in root:
    root="{{root}}"
    mapfile -t files < <(find "$root/.github" -name '*.yml' -type f)
    if [ "${#files[@]}" -eq 0 ]; then
        exit 0
    fi

    while IFS= read -r line; do
        # Skip comments and empty lines.
        case "$line" in
            '#'*|'') continue ;;
        esac

        action="${line%%=*}"
        pin="${line#*=}"
        sha="${pin%%#*}"
        sha="${sha%"${sha##*[! ]}"}"
        version="${pin#*#}"
        version="${version#"${version%%[! ]*}"}"

        # Replace uses: action(/optional-subpath)@anything with the pinned SHA.
        for f in "${files[@]}"; do
            sed -E "s|(uses: ${action}(/[^@]*)?)@[^[:space:]]+([ ]+#.*)?|\1@${sha} # ${version}|g" "$f" > "$f.tmp"
            mv "$f.tmp" "$f"
        done
    done < "$root/.github/action-versions.env"

[script]
pin-actions:
    just --quiet _pin-actions-in .

[script]
check-action-pins:
    tmpdir="$(mktemp -d)"
    trap 'rm -rf "$tmpdir"' EXIT
    cp -R .github "$tmpdir/.github"
    just --quiet _pin-actions-in "$tmpdir"

    if ! diff_output="$(diff -qr .github "$tmpdir/.github" || true)"; then
        :
    fi

    if [ -n "$diff_output" ]; then
        printf 'Action pins are out of sync. Run: just pin-actions\n'
        printf 'Differences:\n%s\n' "$diff_output"
        exit 1
    fi

check: fmt-go-check fmt-nix-check lint test sqlc-check ogen-check vendor-check flake-check

# CI-specific recipes — these are the contract between reusable GitHub Actions
# and the repository. Every repo that uses the shared actions must provide these.
ci-lint: fmt-go-check fmt-nix-check lint sqlc-check ogen-check vendor-check flake-check

ci-test: test

ci-race: race

ci-coverage: coverage coverage-enforce

ci-image-validate: build-image-ci

[script]
ci-image-smoke:
    image_ref="${IMAGE_SMOKE_IMAGE:-base-api:ci}"
    db_url="${DB_URL:-{{db_url_default}}}"
    container_db_url="${db_url//127.0.0.1/host.docker.internal}"
    container_db_url="${container_db_url//localhost/host.docker.internal}"
    container_name="base-api-ci-smoke"

    cleanup() {
        docker rm -f "$container_name" >/dev/null 2>&1 || true
    }
    trap cleanup EXIT
    cleanup

    docker run -d \
        --name "$container_name" \
        --add-host=host.docker.internal:host-gateway \
        -p 18080:8080 \
        -p 19090:9090 \
        -e DB_URL="$container_db_url" \
        -e API_ADDR="0.0.0.0:8080" \
        -e API_INFRA_ADDR="0.0.0.0:9090" \
        -e API_ENVIRONMENT="test" \
        "$image_ref" >/dev/null

    for url in \
        http://127.0.0.1:18080/healthz \
        http://127.0.0.1:19090/livez \
        http://127.0.0.1:19090/readyz \
        http://127.0.0.1:19090/metrics
    do
        success=0
        for _ in $(seq 1 60); do
            if curl --fail --silent --show-error "$url" >/dev/null; then
                success=1
                break
            fi
            sleep 1
        done

        if [ "$success" -ne 1 ]; then
            printf 'image smoke check failed for %s\n' "$url"
            docker logs "$container_name"
            exit 1
        fi
    done

ci-security: security

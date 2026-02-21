set dotenv-load := false
set ignore-comments := true
set shell := ["bash", "-eu", "-o", "pipefail", "-c"]
set script-interpreter := ["bash", "-eu", "-o", "pipefail"]

golangci_lint_version := "v2.10.1"
yamlfmt_version := "v0.21.0"
gofumpt_version := "v0.9.2"
goimports_version := "v0.42.0"
goimports_local_prefix := "github.com/abczzz13/base-api"
bin_dir := ".bin"
golangci_lint := ".bin/golangci-lint"
yamlfmt := ".bin/yamlfmt"
gofumpt := ".bin/gofumpt"
goimports := ".bin/goimports"
yaml_paths := "api/openapi.yaml api/infra_openapi.yaml compose.yaml .ogen.yml .github/workflows"

default:
    @just --list

ci: tools check

# Expected local pre-PR quality gate.
pre-pr: check


tools:
    mkdir -p {{bin_dir}}
    GOBIN="${PWD}/{{bin_dir}}" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@{{golangci_lint_version}}
    GOBIN="${PWD}/{{bin_dir}}" go install github.com/google/yamlfmt/cmd/yamlfmt@{{yamlfmt_version}}
    GOBIN="${PWD}/{{bin_dir}}" go install mvdan.cc/gofumpt@{{gofumpt_version}}
    GOBIN="${PWD}/{{bin_dir}}" go install golang.org/x/tools/cmd/goimports@{{goimports_version}}

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

test pattern="" *args:
    @echo {{ if pattern == "" { "Running full test suite with shuffle..." } else { if pattern == "--" { "Running full test suite with shuffle..." } else { "Running tests matching pattern: " + pattern } } }}
    @go test -v -p 1 -count=1 ./... {{ if pattern == "" { "-shuffle=on" } else { if pattern == "--" { "-shuffle=on" } else { "-run \"" + pattern + "\"" } } }} {{args}}

race:
    go test -race ./...

[script]
coverage:
    packages=()
    while IFS= read -r pkg; do
        case "$pkg" in
            */internal/oas|*/internal/oas/*|*/internal/infraoas|*/internal/infraoas/*)
                continue
                ;;
        esac
        packages+=("$pkg")
    done < <(go list ./...)

    go test -coverprofile=coverage.out "${packages[@]}"
    go tool cover -func=coverage.out

coverage-all:
    go test -coverprofile=coverage-all.out ./...
    go tool cover -func=coverage-all.out

vet:
    go vet ./...

tidy-check:
    before="$(git status --porcelain -- go.mod go.sum)"; go mod tidy; after="$(git status --porcelain -- go.mod go.sum)"; test "$before" = "$after"

check: fmt-go-check lint test

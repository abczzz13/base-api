#!/usr/bin/env bash

set -euo pipefail

run_id="${INPUT_PUBLISH_RUN_ID:-}"
api_headers=(-H 'Accept: application/vnd.github+json' -H 'X-GitHub-Api-Version: 2022-11-28' -H "Authorization: Bearer ${GH_TOKEN}")

case "$run_id" in
  ''|*[!0-9]*)
    printf '::error::publish-run-id must be a numeric GitHub Actions run id.\n'
    exit 1
    ;;
esac

run_api="${GITHUB_API_URL}/repos/${GITHUB_REPOSITORY}/actions/runs/${run_id}"
run_json="$(curl -fsSL "${api_headers[@]}" "$run_api")"

workflow_path="$(jq -r '.path // empty' <<<"$run_json")"
status="$(jq -r '.status // empty' <<<"$run_json")"
conclusion="$(jq -r '.conclusion // empty' <<<"$run_json")"
run_url="$(jq -r '.html_url // empty' <<<"$run_json")"
run_event="$(jq -r '.event // empty' <<<"$run_json")"
head_branch="$(jq -r '.head_branch // empty' <<<"$run_json")"
head_sha="$(jq -r '.head_sha // empty' <<<"$run_json")"
head_repository="$(jq -r '.head_repository.full_name // empty' <<<"$run_json")"

case "$workflow_path" in
  .github/workflows/publish.yml|.github/workflows/publish.yml@*)
    ;;
  *)
    printf '::error::Run %s is not from the Publish workflow.\n' "$run_id"
    exit 1
    ;;
esac

if [ "$status" != 'completed' ] || [ "$conclusion" != 'success' ]; then
  printf '::error::Run %s is not a successful completed Publish run (status=%s, conclusion=%s).\n' "$run_id" "$status" "$conclusion"
  exit 1
fi

if [ -z "$run_event" ] || [ -z "$head_branch" ] || [ -z "$head_sha" ]; then
  printf '::error::Run %s is missing required GitHub source metadata (event=%s, head_branch=%s, head_sha=%s).\n' "$run_id" "$run_event" "$head_branch" "$head_sha"
  exit 1
fi

if [ "$head_repository" != "$GITHUB_REPOSITORY" ]; then
  printf '::error::Run %s originates from %s, expected %s.\n' "$run_id" "${head_repository:-<unknown>}" "$GITHUB_REPOSITORY"
  exit 1
fi

artifacts_api="${GITHUB_API_URL}/repos/${GITHUB_REPOSITORY}/actions/runs/${run_id}/artifacts?per_page=100"
artifacts_json="$(curl -fsSL "${api_headers[@]}" "$artifacts_api")"

if ! jq -e '.artifacts[]? | select(.name == "publish-metadata" and .expired == false)' >/dev/null <<<"$artifacts_json"; then
  printf '::error::Run %s does not expose a usable publish-metadata artifact.\n' "$run_id"
  exit 1
fi

{
  printf 'publish_run_id=%s\n' "$run_id"
  printf 'publish_run_url=%s\n' "$run_url"
  printf 'run_event=%s\n' "$run_event"
  printf 'head_branch=%s\n' "$head_branch"
  printf 'head_sha=%s\n' "$head_sha"
} >> "$GITHUB_OUTPUT"

#!/usr/bin/env bash

set -euo pipefail

required_workflows=(
  'Lint'
  'Test'
  'Image Validation'
  'Security'
)

poll_interval_seconds=20
timeout_seconds=900
deadline=$(( $(date +%s) + timeout_seconds ))
api_path="/repos/${GITHUB_REPOSITORY}/actions/runs?head_sha=${SOURCE_SHA}&branch=${SOURCE_BRANCH}&event=push&per_page=100"
never_seen_warning_threshold=10
declare -A never_seen_count

while :; do
  runs_json="$(gh api "$api_path")"
  pending_workflows=()
  failed_workflows=()

  for workflow_name in "${required_workflows[@]}"; do
    # GitHub API returns workflow_runs sorted by created_at desc; first() picks the latest run.
    workflow_state="$({
      jq -r \
        --arg workflow_name "$workflow_name" \
        'first(.workflow_runs[] | select(.name == $workflow_name) | "\(.status):\(.conclusion // \"\")") // empty'
    } <<<"$runs_json")"

    if [ -z "$workflow_state" ]; then
      never_seen_count["$workflow_name"]=$(( ${never_seen_count["$workflow_name"]:-0} + 1 ))
      if [ "${never_seen_count["$workflow_name"]}" -eq "$never_seen_warning_threshold" ]; then
        printf '::warning::Workflow "%s" has never appeared for %s after %d polls — check if it is misconfigured or renamed.\n' \
          "$workflow_name" "$SOURCE_SHA" "$never_seen_warning_threshold"
      fi
      pending_workflows+=("$workflow_name")
      continue
    fi

    workflow_status="${workflow_state%%:*}"
    workflow_conclusion="${workflow_state#*:}"

    if [ "$workflow_status" != 'completed' ]; then
      pending_workflows+=("$workflow_name")
      continue
    fi

    if [ "$workflow_conclusion" != 'success' ]; then
      failed_workflows+=("${workflow_name} (${workflow_conclusion:-unknown})")
    fi
  done

  if [ "${#failed_workflows[@]}" -ne 0 ]; then
    printf '::error::Required workflows failed for %s: %s\n' "$SOURCE_SHA" "${failed_workflows[*]}"
    exit 1
  fi

  if [ "${#pending_workflows[@]}" -eq 0 ]; then
    printf 'Required workflows succeeded for %s: %s\n' "$SOURCE_SHA" "${required_workflows[*]}"
    exit 0
  fi

  if [ "$(date +%s)" -ge "$deadline" ]; then
    printf '::error::Timed out waiting for required workflows for %s: %s\n' "$SOURCE_SHA" "${pending_workflows[*]}"
    exit 1
  fi

  printf 'Waiting for required workflows for %s: %s\n' "$SOURCE_SHA" "${pending_workflows[*]}"
  sleep "$poll_interval_seconds"
done

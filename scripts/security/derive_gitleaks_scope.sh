#!/usr/bin/env bash

set -euo pipefail

if [[ "${GITHUB_EVENT_NAME}" == "pull_request" ]]; then
  pr_base_sha="$(jq -r '.pull_request.base.sha // empty' "${GITHUB_EVENT_PATH}")"
  pr_head_sha="$(jq -r '.pull_request.head.sha // empty' "${GITHUB_EVENT_PATH}")"
  scope="${pr_base_sha}..${pr_head_sha}"
else
  before_sha="$(jq -r '.before // empty' "${GITHUB_EVENT_PATH}")"
  if [[ -n "${before_sha}" && "${before_sha}" != "0000000000000000000000000000000000000000" ]]; then
    scope="${before_sha}..${GITHUB_SHA}"
  else
    scope="--all"
  fi
fi

printf 'GITLEAKS_LOG_OPTS=%s\n' "$scope" >> "$GITHUB_ENV"
printf 'scope=%s\n' "$scope" >> "$GITHUB_OUTPUT"

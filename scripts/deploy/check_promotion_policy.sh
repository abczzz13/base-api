#!/usr/bin/env bash

set -euo pipefail

metadata_file="${METADATA_FILE:-artifacts/input/publish-metadata.json}"
api_headers=(-H 'Accept: application/vnd.github+json' -H 'X-GitHub-Api-Version: 2022-11-28' -H "Authorization: Bearer ${GH_TOKEN}")

if [ ! -f "$metadata_file" ]; then
  printf '::error::Trusted delivery metadata was not downloaded.\n'
  exit 1
fi

repository="$(jq -r '.repository // empty' "$metadata_file")"
run_id="$(jq -r '.run_id // empty' "$metadata_file")"
event_name="$(jq -r '.event_name // empty' "$metadata_file")"
image_ref="$(jq -r '.image_ref // empty' "$metadata_file")"
push_image="$(jq -r 'if .push_image == true then "true" else "false" end' "$metadata_file")"
source_repository="$(jq -r '.source_repository // empty' "$metadata_file")"
source_ref="$(jq -r '.source_ref // empty' "$metadata_file")"
source_ref_name="$(jq -r '.source_ref_name // empty' "$metadata_file")"
source_type="$(jq -r '.source_type // empty' "$metadata_file")"
source_sha="$(jq -r '.source_sha // empty' "$metadata_file")"
head_branch="${HEAD_BRANCH}"
head_sha="${HEAD_SHA}"
run_event="${RUN_EVENT}"

if [ "$repository" != "$GITHUB_REPOSITORY" ]; then
  printf '::error::Trusted metadata belongs to %s, expected %s.\n' "$repository" "$GITHUB_REPOSITORY"
  exit 1
fi

if [ "$source_repository" != "$GITHUB_REPOSITORY" ]; then
  printf '::error::Trusted metadata source repository is %s, expected %s.\n' "$source_repository" "$GITHUB_REPOSITORY"
  exit 1
fi

if [ "$run_id" != "$PUBLISH_RUN_ID" ]; then
  printf '::error::Trusted metadata run id %s does not match requested run %s.\n' "$run_id" "$PUBLISH_RUN_ID"
  exit 1
fi

if [ "$event_name" != "$run_event" ]; then
  printf '::error::Trusted metadata event %s does not match GitHub run event %s.\n' "$event_name" "$run_event"
  exit 1
fi

if [ "$push_image" != 'true' ]; then
  printf '::error::Publish run %s did not publish a deployable registry image.\n' "$run_id"
  exit 1
fi

if [ "$source_sha" != "$head_sha" ]; then
  printf '::error::Trusted metadata source SHA %s does not match GitHub run SHA %s.\n' "$source_sha" "$head_sha"
  exit 1
fi

if [ "$source_ref_name" != "$head_branch" ]; then
  printf '::error::Trusted metadata source ref name %s does not match GitHub run branch/tag %s.\n' "$source_ref_name" "$head_branch"
  exit 1
fi

case "$source_type" in
  branch)
    expected_source_ref="refs/heads/${head_branch}"
    ;;
  tag)
    expected_source_ref="refs/tags/${head_branch}"
    ;;
  *)
    printf '::error::Trusted metadata source_type %s is not supported.\n' "$source_type"
    exit 1
    ;;
esac

if [ "$source_ref" != "$expected_source_ref" ]; then
  printf '::error::Trusted metadata source ref %s does not match expected %s.\n' "$source_ref" "$expected_source_ref"
  exit 1
fi

case "$image_ref" in
  *@sha256:*)
    ;;
  *)
    printf '::error::Trusted metadata image_ref %s is not an immutable digest reference.\n' "$image_ref"
    exit 1
    ;;
esac

if [ "${TARGET_ENVIRONMENT}" = 'production' ]; then
  if [ "$run_event" != 'push' ]; then
    printf '::error::Production deploys require a tag-push Publish run; got event %s.\n' "$run_event"
    exit 1
  fi

  if [ "$source_type" != 'tag' ] || [[ ! "$source_ref_name" == v* ]]; then
    printf '::error::Production deploys require a Publish run built from a v* tag; got %s (%s).\n' "$source_type" "$source_ref"
    exit 1
  fi

  tag_ref_encoded="$(jq -rn --arg ref "tags/${source_ref_name}" '$ref | @uri')"
  tag_api="${GITHUB_API_URL}/repos/${GITHUB_REPOSITORY}/git/ref/${tag_ref_encoded}"

  if ! tag_json="$(curl -fsSL "${api_headers[@]}" "$tag_api")"; then
    printf '::error::Production deploys require tag %s to exist in %s.\n' "$source_ref_name" "$GITHUB_REPOSITORY"
    exit 1
  fi

  tag_object_type="$(jq -r '.object.type // empty' <<<"$tag_json")"
  tag_object_sha="$(jq -r '.object.sha // empty' <<<"$tag_json")"

  case "$tag_object_type" in
    commit)
      tag_commit_sha="$tag_object_sha"
      ;;
    tag)
      tag_object_api="${GITHUB_API_URL}/repos/${GITHUB_REPOSITORY}/git/tags/${tag_object_sha}"
      tag_object_json="$(curl -fsSL "${api_headers[@]}" "$tag_object_api")"
      nested_type="$(jq -r '.object.type // empty' <<<"$tag_object_json")"
      tag_commit_sha="$(jq -r '.object.sha // empty' <<<"$tag_object_json")"

      if [ "$nested_type" != 'commit' ] || [ -z "$tag_commit_sha" ]; then
        printf '::error::Tag %s does not resolve to a commit.\n' "$source_ref_name"
        exit 1
      fi
      ;;
    *)
      printf '::error::Tag %s resolved to unexpected object type %s.\n' "$source_ref_name" "$tag_object_type"
      exit 1
      ;;
  esac

  if [ "$tag_commit_sha" != "$head_sha" ]; then
    printf '::error::Tag %s resolves to %s, but the Publish run built %s.\n' "$source_ref_name" "$tag_commit_sha" "$head_sha"
    exit 1
  fi
fi

{
  printf 'image_ref=%s\n' "$image_ref"
  printf 'source_ref=%s\n' "$source_ref"
  printf 'source_ref_name=%s\n' "$source_ref_name"
  printf 'source_type=%s\n' "$source_type"
  printf 'source_sha=%s\n' "$source_sha"
} >> "$GITHUB_OUTPUT"

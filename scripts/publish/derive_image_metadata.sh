#!/usr/bin/env bash

set -euo pipefail

image_name="ghcr.io/$(printf '%s' "$GITHUB_REPOSITORY" | tr '[:upper:]' '[:lower:]')"
sha_short="${GITHUB_SHA::12}"
ref_name="${GITHUB_REF_NAME}"
ref_slug="$(printf '%s' "$ref_name" | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9._-' '-')"
build_time="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
source_repository="$GITHUB_REPOSITORY"
source_ref="$GITHUB_REF"
source_ref_name="$GITHUB_REF_NAME"
source_sha="$GITHUB_SHA"
source_type=branch
tags=()
git_branch=unknown
git_tag=unknown
build_platforms='linux/amd64'
scan_matrix='[{"platform":"linux/amd64","platform_slug":"linux-amd64"}]'
requires_qemu=false

if [[ "$GITHUB_REF" == refs/tags/* ]]; then
  source_type=tag
  git_tag="$ref_name"
  version_value="$ref_name"
  artifact_name="publish-tag-${ref_slug}-${sha_short}"
  tags=("${image_name}:${ref_name}" "${image_name}:sha-${sha_short}")

  if [[ "$ref_name" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
    major="${BASH_REMATCH[1]}"
    minor="${BASH_REMATCH[2]}"
    tags+=("${image_name}:v${major}.${minor}" "${image_name}:v${major}" "${image_name}:latest")
  fi
elif [ "$ref_name" = 'main' ]; then
  git_branch="$ref_name"
  version_value="edge-${sha_short}"
  artifact_name="publish-main-${sha_short}"
  tags=("${image_name}:edge" "${image_name}:sha-${sha_short}")
else
  git_branch="$ref_name"
  version_value="preview-${ref_slug}-${sha_short}"
  artifact_name="publish-preview-${ref_slug}-${sha_short}"
  tags=("${image_name}:preview-${ref_slug}-${sha_short}" "${image_name}:sha-${sha_short}")
fi

primary_image="${tags[0]}"

{
  printf 'artifact_name=%s\n' "$artifact_name"
  printf 'build_time=%s\n' "$build_time"
  printf 'build_platforms=%s\n' "$build_platforms"
  printf 'git_branch=%s\n' "$git_branch"
  printf 'git_tag=%s\n' "$git_tag"
  printf 'image_name=%s\n' "$image_name"
  printf 'primary_image=%s\n' "$primary_image"
  printf 'requires_qemu=%s\n' "$requires_qemu"
  printf 'scan_matrix=%s\n' "$scan_matrix"
  printf 'source_ref=%s\n' "$source_ref"
  printf 'source_ref_name=%s\n' "$source_ref_name"
  printf 'source_repository=%s\n' "$source_repository"
  printf 'source_sha=%s\n' "$source_sha"
  printf 'source_type=%s\n' "$source_type"
  printf 'version_value=%s\n' "$version_value"
  printf 'tags<<EOF\n'
  printf '%s\n' "${tags[@]}"
  printf 'EOF\n'
} >> "$GITHUB_OUTPUT"

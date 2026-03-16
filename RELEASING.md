# Releasing

This project uses `go-semantic-release` to create semver tags and GitHub Releases automatically from Conventional Commits merged into `main`. Changelogs live in the GitHub Release bodies — there is no `CHANGELOG.md` file.

## Automatic Release Flow

1. Merge a green pull request into `main`.
2. The `Release` workflow runs on the resulting `main` push and waits for `Lint`, `Test`, `Image Validation`, and `Security` to succeed for that same commit.
3. `go-semantic-release` reads the commits since the last `v*` tag and decides the next version:
   - `feat:` -> minor
   - `fix:` -> patch
   - `BREAKING CHANGE:` or `!` -> major
4. The workflow creates the `v<version>` tag and publishes the GitHub Release with generated release notes.
5. The generated tag triggers the `Publish` workflow, which builds, scans, and publishes the release image.
6. Trigger the `Deploy` workflow manually with `environment: production` and the successful tag-push `Publish` workflow `publish-run-id` to promote.

## Prerequisites

- `main` should keep `Lint`, `Test`, `Security`, and `Image Validation` as required checks. The `Release` workflow also waits for those workflows before tagging, but branch protection should still prevent bypasses.
- Configure a `RELEASE_TOKEN` GitHub Actions secret with permission to push tags and create releases. A fine-grained PAT or GitHub App token is preferred.
- Keep commit messages in Conventional Commit format so the release engine can classify them correctly.

## Bootstrapping

- If you want the first automated feature release to be `v0.1.0`, create an initial annotated `v0.0.0` tag before enabling the workflow.
- Without an existing release tag, `go-semantic-release` starts at `v1.0.0` for the first automated release.

## Deployment Lanes

- `Publish` builds and scans images for `main`, `v*` tags, and manual `workflow_dispatch` preview publishes.
- Deployments are manual: trigger the `Deploy` workflow via `workflow_dispatch` with the target environment and a successful `Publish` workflow `publish-run-id`.
- Any successful published delivery run may deploy to `test`; only successful tag-push runs built from `v*` tags may deploy to `production`.
- The current deploy steps are placeholders wired through `just deploy-test` and `just deploy-prod`, so the real deployment command can be swapped in later without changing the overall GitHub Actions shape.
- Deploy jobs send optional Slack notifications when the `SLACK_WEBHOOK_URL` GitHub secret is configured.

## Build Metadata

Version metadata is embedded at build time and exposed through runtime health information.
Build inputs are wired through `justfile`, `Dockerfile`, and `internal/version`.

Useful build commands:

```bash
just build-api
just build-image
```

## Container Publishing

The delivery workflow publishes images to `ghcr.io/abczzz13/base-api` by default.
If you publish images for a derived service, update the image name in `compose.yaml`, the delivery workflow, and OCI labels in `Dockerfile` as part of the rename step.

## Template Consumers

After cloning this template into a new service, set up the `Release` workflow and `RELEASE_TOKEN` secret early so tags, images, and release notes reflect the new service identity instead of the starter repository.

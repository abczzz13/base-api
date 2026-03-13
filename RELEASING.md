# Releasing

This project follows a simple semver-based manual release flow.

## Release Steps

1. Make sure the branch is green and `just pre-pr` passes.
2. Update any docs that need release notes context.
3. Create an annotated semver tag:

```bash
git tag -a v0.1.0 -m "v0.1.0"
```

4. Push the tag.
5. Wait for the tag-triggered `Publish` workflow to pass the pre-publish image scan gates and publish the release image.
6. Trigger the `Deploy` workflow manually (`workflow_dispatch`) with `environment: production` and the successful tag-push `Publish` workflow `publish-run-id` to promote.
7. Create a GitHub Release for the tag and summarize notable changes.

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

After cloning this template into a new service, set up a release workflow early so tags, images, and release notes reflect the new service identity instead of the starter repository.

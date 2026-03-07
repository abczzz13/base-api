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
5. Create a GitHub Release for the tag and summarize notable changes.

## Build Metadata

Version metadata is embedded at build time and exposed through runtime health information.
Build inputs are wired through `justfile`, `Dockerfile`, and `internal/version`.

Useful build commands:

```bash
just build-api
just build-image
```

## Container Publishing

If you publish images for a derived service, update the image name in `compose.yaml` and OCI labels in `Dockerfile` as part of the rename step.

## Template Consumers

After cloning this template into a new service, set up a release workflow early so tags, images, and release notes reflect the new service identity instead of the starter repository.

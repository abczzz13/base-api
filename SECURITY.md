# Security Policy

## Reporting

If you discover a security issue, please do not open a public issue with exploit details.

Instead, report it privately to the maintainer through the repository hosting platform or another agreed private channel.

Please include:

- a clear description of the issue
- affected versions or commits
- reproduction steps or proof of concept
- suggested mitigations, if known

## Project Security Practices

This project currently includes:

- dependency vulnerability scanning with `govulncheck`
- secret scanning with `gitleaks`
- CI config scanning and image scanning with Trivy
- non-root distroless runtime images
- read-only container filesystem and dropped Linux capabilities in compose

Local security checks:

```bash
just security
```

## Supported Versions

This repository is a template starter, so the supported version is the current default branch unless noted otherwise in releases.

# AGENTS.md

Guidelines for AI agents working on this codebase.

## Commands

Use `just` commands for all checks and builds. Install tools first if needed:

```bash
just tools        # Install required tools (golangci-lint, gofumpt, etc.)
just check        # Run formatting check, linting, and tests
just security     # Run vulnerability and secret scanning
just pre-pr       # Full pre-PR quality gate (check + security)
```

**Important:** Always run `just check` after making code changes to verify formatting, linting, and tests pass.

## Testing

All changed or new features must be accompanied by tests. Preferred approach:

- Use table-driven tests with the standard library
- Use `cmp.Diff` from `github.com/google/go-cmp/cmp` for comparisons
